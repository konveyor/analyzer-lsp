package java

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/nxadm/tail"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
)

const (
	JavaFile          = ".java"
	JavaArchive       = ".jar"
	WebArchive        = ".war"
	EnterpriseArchive = ".ear"
	ClassFile         = ".class"
	MvnURIPrefix      = "mvn://"
)

// provider specific config keys
const (
	BUNDLES_INIT_OPTION           = "bundles"
	WORKSPACE_INIT_OPTION         = "workspace"
	MVN_SETTINGS_FILE_INIT_OPTION = "mavenSettingsFile"
	GLOBAL_SETTINGS_INIT_OPTION   = "mavenCacheDir"
	MVN_INSECURE_SETTING          = "mavenInsecure"
	CLEAN_EXPLODED_BIN_OPTION     = "cleanExplodedBin"
	JVM_MAX_MEM_INIT_OPTION       = "jvmMaxMem"
	FERN_FLOWER_INIT_OPTION       = "fernFlowerPath"
)

// Rule Location to location that the bundle understands
var locationToCode = map[string]int{
	//Type is the default.
	"":                 0,
	"inheritance":      1,
	"method_call":      2,
	"constructor_call": 3,
	"annotation":       4,
	"implements_type":  5,
	// Not Implemented
	"enum":                 6,
	"return_type":          7,
	"import":               8,
	"variable_declaration": 9,
	"type":                 10,
	"package":              11,
	"field":                12,
	"method":               13,
	"class":                14,
}

type javaProvider struct {
	config       provider.Config
	Log          logr.Logger
	contextLines int

	clients []provider.ServiceClient

	lspServerName string

	hasMaven          bool
	depsMutex         sync.RWMutex
	depsLocationCache map[string]int

	logFollow sync.Once
}

var _ provider.BaseClient = &javaProvider{}

var _ provider.InternalProviderClient = &javaProvider{}

var _ provider.DependencyLocationResolver = &javaProvider{}

type javaCondition struct {
	Referenced referenceCondition `yaml:"referenced"`
}

type referenceCondition struct {
	Pattern   string    `yaml:"pattern"`
	Location  string    `yaml:"location"`
	Annotated annotated `yaml:"annotated,omitempty" json:"annotated,omitempty"`
	Filepaths []string  `yaml:"filepaths"`
}

type annotated struct {
	Pattern  string    `yaml:"pattern" json:"pattern"`
	Elements []element `yaml:"elements,omitempty" json:"elements,omitempty"`
}

type element struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"` // can be a (java) regex pattern
}

func NewJavaProvider(log logr.Logger, lspServerName string, contextLines int, config provider.Config) *javaProvider {

	_, mvnBinaryError := exec.LookPath("mvn")

	return &javaProvider{
		config:            config,
		hasMaven:          mvnBinaryError == nil,
		Log:               log,
		clients:           []provider.ServiceClient{},
		lspServerName:     lspServerName,
		depsLocationCache: make(map[string]int),
		contextLines:      contextLines,
		logFollow:         sync.Once{},
	}
}

func (p *javaProvider) Stop() {
	// Ignore the error here, it stopped and we wanted it to.
	for _, c := range p.clients {
		c.Stop()
	}
}

func (p *javaProvider) Capabilities() []provider.Capability {
	r := openapi3.NewReflector()
	caps := []provider.Capability{}
	refCap, err := provider.ToProviderCap(r, p.Log, javaCondition{}, "referenced")
	if err != nil {
		p.Log.Error(err, "this is not going to be cool if it fails")
	} else {
		caps = append(caps, refCap)
	}
	if p.hasMaven {
		depCap, err := provider.ToProviderCap(r, p.Log, provider.DependencyConditionCap{}, "dependency")
		if err != nil {
			p.Log.Error(err, "this is not goinag to be cool if it fails")
		} else {
			caps = append(caps, depCap)
		}
	}
	return caps
}

func symbolKindToString(symbolKind protocol.SymbolKind) string {
	switch symbolKind {
	case 1:
		return "File"
	case 2:
		return "Module"
	case 3:
		return "Namespace"
	case 4:
		return "Package"
	case 5:
		return "Class"
	case 6:
		return "Method"
	case 7:
		return "Property"
	case 8:
		return "Field"
	case 9:
		return "Constructor"
	case 10:
		return "Enum"
	case 11:
		return "Interface"
	case 12:
		return "Function"
	case 13:
		return "Variable"
	case 14:
		return "Constant"
	case 15:
		return "String"
	case 16:
		return "Number"
	case 17:
		return "Boolean"
	case 18:
		return "Array"
	case 19:
		return "Object"
	case 20:
		return "Key"
	case 21:
		return "Null"
	case 22:
		return "EnumMember"
	case 23:
		return "Struct"
	case 24:
		return "Event"
	case 25:
		return "Operator"
	case 26:
		return "TypeParameter"
	}
	return ""
}

