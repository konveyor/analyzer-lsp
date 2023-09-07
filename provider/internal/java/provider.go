package java

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
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

func (p *javaProvider) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.FullResponseFromServiceClients(p.clients, cap, conditionInfo)
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
		// for binaries, always run in source-only mode as we don't know how to correctly resolve deps
		config.AnalysisMode = provider.SourceOnlyAnalysisMode
		isBinary = true
	}

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
		cancelFunc()
		return nil, fmt.Errorf("invalid lspServerPath provided, unable to init java provider")
	}

	// handle proxy settings
	for k, v := range config.Proxy.ToEnvVars() {
		os.Setenv(k, v)
	}

	cmd := exec.CommandContext(ctx, lspServerPath,
		"-Djava.net.useSystemProxies=true",
		"-configuration",
		"./",
		"-data",
		workspace,
	)
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
		ctx:              ctx,
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

	svcClient.initialization()
	err = svcClient.depInit()
	if err != nil {
		return nil, err
	}
	return &svcClient, returnErr
}

func (p *javaProvider) GetDependencies() (map[uri.URI][]*provider.Dep, error) {
	return provider.FullDepsResponse(p.clients)
}

func (p *javaProvider) GetDependenciesDAG() (map[uri.URI][]provider.DepDAGItem, error) {
	return provider.FullDepDAGResponse(p.clients)
}
