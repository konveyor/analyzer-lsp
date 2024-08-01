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
	JVM_MAX_MEM_INIT_OPTION       = "jvmMaxMem"
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
	"enum_constant":        6,
	"return_type":          7,
	"import":               8,
	"variable_declaration": 9,
	"type":                 10,
	"package":              11,
	"field":                12,
	"method":               13,
}

type javaProvider struct {
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

var _ provider.DependencyLocationResolver = &javaProvider{}

type javaCondition struct {
	Referenced referenceCondition `yaml:"referenced"`
}

type referenceCondition struct {
	Pattern   string    `yaml:"pattern"`
	Location  string    `yaml:"location"`
	Annotated annotated `yaml:"annotated,omitempty" json:"annotated,omitempty"`
}

type annotated struct {
	Pattern  string    `yaml:"pattern" json:"pattern"`
	Elements []element `yaml:"elements,omitempty" json:"elements,omitempty"`
}

type element struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"` // can be a (java) regex pattern
}

func NewJavaProvider(log logr.Logger, lspServerName string, contextLines int) *javaProvider {

	_, mvnBinaryError := exec.LookPath("mvn")

	return &javaProvider{
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

	lspServerPath, ok := config.ProviderSpecificConfig[provider.LspServerPathConfigKey].(string)
	if !ok || lspServerPath == "" {
		return nil, additionalBuiltinConfig, fmt.Errorf("invalid lspServerPath provided, unable to init java provider")
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
		log.Info("downloading maven artifact", "artifact", mvnCoordinates, "options", mvnOptions)
		cmd := exec.CommandContext(ctx, "mvn", mvnOptions...)
		cmd.Dir = outputDir
		mvnOutput, err := cmd.CombinedOutput()
		if err != nil {
			cancelFunc()
			return nil, additionalBuiltinConfig, fmt.Errorf("error downloading java artifact %s - %w", mvnUri, err)
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
	switch extension {
	case JavaArchive, WebArchive, EnterpriseArchive:
		depLocation, sourceLocation, err := decompileJava(ctx, log,
			config.Location, getMavenLocalRepoPath(mavenSettingsFile))
		if err != nil {
			cancelFunc()
			return nil, additionalBuiltinConfig, err
		}
		config.Location = sourceLocation
		// for binaries, we fallback to looking at .jar files only for deps
		config.DependencyPath = depLocation
		isBinary = true
	}
	additionalBuiltinConfig.Location = config.Location
	additionalBuiltinConfig.DependencyPath = config.DependencyPath

	// handle proxy settings
	for k, v := range config.Proxy.ToEnvVars() {
		os.Setenv(k, v)
	}

	args := []string{
		"-Djava.net.useSystemProxies=true",
		"-configuration",
		"./",
		//"--jvm-arg=-agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=*:1044",
		"-data",
		workspace,
	}
	if val, ok := config.ProviderSpecificConfig[JVM_MAX_MEM_INIT_OPTION].(string); ok && val != "" {
		args = append(args, fmt.Sprintf("-Xmx%s", val))
	}
	cmd := exec.CommandContext(ctx, lspServerPath, args...)
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

	go func() {
		err := cmd.Start()
		if err != nil {
			cancelFunc()
			returnErr = err
			log.Error(err, "unable to  start lsp command")
			return
		}
	}()
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
		mvnSettingsFile:   mavenSettingsFile,
		depsLocationCache: make(map[string]int),
		includedPaths:     provider.GetIncludedPathsFromConfig(config, false),
	}

	if mode == provider.FullAnalysisMode {
		// we attempt to decompile JARs of dependencies that don't have a sources JAR attached
		// we need to do this for jdtls to correctly recognize source attachment for dep
		switch svcClient.GetBuildTool() {
		case maven:
			err := resolveSourcesJarsForMaven(ctx, log, config.Location, mavenSettingsFile)
			if err != nil {
				// TODO (pgaikwad): should we ignore this failure?
				log.Error(err, "failed to resolve maven sources jar for location", "location", config.Location)
			}
		case gradle:
			err = resolveSourcesJarsForGradle(ctx, log, config.Location, mavenSettingsFile, &svcClient)
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

func resolveSourcesJarsForGradle(ctx context.Context, log logr.Logger, location string, _ string, svc *javaServiceClient) error {
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

	fmt.Printf("%d", len(unresolvedSources))

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
		err = decompile(ctx, log, alwaysDecompileFilter(true), 10, decompileJobs, "")
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
func resolveSourcesJarsForMaven(ctx context.Context, log logr.Logger, location, mavenSettings string) error {
	// TODO (pgaikwad): when we move to external provider, inherit context from parent
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

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
	cmd := exec.CommandContext(ctx, "mvn", args...)
	cmd.Dir = location
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	reader := bytes.NewReader(mvnOutput)
	artifacts, err := parseUnresolvedSources(reader)
	if err != nil {
		return err
	}

	m2Repo := getMavenLocalRepoPath(mavenSettings)
	if m2Repo == "" {
		return nil
	}
	for _, artifact := range artifacts {
		log.WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

		groupDirs := filepath.Join(strings.Split(artifact.GroupId, ".")...)
		artifactDirs := filepath.Join(strings.Split(artifact.ArtifactId, ".")...)
		jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
		decompileJobs = append(decompileJobs, decompileJob{
			artifact: artifact,
			inputPath: filepath.Join(
				m2Repo, groupDirs, artifactDirs, artifact.Version, jarName),
			outputPath: filepath.Join(
				m2Repo, groupDirs, artifactDirs, artifact.Version, "decompiled", jarName),
		})
	}
	err = decompile(ctx, log, alwaysDecompileFilter(true), 10, decompileJobs, "")
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