func (p *javaProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	// By default, if nothing is set for analysis mode in the config, we should default to full for external providers
	var mode provider.AnalysisMode = provider.AnalysisMode(config.AnalysisMode)
	// in case of a binary, provider decompiles it and returns
	// a builtin config that points to the decompiled archive
	additionalBuiltinConfig := provider.InitConfig{}

	if mode == provider.AnalysisMode("") {
		mode = provider.FullAnalysisMode
	} else if !(mode == provider.FullAnalysisMode || mode == provider.SourceOnlyAnalysisMode) {
		return nil, additionalBuiltinConfig, fmt.Errorf("invalid Analysis Mode")
	}
	log = log.WithValues("provider", "java")

	if config.RPC != nil {
		return &javaServiceClient{
			rpc:               config.RPC,
			config:            config,
			log:               log,
			depsLocationCache: make(map[string]int),
			includedPaths:     provider.GetIncludedPathsFromConfig(config, false),
		}, provider.InitConfig{}, nil
	}

	// read provider settings
	bundlesString, ok := config.ProviderSpecificConfig[BUNDLES_INIT_OPTION].(string)
	if !ok {
		bundlesString = ""
	}
	bundles := strings.Split(bundlesString, ",")

	workspace, ok := config.ProviderSpecificConfig[WORKSPACE_INIT_OPTION].(string)
	if !ok {
		workspace = ""
	}

	mavenSettingsFile, ok := config.ProviderSpecificConfig[MVN_SETTINGS_FILE_INIT_OPTION].(string)
	if !ok {
		mavenSettingsFile = ""
	}
	var globalSettingsFile string
	var returnError error
	globalM2, ok := config.ProviderSpecificConfig[GLOBAL_SETTINGS_INIT_OPTION].(string)
	if !ok {
		globalM2 = ""
	} else {
		globalSettingsFile, returnError = p.BuildSettingsFile(globalM2)
		if returnError != nil {
			return nil, additionalBuiltinConfig, returnError
		}
	}

	mavenInsecure, ok := config.ProviderSpecificConfig[MVN_INSECURE_SETTING].(bool)
	if ok && mavenInsecure {
		log.Info("maven insecure setting enabled")
	} else {
		mavenInsecure = false
	}

	lspServerPath, ok := config.ProviderSpecificConfig[provider.LspServerPathConfigKey].(string)
	if !ok || lspServerPath == "" {
		return nil, additionalBuiltinConfig, fmt.Errorf("invalid lspServerPath provided, unable to init java provider")
	}
	fernflower, ok := config.ProviderSpecificConfig[FERN_FLOWER_INIT_OPTION].(string)
	if !ok {
		fernflower = "/bin/fernflower.jar"
	}

	isBinary := false
	var returnErr error
	// each service client should have their own context
	ctx, cancelFunc := context.WithCancel(ctx)
	// location can be a coordinate to a remote mvn artifact
	if strings.HasPrefix(config.Location, MvnURIPrefix) {
		mvnUri := strings.Replace(config.Location, MvnURIPrefix, "", 1)
		// URI format is <group>:<artifact>:<version>:<classifier>@<path>
		// <path> is optional & points to a local path where it will be downloaded
		mvnCoordinates, destPath, _ := strings.Cut(mvnUri, "@")
		mvnCoordinatesParts := strings.Split(mvnCoordinates, ":")
		if mvnCoordinates == "" || len(mvnCoordinatesParts) < 3 {
			cancelFunc()
			return nil, additionalBuiltinConfig, fmt.Errorf("invalid maven coordinates in location %s, must be in format mvn://<group>:<artifact>:<version>:<classifier>@<path>", config.Location)
		}
		outputDir := "."
		if destPath != "" {
			if stat, err := os.Stat(destPath); err != nil || !stat.IsDir() {
				cancelFunc()
				return nil, additionalBuiltinConfig, fmt.Errorf("output path does not exist or not a directory")
			}
			outputDir = destPath
		}
		mvnOptions := []string{
			"dependency:copy",
			fmt.Sprintf("-Dartifact=%s", mvnCoordinates),
			fmt.Sprintf("-DoutputDirectory=%s", outputDir),
		}
		if mavenSettingsFile != "" {
			mvnOptions = append(mvnOptions, "-s", mavenSettingsFile)
		}
		if mavenInsecure {
			mvnOptions = append(mvnOptions, "-Dmaven.wagon.http.ssl.insecure=true")
		}
		log.Info("downloading maven artifact", "artifact", mvnCoordinates, "options", mvnOptions)
		cmd := exec.CommandContext(ctx, "mvn", mvnOptions...)
		cmd.Dir = outputDir
		mvnOutput, err := cmd.CombinedOutput()
		if err != nil {
			cancelFunc()
			return nil, additionalBuiltinConfig, fmt.Errorf("error downloading java artifact %s - maven output: %s - with error %w", mvnUri, string(mvnOutput), err)
		}
		downloadedPath := filepath.Join(outputDir,
			fmt.Sprintf("%s.jar", strings.Join(mvnCoordinatesParts[1:3], "-")))
		if len(mvnCoordinatesParts) == 4 {
			downloadedPath = filepath.Join(outputDir,
				fmt.Sprintf("%s.%s", strings.Join(mvnCoordinatesParts[1:3], "-"), strings.ToLower(mvnCoordinatesParts[3])))
		}
		outputLinePattern := regexp.MustCompile(`.*?Copying.*?to (.*)`)
		for _, line := range strings.Split(string(mvnOutput), "\n") {
			if outputLinePattern.MatchString(line) {
				match := outputLinePattern.FindStringSubmatch(line)
				if match != nil {
					downloadedPath = match[1]
				}
			}
		}
		if _, err := os.Stat(downloadedPath); err != nil {
			cancelFunc()
			return nil, additionalBuiltinConfig, fmt.Errorf("failed to download maven artifact to path %s - %w", downloadedPath, err)
		}
		config.Location = downloadedPath
	}

	extension := strings.ToLower(path.Ext(config.Location))
	explodedBins := []string{}
	switch extension {
	case JavaArchive, WebArchive, EnterpriseArchive:
		cleanBin, ok := config.ProviderSpecificConfig[CLEAN_EXPLODED_BIN_OPTION].(bool)

		depLocation, sourceLocation, err := decompileJava(ctx, log, fernflower,
			config.Location, getMavenLocalRepoPath(mavenSettingsFile), ok)
		if err != nil {
			cancelFunc()
			return nil, additionalBuiltinConfig, err
		}
		config.Location = sourceLocation
		// for binaries, we fallback to looking at .jar files only for deps
		config.DependencyPath = depLocation
		isBinary = true

		if ok && cleanBin {
			log.Info("removing exploded binaries after analysis")
			explodedBins = append(explodedBins, depLocation, sourceLocation)
		}
	}

	additionalBuiltinConfig.Location = config.Location
	additionalBuiltinConfig.DependencyPath = config.DependencyPath

	if config.Proxy != nil {
		// handle proxy settings
		for k, v := range config.Proxy.ToEnvVars() {
			os.Setenv(k, v)
		}
	}

	jdtlsBasePath, err := filepath.Abs(filepath.Dir(filepath.Dir(lspServerPath)))
	if err != nil {
		cancelFunc()
		return nil, additionalBuiltinConfig, fmt.Errorf("failed finding jdtls base path - %w", err)
	}

	sharedConfigPath, err := getSharedConfigPath(jdtlsBasePath)
	if err != nil {
		cancelFunc()
		return nil, additionalBuiltinConfig, fmt.Errorf("failed to get shared config path - %w", err)
	}

	jarPath, err := findEquinoxLauncher(jdtlsBasePath)
	if err != nil {
		cancelFunc()
		return nil, additionalBuiltinConfig, fmt.Errorf("failed to find equinox launcher - %w", err)
	}

	javaExec, err := getJavaExecutable(true)
	if err != nil {
		cancelFunc()
		return nil, additionalBuiltinConfig, fmt.Errorf("failed getting java executable - %v", err)
	}

	jdtlsArgs := []string{
		"-Declipse.application=org.eclipse.jdt.ls.core.id1",
		"-Dosgi.bundles.defaultStartLevel=4",
		"-Declipse.product=org.eclipse.jdt.ls.core.product",
		"-Dosgi.checkConfiguration=true",
		fmt.Sprintf("-Dosgi.sharedConfiguration.area=%s", sharedConfigPath),
		"-Dosgi.sharedConfiguration.area.readOnly=true",
		//"-agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=*:1044",
		"-Dosgi.configuration.cascaded=true",
		"-Xms1g",
		"-XX:MaxRAMPercentage=70.0",
		"--add-modules=ALL-SYSTEM",
		"--add-opens", "java.base/java.util=ALL-UNNAMED",
		"--add-opens", "java.base/java.lang=ALL-UNNAMED",
		"-jar", jarPath,
		"-Djava.net.useSystemProxies=true",
		"-configuration", "./",
		"-data", workspace,
	}

	if val, ok := config.ProviderSpecificConfig[JVM_MAX_MEM_INIT_OPTION].(string); ok && val != "" {
		jdtlsArgs = append(jdtlsArgs, fmt.Sprintf("-Xmx%s", val))
	}
	cmd := exec.CommandContext(ctx, javaExec, jdtlsArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancelFunc()
		return nil, additionalBuiltinConfig, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelFunc()
		return nil, additionalBuiltinConfig, err
	}

	waitErrorChannel := make(chan error)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := cmd.Start()
		wg.Done()
		if err != nil {
			cancelFunc()
			returnErr = err
			log.Error(err, "unable to  start lsp command")
			return
		}
		// Here we need to wait for the command to finish or if the ctx is cancelled,
		// To close the pipes.
		select {
		case err := <-waitErrorChannel:
			// language server has not started - don't error yet
			if err != nil && cmd.ProcessState == nil {
				log.Info("retrying language server start")
			} else {
				log.Error(err, "language server stopped with error")
			}
			log.V(5).Info("language server stopped")
		case <-ctx.Done():
			log.Info("language server context cancelled closing pipes")
			stdin.Close()
			stdout.Close()
		}
	}()

	// This will close the go routine above when wait has completed.
	go func() {
		waitErrorChannel <- cmd.Wait()
	}()

	wg.Wait()

	rpc := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(stdout, stdin), log)

	rpc.AddHandler(jsonrpc2.NewBackoffHandler(log))

	go func() {
		err := rpc.Run(ctx)
		if err != nil {
			//TODO: we need to pipe the ctx further into the stream header and run.
			// basically it is checking if done, then reading. When it gets EOF it errors.
			// We need the read to be at the same level of selection to fully implment graceful shutdown
			cancelFunc()
			returnErr = err
			return
		}
	}()

	m2Repo := getMavenLocalRepoPath(mavenSettingsFile)

	mavenIndexPath := ""
	if val, ok := config.ProviderSpecificConfig[providerSpecificConfigOpenSourceDepListKey]; ok {
		if strVal, ok := val.(string); ok {
			mavenIndexPath = strVal
		}
	}

	svcClient := javaServiceClient{
		rpc:               rpc,
		cancelFunc:        cancelFunc,
		config:            config,
		cmd:               cmd,
		bundles:           bundles,
		workspace:         workspace,
		log:               log,
		depToLabels:       map[string]*depLabelItem{},
		isLocationBinary:  isBinary,
		mvnInsecure:       mavenInsecure,
		mvnSettingsFile:   mavenSettingsFile,
		mvnLocalRepo:      m2Repo,
		mvnIndexPath:      mavenIndexPath,
		globalSettings:    globalSettingsFile,
		depsLocationCache: make(map[string]int),
		includedPaths:     provider.GetIncludedPathsFromConfig(config, false),
		cleanExplodedBins: explodedBins,
	}

	if mode == provider.FullAnalysisMode {
		// we attempt to decompile JARs of dependencies that don't have a sources JAR attached
		// we need to do this for jdtls to correctly recognize source attachment for dep
		switch svcClient.GetBuildTool() {
		case maven:
			err := resolveSourcesJarsForMaven(ctx, log, fernflower, config.Location, mavenSettingsFile, m2Repo, mavenInsecure)
			if err != nil {
				// TODO (pgaikwad): should we ignore this failure?
				log.Error(err, "failed to resolve maven sources jar for location", "location", config.Location)
			}
		case gradle:
			err = resolveSourcesJarsForGradle(ctx, log, fernflower, config.Location, mavenSettingsFile, &svcClient)
			if err != nil {
				log.Error(err, "failed to resolve gradle sources jar for location", "location", config.Location)
			}
		}

	}

	svcClient.initialization(ctx)
	err = svcClient.depInit()
	if err != nil {
		cancelFunc()
		return nil, provider.InitConfig{}, err
	}
	// Will only set up log follow one time
	// Will work in container image and hub, will not work
	// When running for long period of time.
	p.logFollow.Do(func() {
		go func() {
			t, err := tail.TailFile(".metadata/.log", tail.Config{
				ReOpen:    true,
				MustExist: false,
				Follow:    true,
				Logger:    tail.DiscardingLogger,
			})
			if err != nil {
				log.Error(err, "unable to set up follower")
				return
			}

			for line := range t.Lines {
				if strings.Contains(line.Text, "KONVEYOR_LOG") {
					log.Info("language server log", "line", line.Text)
				}
			}
		}()
	})
	return &svcClient, additionalBuiltinConfig, returnErr
}

