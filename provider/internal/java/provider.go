package java

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	JavaFile          = ".java"
	JavaArchive       = ".jar"
	WebArchive        = ".war"
	EnterpriseArchive = ".ear"
	ClassFile         = ".class"
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
}

type javaProvider struct {
	config provider.Config
	Log    logr.Logger

	clients []provider.ServiceClient

	hasMaven          bool
	depsMutex         sync.RWMutex
	depsLocationCache map[string]int
}

var _ provider.InternalProviderClient = &javaProvider{}

var _ provider.DependencyLocationResolver = &javaProvider{}

type javaCondition struct {
	Referenced referenceCondition `yaml:"referenced"`
}

type referenceCondition struct {
	Pattern  string `yaml:"pattern"`
	Location string `yaml:"location"`
}

func NewJavaProvider(config provider.Config, log logr.Logger) *javaProvider {

	_, mvnBinaryError := exec.LookPath("mvn")

	return &javaProvider{
		config:            config,
		hasMaven:          mvnBinaryError == nil,
		Log:               log,
		clients:           []provider.ServiceClient{},
		depsLocationCache: make(map[string]int),
	}
}

func (p *javaProvider) Stop() {
	// Ignore the error here, it stopped and we wanted it to.
	for _, c := range p.clients {
		c.Stop()
	}
}

func (p *javaProvider) Capabilities() []provider.Capability {
	caps := []provider.Capability{
		{
			Name:            "referenced",
			TemplateContext: openapi3.SchemaRef{},
		},
	}
	if p.hasMaven {
		caps = append(caps, provider.Capability{
			Name:            "dependency",
			TemplateContext: openapi3.SchemaRef{},
		})
	}
	return caps
}

func (p *javaProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.FullResponseFromServiceClients(ctx, p.clients, cap, conditionInfo)
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

func (p *javaProvider) ProviderInit(ctx context.Context) error {
	for _, c := range p.config.InitConfig {
		client, err := p.Init(ctx, p.Log, c)
		if err != nil {
			return err
		}
		p.clients = append(p.clients, client)
	}
	return nil
}

func (p *javaProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, error) {
	// By default, if nothing is set for analysis mode in the config, we should default to full for external providers
	var mode provider.AnalysisMode = provider.AnalysisMode(config.AnalysisMode)
	if mode == provider.AnalysisMode("") {
		mode = provider.FullAnalysisMode
	} else if !(mode == provider.FullAnalysisMode || mode == provider.SourceOnlyAnalysisMode) {
		return nil, fmt.Errorf("invalid Analysis Mode")
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
		return nil, fmt.Errorf("invalid lspServerPath provided, unable to init java provider")
	}

	isBinary := false
	var returnErr error
	// each service client should have their own context
	ctx, cancelFunc := context.WithCancel(ctx)
	extension := strings.ToLower(path.Ext(config.Location))
	switch extension {
	case JavaArchive, WebArchive, EnterpriseArchive:
		depLocation, sourceLocation, err := decompileJava(ctx, log, config.Location)
		if err != nil {
			cancelFunc()
			return nil, err
		}
		config.Location = sourceLocation
		// for binaries, we fallback to looking at .jar files only for deps
		config.DependencyPath = depLocation
		isBinary = true
	}

	if mode == provider.FullAnalysisMode {
		// we attempt to decompile JARs of dependencies that don't have a sources JAR attached
		// we need to do this for jdtls to correctly recognize source attachment for dep
		err := resolveSourcesJars(ctx, log, config.Location, mavenSettingsFile)
		if err != nil {
			// TODO (pgaikwad): should we ignore this failure?
			log.Error(err, "failed to resolve sources jar for location", "location", config.Location)
		}
	}

	// handle proxy settings
	for k, v := range config.Proxy.ToEnvVars() {
		os.Setenv(k, v)
	}

	args := []string{
		"-Djava.net.useSystemProxies=true",
		"-configuration",
		"./",
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
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelFunc()
		return nil, err
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
	}

	svcClient.initialization(ctx)
	err = svcClient.depInit()
	if err != nil {
		return nil, err
	}
	return &svcClient, returnErr
}

func (p *javaProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return provider.FullDepsResponse(ctx, p.clients)
}

func (p *javaProvider) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return provider.FullDepDAGResponse(ctx, p.clients)
}

// GetLocation given a dep, attempts to find line number, caches the line number for a given dep
func (j *javaProvider) GetLocation(ctx context.Context, dep konveyor.Dep) (engine.Location, error) {
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

// resolveSourcesJars for a given source code location, runs maven to find
// deps that don't have sources attached and decompiles them
func resolveSourcesJars(ctx context.Context, log logr.Logger, location, mavenSettings string) error {
	decompileJobs := []decompileJob{}

	log.V(5).Info("resolving dependency sources")

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

	log.V(8).WithValues("output", mvnOutput).Info("got maven output")

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
		log.V(5).WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

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
			log.V(5).Error(err, "failed to move decompiled file", "file", decompileJob.outputPath)
		}
	}
	return nil
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
