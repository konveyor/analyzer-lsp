package java

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/bldtool"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/nxadm/tail"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
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
	DISABLE_MAVEN_SEARCH          = "disableMavenSearch"
	GRADLE_SOURCES_TASK_FILE      = "gradleSourcesTaskFile"
	MAVEN_INDEX_PATH              = "mavenIndexPath"
)

const (
	artifactIdKey = "artifactId"
	groupIdKey    = "groupId"
	pomPathKey    = "pomPath"
	baseDepKey    = "baseDep"
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
	encoding     string

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
		encoding:          "",
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

	p.encoding = provider.GetEncodingFromConfig(config)
	log = log.WithValues("provider", "java").WithValues("analysis-mode", mode).WithValues("project", config.Location)

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

	gradleTaskFile, ok := config.ProviderSpecificConfig[GRADLE_SOURCES_TASK_FILE].(string)
	if !ok {
		gradleTaskFile = ""
	}

	mavenIndexPath, ok := config.ProviderSpecificConfig[MAVEN_INDEX_PATH].(string)
	if !ok {
		log.Info("unable to find the maven index path in the provider specific config")
	}

	// each service client should have their own context
	downloadCtx, cancelFunc := context.WithCancel(ctx)
	// location can be a coordinate to a remote mvn artifact
	if downloader, ok := bldtool.GetDownloader(config.Location, mavenSettingsFile, mavenInsecure, log); ok {
		downloadPath, err := downloader.Download(downloadCtx)
		if err != nil {
			cancelFunc()
			return nil, additionalBuiltinConfig, err
		}
		config.Location = downloadPath
	}
	cancelFunc()

	openSourceLabeler, err := labels.GetOpenSourceLabeler(config.ProviderSpecificConfig, log)
	if err != nil {
		log.V(5).Error(err, "failed to initialize dep labels lookup for open source packages")
		cancelFunc()
		return nil, provider.InitConfig{}, err
	}

	/// Full Analysis Mode OR binary analysis should kick of the resolve sources.
	// TODO: handle Continue Errors vs Non Continue Errors in bldtool
	buildTool := bldtool.GetBuildTool(bldtool.BuildToolOptions{
		Config:          config,
		MvnSettingsFile: mavenSettingsFile,
		MvnInsecure:     mavenInsecure,
		MavenIndexPath:  mavenIndexPath,
		Labeler:         openSourceLabeler,
		GradleTaskFile:  gradleTaskFile,
	}, log)
	if buildTool == nil {
		return nil, additionalBuiltinConfig, errors.New("unable to get build tool")
	}

	if buildTool.ShouldResolve() || mode == provider.FullAnalysisMode {
		log.Info("Resolving project", "location", config.Location)
		resolver, err := buildTool.GetResolver(fernflower)
		if err != nil {
			log.Error(err, "unable to resolve")
			return nil, additionalBuiltinConfig, err
		}
		location, depLocation, err := resolver.ResolveSources(ctx)
		if err != nil {
			log.Error(err, "unable to resolve")
			return nil, additionalBuiltinConfig, err
		}
		config.Location = location
		config.DependencyPath = depLocation
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

	var returnErr error
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
				log.Error(err, "language server error")
			} else if err != nil {
				log.Error(err, "language server process terminated")
			}
			log.Info("language server stopped")

		case <-ctx.Done():
			log.Info("language server context cancelled, closing pipes")
			stdin.Close()
			stdout.Close()
		}
	}()

	// This will close the go routine above when wait has completed.
	go func() {
		waitErrorChannel <- cmd.Wait()
	}()

	wg.Wait()

	// Create a shared input,ouput dialer
	dialer := base.NewStdDialer(stdin, stdout)

	rpc, err := jsonrpc2.Dial(ctx, dialer, jsonrpc2.ConnectionOptions{
		Handler: base.NewChainHandler(base.LogHandler(log)),
	})
	if err != nil {
		cancelFunc()
		log.Error(err, "unable to connect over new package")
		return nil, additionalBuiltinConfig, err
	}

	svcClient := javaServiceClient{
		rpc:               rpc,
		cancelFunc:        cancelFunc,
		config:            config,
		cmd:               cmd,
		bundles:           bundles,
		workspace:         workspace,
		log:               log,
		globalSettings:    globalSettingsFile,
		depsLocationCache: make(map[string]int),
		includedPaths:     provider.GetIncludedPathsFromConfig(config, false),
		buildTool:         buildTool,
		mvnIndexPath:      mavenIndexPath,
		mvnSettingsFile:   mavenSettingsFile,
	}

	svcClient.initialization(ctx)

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
	if returnErr != nil {
		return nil, additionalBuiltinConfig, err
	}
	return &svcClient, additionalBuiltinConfig, nil
}

// GetLocation given a dep, attempts to find line number, caches the line number for a given dep
func (j *javaProvider) GetLocation(ctx context.Context, dep konveyor.Dep, file string) (engine.Location, error) {
	j.Log.Info("getting dep location", "dep", dep, "file", file)
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
	extrasKeys := []string{artifactIdKey, groupIdKey}
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
	_, err = fmt.Fprint(f, fileContentTemplate, m2CacheDir)
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