func resolveSourcesJarsForGradle(ctx context.Context, log logr.Logger, fernflower, location string, _ string, svc *javaServiceClient) error {
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

	log.V(5).Info("resolving dependency sources for gradle")

	gb := svc.findGradleBuild()
	if gb == "" {
		return fmt.Errorf("could not find gradle build file for project")
	}

	// create a temporary build file to append the task for downloading sources
	taskgb := filepath.Join(filepath.Dir(gb), "tmp.gradle")
	err := CopyFile(gb, taskgb)
	if err != nil {
		return fmt.Errorf("error copying file %s to %s", gb, taskgb)
	}
	defer os.Remove(taskgb)

	// append downloader task
	taskfile := "/root/.gradle/task.gradle"
	err = AppendToFile(taskfile, taskgb)
	if err != nil {
		return fmt.Errorf("error appending file %s to %s", taskfile, taskgb)
	}

	tmpgbname := filepath.Join(location, "toberenamed.gradle")
	err = os.Rename(gb, tmpgbname)
	if err != nil {
		return fmt.Errorf("error renaming file %s to %s", gb, "toberenamed.gradle")
	}
	defer os.Rename(tmpgbname, gb)

	err = os.Rename(taskgb, gb)
	if err != nil {
		return fmt.Errorf("error renaming file %s to %s", gb, "toberenamed.gradle")
	}
	defer os.Remove(gb)

	// run gradle wrapper with tmp build file
	exe, err := filepath.Abs(filepath.Join(svc.config.Location, "gradlew"))
	if err != nil {
		return fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("a gradle wrapper must be present in the project")
	}

	// gradle must run with java 8 (see compatibility matrix)
	java8home := os.Getenv("JAVA8_HOME")
	if java8home == "" {
		return fmt.Errorf("")
	}

	args := []string{
		"konveyorDownloadSources",
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", java8home))
	cmd.Dir = location
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	log.V(8).WithValues("output", output).Info("got gradle output")

	// TODO: what if all sources available
	reader := bytes.NewReader(output)
	unresolvedSources, err := parseUnresolvedSourcesForGradle(reader)
	if err != nil {
		return err
	}

	log.V(5).Info("total unresolved sources", "count", len(unresolvedSources))

	decompileJobs := []decompileJob{}
	if len(unresolvedSources) > 1 {
		// Gradle cache dir structure changes over time - we need to find where the actual dependencies are stored
		cache, err := findGradleCache(unresolvedSources[0].GroupId)
		if err != nil {
			return err
		}

		for _, artifact := range unresolvedSources {
			log.V(5).WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

			artifactDir := filepath.Join(cache, artifact.GroupId, artifact.ArtifactId)
			jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
			artifactPath, err := findGradleArtifact(artifactDir, jarName)
			if err != nil {
				return err
			}
			decompileJobs = append(decompileJobs, decompileJob{
				artifact:   artifact,
				inputPath:  artifactPath,
				outputPath: filepath.Join(filepath.Dir(artifactPath), "decompiled", jarName),
			})
		}
		err = decompile(ctx, log, alwaysDecompileFilter(true), 10, decompileJobs, fernflower, "")
		if err != nil {
			return err
		}
		// move decompiled files to base location of the jar
		for _, decompileJob := range decompileJobs {
			jarName := strings.TrimSuffix(filepath.Base(decompileJob.inputPath), ".jar")
			err = moveFile(decompileJob.outputPath,
				filepath.Join(filepath.Dir(decompileJob.inputPath),
					fmt.Sprintf("%s-sources.jar", jarName)))
			if err != nil {
				log.V(5).Error(err, "failed to move decompiled file", "file", decompileJob.outputPath)
			}
		}

	}
	return nil
}

// findGradleCache looks for the folder within the Gradle cache where the actual dependencies are stored
// by walking the cache directory looking for a directory equal to the given sample group id
func findGradleCache(sampleGroupId string) (string, error) {
	// TODO(jmle): atm taking for granted that the cache is going to be here
	root := "/root/.gradle/caches"
	cache := ""
	walker := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("found error looking for cache directory: %w", err)
		}
		if d.IsDir() && d.Name() == sampleGroupId {
			cache = path
			return filepath.SkipAll
		}
		return nil
	}
	err := filepath.WalkDir(root, walker)
	if err != nil {
		return "", err
	}
	cache = filepath.Dir(cache) // return the parent of the found directory
	return cache, nil
}

