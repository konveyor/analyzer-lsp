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

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/vifraa/gopom"
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

	hasMaven bool
}

var _ provider.InternalProviderClient = &javaProvider{}

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
		config:   config,
		hasMaven: mvnBinaryError == nil,
		Log:      log,
		clients:  []provider.ServiceClient{},
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
	//By default if nothing is set for analysis mode, in the config, we should default to full for external providers
	var a provider.AnalysisMode = provider.AnalysisMode(config.AnalysisMode)
	if a == provider.AnalysisMode("") {
		a = provider.FullAnalysisMode
	} else if !(a == provider.FullAnalysisMode || a == provider.SourceOnlyAnalysisMode) {
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

	// we attempt to decompile JARs of dependencies that don't have a sources JAR attached
	// we need to do this for jdtls to correctly recognize source attachment for dep
	err := resolveSourcesJars(ctx, log, config.Location, mavenSettingsFile)
	if err != nil {
		// TODO (pgaikwad): should we ignore this failure?
		log.Error(err, "failed to resolve sources jar for location", "location", config.Location)
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
		rpc:              rpc,
		cancelFunc:       cancelFunc,
		config:           config,
		cmd:              cmd,
		bundles:          bundles,
		workspace:        workspace,
		log:              log,
		depToLabels:      map[string]*depLabelItem{},
		isLocationBinary: isBinary,
		mvnSettingsFile:  mavenSettingsFile,
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

// resolveSourcesJars for a given source code location, runs maven to find
// deps that don't have sources attached and decompiles them
func resolveSourcesJars(ctx context.Context, log logr.Logger, location, mavenSettings string) error {
	decompileJobs := []decompileJob{}

	log.V(5).Info("resolving dependency sources")

	pomPath := fmt.Sprintf("%s/pom.xml", location)
	pom, err := gopom.Parse(pomPath)
	if err != nil {
		return err
	}

	args := []string{
		"dependency:sources",
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

	// remove unresolved sources if they are an actual module in the project
	artifacts = filterExistingSubmodules(artifacts, pom)

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
		moveFile(decompileJob.outputPath,
			filepath.Join(filepath.Dir(decompileJob.inputPath),
				fmt.Sprintf("%s-sources.jar", jarName)))
	}
	return nil
}

// filterExistingSubmodules takes a list of artifacts and removes the ones existing in the given pom's modules list
func filterExistingSubmodules(artifacts []javaArtifact, pom *gopom.Project) []javaArtifact {
	if pom.Modules == nil {
		return artifacts
	}

	var filtered []javaArtifact
	for _, artifact := range artifacts {
		found := false
		for _, module := range *pom.Modules {
			if artifact.ArtifactId == module {
				found = true
				break
			}
		}
		if !found {
			filtered = append(filtered, artifact)
		}
	}

	return filtered
}

func parseUnresolvedSources(output io.Reader) ([]javaArtifact, error) {
	artifacts := []javaArtifact{}
	scanner := bufio.NewScanner(output)

	sourcesPluginSeparatorSeen := false
	unresolvedSeparatorSeen := false
	sourcesPluginRegex := regexp.MustCompile(`maven-dependency-plugin:[\d\.]+:sources`)
	unresolvedRegex := regexp.MustCompile(`The following files have NOT been resolved`)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimLeft(line, "[INFO] ")

		if sourcesPluginRegex.Find([]byte(line)) != nil {
			sourcesPluginSeparatorSeen = true
			unresolvedSeparatorSeen = false
		} else if unresolvedRegex.Find([]byte(line)) != nil {
			unresolvedSeparatorSeen = true
		} else if sourcesPluginSeparatorSeen && unresolvedSeparatorSeen {
			line, _, _ = strings.Cut(line, "--") // ie: "org.apache.derby:derby:jar:10.14.2.0:test -- module derby (auto)"

			parts := strings.Split(line, ":")
			if len(parts) != 6 && len(parts) != 5 {
				continue
			}

			// sometimes maven puts here artifacts that are not sources; don't count those as unresolved (ie: spring-petclinic project)
			if len(parts) == 6 {
				classifier := parts[3]
				if classifier != "sources" {
					continue
				}
			} else if len(parts) == 5 {
				classifier := parts[2]
				if classifier != "sources" {
					continue
				}
			}

			var version string
			groupId := parts[0]
			artifactId := parts[1]
			if len(parts) == 6 {
				version = parts[4]
			} else {
				version = parts[3]
			}

			artifacts = append(artifacts,
				javaArtifact{
					packaging:  JavaArchive,
					ArtifactId: artifactId,
					GroupId:    groupId,
					Version:    version,
				})
		}
	}
	return artifacts, scanner.Err()
}