// findGradleArtifact looks for a given artifact jar within the given root dir
func findGradleArtifact(root string, artifactId string) (string, error) {
	artifactPath := ""
	walker := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("found error looking for artifact: %w", err)
		}
		if !d.IsDir() && d.Name() == artifactId {
			artifactPath = path
			return filepath.SkipAll
		}
		return nil
	}
	err := filepath.WalkDir(root, walker)
	if err != nil {
		return "", err
	}
	return artifactPath, nil
}

// GetLocation given a dep, attempts to find line number, caches the line number for a given dep
func (j *javaProvider) GetLocation(ctx context.Context, dep konveyor.Dep, file string) (engine.Location, error) {
	location := engine.Location{StartPosition: engine.Position{}, EndPosition: engine.Position{}}

	cacheKey := fmt.Sprintf("%s-%s-%s-%v",
		dep.Name, dep.Version, dep.ResolvedIdentifier, dep.Indirect)
	j.depsMutex.RLock()
	val, exists := j.depsLocationCache[cacheKey]
	j.depsMutex.RUnlock()
	if exists {
		if val == -1 {
			return location,
				fmt.Errorf("unable to get location for dep %s due to a previous error", dep.Name)
		}
		return engine.Location{
			StartPosition: engine.Position{
				Line: val,
			},
			EndPosition: engine.Position{
				Line: val,
			},
		}, nil
	}

	defer func() {
		j.depsMutex.Lock()
		j.depsLocationCache[cacheKey] = location.StartPosition.Line
		j.depsMutex.Unlock()
	}()

	location.StartPosition.Line = -1
	// we know that this provider populates extras with required information
	if dep.Extras == nil {
		return location, fmt.Errorf("unable to get location for dep %s, dep.Extras not set", dep.Name)
	}
	extrasKeys := []string{artifactIdKey, groupIdKey, pomPathKey}
	for _, key := range extrasKeys {
		if val, ok := dep.Extras[key]; !ok {
			return location,
				fmt.Errorf("unable to get location for dep %s, missing dep.Extras key %s", dep.Name, key)
		} else if _, ok := val.(string); !ok {
			return location,
				fmt.Errorf("unable to get location for dep %s, dep.Extras key %s not a string", dep.Name, key)
		}
	}

	groupId := dep.Extras[groupIdKey].(string)
	artifactId := dep.Extras[artifactIdKey].(string)
	path := dep.Extras[pomPathKey].(string)
	if path == "" {
		path = file
	}
	if path == "" {
		return location, fmt.Errorf("unable to get location for dep %s, empty pom path", dep.Name)
	}
	lineNumber, err := provider.MultilineGrep(ctx, 2, path,
		fmt.Sprintf("(<groupId>%s</groupId>|<artifactId>%s</artifactId>).*?(<artifactId>%s</artifactId>|<groupId>%s</groupId>).*",
			groupId, artifactId, artifactId, groupId))
	if err != nil || lineNumber == -1 {
		return location, fmt.Errorf("unable to get location for dep %s, search error - %w", dep.Name, err)
	}
	location.StartPosition.Line = lineNumber
	location.EndPosition.Line = lineNumber
	return location, nil
}

// resolveSourcesJarsForMaven for a given source code location, runs maven to find
// deps that don't have sources attached and decompiles them
func resolveSourcesJarsForMaven(ctx context.Context, log logr.Logger, fernflower, location, mavenSettings, mavenLocalRepo string, mvnInsecure bool) error {
	// TODO (pgaikwad): when we move to external provider, inherit context from parent
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

	if mavenLocalRepo == "" {
		log.V(5).Info("unable to discover dependency sources as maven local repo path is unknown")
		return nil
	}

	decompileJobs := []decompileJob{}

	log.Info("resolving dependency sources")

	args := []string{
		"-B",
		"de.qaware.maven:go-offline-maven-plugin:resolve-dependencies",
		"-DdownloadSources",
		"-Djava.net.useSystemProxies=true",
	}
	if mavenSettings != "" {
		args = append(args, "-s", mavenSettings)
	}
	if mvnInsecure {
		args = append(args, "-Dmaven.wagon.http.ssl.insecure=true")
	}
	cmd := exec.CommandContext(ctx, "mvn", args...)
	cmd.Dir = location
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("maven downloadSources command failed with error %w, maven output: %s", err, string(mvnOutput))
	}

	reader := bytes.NewReader(mvnOutput)
	artifacts, err := parseUnresolvedSources(reader)
	if err != nil {
		return err
	}

	for _, artifact := range artifacts {
		log.WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

		groupDirs := filepath.Join(strings.Split(artifact.GroupId, ".")...)
		artifactDirs := filepath.Join(strings.Split(artifact.ArtifactId, ".")...)
		jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
		decompileJobs = append(decompileJobs, decompileJob{
			artifact: artifact,
			inputPath: filepath.Join(
				mavenLocalRepo, groupDirs, artifactDirs, artifact.Version, jarName),
			outputPath: filepath.Join(
				mavenLocalRepo, groupDirs, artifactDirs, artifact.Version, "decompiled", jarName),
		})
	}
	err = decompile(ctx, log, alwaysDecompileFilter(true), 10, decompileJobs, fernflower, "")
	if err != nil {
		return err
	}
	// move decompiled files to base location of the jar
	for _, decompileJob := range decompileJobs {
		jarName := strings.TrimSuffix(filepath.Base(decompileJob.inputPath), ".jar")
		err = moveFile(decompileJob.outputPath,
			filepath.Join(filepath.Dir(decompileJob.inputPath),
				fmt.Sprintf("%s-sources.jar", jarName)))
		if err != nil {
			log.Error(err, "failed to move decompiled file", "file", decompileJob.outputPath)
		}
	}
	return nil
}

// parseUnresolvedSources takes the output from the download sources gradle task and returns the artifacts whose sources
// could not be found. Sample gradle output:
// Found 0 sources for :simple-jar:
// Found 1 sources for com.codevineyard:hello-world:1.0.1
// Found 1 sources for org.codehaus.groovy:groovy:3.0.21
func parseUnresolvedSourcesForGradle(output io.Reader) ([]javaArtifact, error) {
	unresolvedSources := []javaArtifact{}
	unresolvedRegex := regexp.MustCompile(`Found 0 sources for (.*)`)
	artifactRegex := regexp.MustCompile(`(.+):(.+):(.+)|:(.+):`)

	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Text()

		if match := unresolvedRegex.FindStringSubmatch(line); len(match) != 0 {
			gav := artifactRegex.FindStringSubmatch(match[1])
			if gav[4] != "" { // internal library, unknown group/version
				artifact := javaArtifact{
					ArtifactId: match[4],
				}
				unresolvedSources = append(unresolvedSources, artifact)
			} else { // external dependency
				artifact := javaArtifact{
					GroupId:    gav[1],
					ArtifactId: gav[2],
					Version:    gav[3],
				}
				unresolvedSources = append(unresolvedSources, artifact)
			}
		}
	}

	// dedup artifacts
	result := []javaArtifact{}
	for _, artifact := range unresolvedSources {
		if contains(result, artifact) {
			continue
		}
		result = append(result, artifact)
	}

	return result, scanner.Err()
}

// parseUnresolvedSources takes the output from the go-offline maven plugin and returns the artifacts whose sources
// could not be found.
func parseUnresolvedSources(output io.Reader) ([]javaArtifact, error) {
	unresolvedSources := []javaArtifact{}
	unresolvedArtifacts := []javaArtifact{}

	scanner := bufio.NewScanner(output)

	unresolvedRegex := regexp.MustCompile(`\[WARNING] The following artifacts could not be resolved`)
	artifactRegex := regexp.MustCompile(`([\w\.]+):([\w\-]+):\w+:([\w\.]+):?([\w\.]+)?`)

	for scanner.Scan() {
		line := scanner.Text()

		if unresolvedRegex.Find([]byte(line)) != nil {
			gavs := artifactRegex.FindAllStringSubmatch(line, -1)
			for _, gav := range gavs {
				// dependency jar (not sources) also not found
				if len(gav) == 5 && gav[3] != "sources" {
					artifact := javaArtifact{
						packaging:  JavaArchive,
						GroupId:    gav[1],
						ArtifactId: gav[2],
						Version:    gav[3],
					}
					unresolvedArtifacts = append(unresolvedArtifacts, artifact)
					continue
				}

				var v string
				if len(gav) == 4 {
					v = gav[3]
				} else {
					v = gav[4]
				}
				artifact := javaArtifact{
					packaging:  JavaArchive,
					GroupId:    gav[1],
					ArtifactId: gav[2],
					Version:    v,
				}

				unresolvedSources = append(unresolvedSources, artifact)
			}
		}
	}

	// if we don't have the dependency itself available, we can't even decompile
	result := []javaArtifact{}
	for _, artifact := range unresolvedSources {
		if contains(unresolvedArtifacts, artifact) || contains(result, artifact) {
			continue
		}
		result = append(result, artifact)
	}

	return result, scanner.Err()
}

func contains(artifacts []javaArtifact, artifactToFind javaArtifact) bool {
	if len(artifacts) == 0 {
		return false
	}

	for _, artifact := range artifacts {
		if artifact == artifactToFind {
			return true
		}
	}

	return false
}

func (p *javaProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.FullResponseFromServiceClients(ctx, p.clients, cap, conditionInfo)
}

func (p *javaProvider) ProviderInit(ctx context.Context, additionalConfigs []provider.InitConfig) ([]provider.InitConfig, error) {
	builtinConfs := []provider.InitConfig{}
	if additionalConfigs != nil {
		p.config.InitConfig = append(p.config.InitConfig, additionalConfigs...)
	}
	for _, c := range p.config.InitConfig {
		client, builtinConf, err := p.Init(ctx, p.Log, c)
		if err != nil {
			return nil, err
		}
		p.clients = append(p.clients, client)
		if builtinConf.Location != "" {
			builtinConfs = append(builtinConfs, builtinConf)
		}
	}
	return builtinConfs, nil
}

func (p *javaProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return provider.FullDepsResponse(ctx, p.clients)
}

func (p *javaProvider) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return provider.FullDepDAGResponse(ctx, p.clients)
}

func (p *javaProvider) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	return provider.FullNotifyFileChangesResponse(ctx, p.clients, changes...)
}

func (p *javaProvider) BuildSettingsFile(m2CacheDir string) (settingsFile string, err error) {
	fileContentTemplate := `
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0 https://maven.apache.org/xsd/settings-1.0.0.xsd">
  <localRepository>%v</localRepository>
</settings>
	`
	var homeDir string
	set := true
	ops := runtime.GOOS
	if ops == "linux" {
		homeDir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || homeDir == "" || !set {
		// on Unix, including macOS, this returns the $HOME environment variable. On Windows, it returns %USERPROFILE%
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	settingsFilePath := filepath.Join(homeDir, ".analyze", "globalSettings.xml")
	err = os.Mkdir(filepath.Join(homeDir, ".analyze"), 0777)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	f, err := os.Create(settingsFilePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()
	err = os.Chmod(settingsFilePath, 0777)
	if err != nil {
		return "", err
	}
	_, err = f.Write([]byte(fmt.Sprintf(fileContentTemplate, m2CacheDir)))
	if err != nil {
		return "", err
	}

	return settingsFilePath, nil
}

func getJavaExecutable(validateJavaVersion bool) (string, error) {
	javaExecutable := "java"
	if javaHome, exists := os.LookupEnv("JAVA_HOME"); exists {
		javaExecToTest := filepath.Join(javaHome, "bin", "java")
		if runtime.GOOS == "windows" {
			javaExecToTest += ".exe"
		}
		if _, err := os.Stat(javaExecToTest); err == nil {
			javaExecutable = javaExecToTest
		}
	}

	if !validateJavaVersion {
		return javaExecutable, nil
	}

	out, err := exec.Command(javaExecutable, "-version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %s -version: %w", javaExecutable, err)
	}

	re := regexp.MustCompile(`version\s"(\d+)[.\d]*"`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) > 1 {
		javaVersion := matches[1]
		if majorVersion := javaVersion; majorVersion < "17" {
			return "", errors.New("jdtls requires at least Java 17")
		}
		return javaExecutable, nil
	}

	return "", errors.New("could not determine Java version")
}

func findEquinoxLauncher(jdtlsBaseDir string) (string, error) {
	pluginsDir := filepath.Join(jdtlsBaseDir, "plugins")
	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "org.eclipse.equinox.launcher_") && strings.HasSuffix(file.Name(), ".jar") {
			return filepath.Join(pluginsDir, file.Name()), nil
		}
	}

	return "", errors.New("cannot find equinox launcher")
}

func getSharedConfigPath(jdtlsBaseDir string) (string, error) {
	var configDir string
	switch runtime.GOOS {
	case "linux", "freebsd":
		configDir = "config_linux"
	case "darwin":
		configDir = "config_mac"
	case "windows":
		configDir = "config_win"
	default:
		return "", fmt.Errorf("unknown platform %s detected", runtime.GOOS)
	}
	return filepath.Join(jdtlsBaseDir, configDir), nil
}
