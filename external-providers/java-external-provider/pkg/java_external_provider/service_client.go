package java

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/bldtool"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type javaServiceClient struct {
	rpc                provider.RPCClient
	cancelFunc         context.CancelFunc
	config             provider.InitConfig
	log                logr.Logger
	cmd                *exec.Cmd
	bundles            []string
	workspace          string
	isLocationBinary   bool
	globalSettings     string
	includedPaths      []string
	cleanExplodedBins  []string
	disableMavenSearch bool
	activeRPCCalls     sync.WaitGroup
	depsLocationCache  map[string]int
	buildTool          bldtool.BuildTool
	mvnIndexPath       string
	mvnSettingsFile    string
}

var _ provider.ServiceClient = &javaServiceClient{}

func (p *javaServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {

	cond := &javaCondition{}
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}
	// filepaths get rendered as a string and must be converted
	if len(cond.Referenced.Filepaths) > 0 {
		cond.Referenced.Filepaths = strings.Split(cond.Referenced.Filepaths[0], " ")
	}

	condCtx := &provider.ProviderContext{}
	err = yaml.Unmarshal(conditionInfo, condCtx)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get condition context info: %v", err)
	}

	if cond.Referenced.Pattern == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("provided query pattern empty")
	}
	symbols, err := p.GetAllSymbols(ctx, *cond, condCtx)
	if err != nil {
		p.log.Error(err, "unable to get symbols", "symbols", symbols, "cap", cap, "conditionInfo", cond)
		return provider.ProviderEvaluateResponse{}, err
	}
	p.log.Info("Symbols retrieved", "symbols", len(symbols), "cap", cap, "conditionInfo", cond)

	incidents := []provider.IncidentContext{}
	locationCode := LocationType(locationToCode[strings.ToLower(cond.Referenced.Location)])
	switch locationCode {
	case LocationTypeDefault, LocationConstructorCall, LocationAnnotation, LocationEnum, LocationTypeKeyword, LocationPackage, LocationField, LocationMethod, LocationClass:
		// Filter handle for type, find all the references to this type.
		incidents, err = p.filterDefault(symbols)
	case LocationInheritance, LocationImplementsType:
		incidents, err = p.filterTypesInheritance(symbols)
	case LocationMethodCall:
		incidents, err = p.filterMethodSymbols(symbols)
	case LocationReturnType:
		incidents, err = p.filterMethodSymbols(symbols)
	case LocationImport:
		incidents, err = p.filterModulesImports(symbols)
	case LocationVariableDeclaration:
		incidents, err = p.filterVariableDeclaration(symbols)
	default:

	}
	// push error up for easier printing.
	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	if len(incidents) == 0 {
		return provider.ProviderEvaluateResponse{
			Matched: false,
		}, nil
	}
	return provider.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (p *javaServiceClient) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	return nil
}

func (p *javaServiceClient) GetAllSymbols(ctx context.Context, c javaCondition, condCTX *provider.ProviderContext) ([]protocol.WorkspaceSymbol, error) {
	// This command will run the added bundle to the language server. The command over the wire needs too look like this.
	// in this case the project is hardcoded in the init of the Langauge Server above
	// workspace/executeCommand '{"command": "io.konveyor.tackle.ruleEntry", "arguments": {"query":"*customresourcedefinition","project": "java"}}'
	argumentsMap := map[string]any{
		"query":                      c.Referenced.Pattern,
		"project":                    "java",
		"location":                   fmt.Sprintf("%v", locationToCode[strings.ToLower(c.Referenced.Location)]),
		"analysisMode":               string(p.config.AnalysisMode),
		"includeOpenSourceLibraries": true,
		"mavenLocalRepo":             p.buildTool.GetLocalRepoPath(),
	}

	if p.mvnIndexPath != "" {
		argumentsMap["mavenIndexPath"] = p.mvnIndexPath
	}

	canRestrict, err := labels.CanRestrictSelector(condCTX.DepLabelSelector)
	if err != nil {
		p.log.Error(err, "could not construct dep label selector from condition context, search scope will not be limited", "label selector", condCTX.DepLabelSelector)
	} else if !canRestrict {
		// only set to false, when explicitely set to exclude oss libraries
		// this makes it backward compatible
		argumentsMap["includeOpenSourceLibraries"] = false
	}

	if !reflect.DeepEqual(c.Referenced.Annotated, annotated{}) {
		argumentsMap["annotationQuery"] = c.Referenced.Annotated
	}

	log := p.log.WithValues("ruleID", condCTX.RuleID)

	if len(p.includedPaths) > 0 {
		argumentsMap[provider.IncludedPathsConfigKey] = p.includedPaths
		log.V(8).Info("setting search scope by filepaths", "paths", p.includedPaths)
	} else if includedPaths, _ := condCTX.GetScopedFilepaths(); len(includedPaths) > 0 {
		argumentsMap[provider.IncludedPathsConfigKey] = includedPaths
		log.V(8).Info("setting search scope by filepaths", "paths", p.includedPaths, "argumentMap", argumentsMap)
	}

	argumentsBytes, _ := json.Marshal(argumentsMap)
	arguments := []json.RawMessage{argumentsBytes}

	wsp := &protocol.ExecuteCommandParams{
		Command:   "io.konveyor.tackle.ruleEntry",
		Arguments: arguments,
	}

	var refs []protocol.WorkspaceSymbol
	// If it takes us 5 min to complete a request, then we are in trouble
	timeout := 5 * time.Minute
	// certain wildcard queries are known to perform worse especially in containers
	if strings.HasSuffix(c.Referenced.Pattern, "*") || strings.HasSuffix(c.Referenced.Pattern, "*)") {
		timeout = 10 * time.Minute
	}
	p.activeRPCCalls.Add(1)
	defer p.activeRPCCalls.Done()

	timeOutCtx, cancelFunc := context.WithTimeout(ctx, timeout)
	defer cancelFunc()
	err = p.rpc.Call(timeOutCtx, "workspace/executeCommand", wsp).Await(timeOutCtx, &refs)
	if err != nil {
		if jsonrpc2.IsRPCClosed(err) {
			log.Error(err, "connection to the language server is closed, language server is not running")
			return refs, fmt.Errorf("connection to the language server is closed, language server is not running")
		} else {
			log.Error(err, "unable to ask for Konveyor rule entry")
			return refs, fmt.Errorf("unable to ask for Konveyor rule entry")
		}
	}

	// Only apply filepath filtering if there are actual filepath constraints.
	// The java language server indexes both source and dependencies (e.g., Maven repo),
	// and we need to allow analyzing dependency code unless explicitly filtered.
	if c.Referenced.Filepaths != nil {
		// filter according to the given filepaths
		var filteredRefs []protocol.WorkspaceSymbol
		for _, ref := range refs {
			for _, fp := range c.Referenced.Filepaths {
				if strings.HasSuffix(ref.Location.Value.(protocol.Location).URI, fp) {
					filteredRefs = append(filteredRefs, ref)
				}
			}
		}
		return filteredRefs, nil
	}

	return refs, nil
}


func (p *javaServiceClient) GetAllReferences(ctx context.Context, symbol protocol.WorkspaceSymbol) []protocol.Location {
	var locationURI protocol.DocumentURI
	var locationRange protocol.Range
	switch x := symbol.Location.Value.(type) {
	case protocol.Location:
		locationURI = x.URI
		locationRange = x.Range
	case protocol.PLocationMsg_workspace_symbol:
		locationURI = x.URI
		locationRange = protocol.Range{}
	default:
		locationURI = ""
		locationRange = protocol.Range{}
	}

	if strings.Contains(locationURI, JDT_CLASS_FILE_URI_PREFIX) {
		return []protocol.Location{
			{
				URI:   locationURI,
				Range: locationRange,
			},
		}
	}
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: locationURI,
			},
			Position: locationRange.Start,
		},
	}

	p.activeRPCCalls.Add(1)
	defer p.activeRPCCalls.Done()

	res := []protocol.Location{}
	err := p.rpc.Call(ctx, "textDocument/references", params).Await(ctx, &res)
	if err != nil {
		if jsonrpc2.IsRPCClosed(err) {
			p.log.Error(err, "connection to the language server is closed, language server is not running")
		} else {
			p.log.Error(err, "unknown error in RPC connection")
		}
	}
	return res
}

// TODO (pgaikwad) - implement this for real
func (p *javaServiceClient) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	return nil
}

func (p *javaServiceClient) Stop() {
	err := p.shutdown()
	if err != nil {
		p.log.Error(err, "failed to gracefully shutdown java provider")
	}
	err = p.cmd.Wait()
	if err != nil {
		if isSafeErr(err) {
			p.log.Info("java provider stopped")
		} else {
			p.log.Error(err, "java provider stopped with error")
		}
	} else {
		p.log.Info("java provider stopped")
	}

	if len(p.cleanExplodedBins) > 0 {
		for _, explodedPath := range p.cleanExplodedBins {
			os.RemoveAll(explodedPath)
		}
	}
}

func (p *javaServiceClient) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p.log.Info("waiting for active RPC calls to complete")
	done := make(chan struct{})
	go func() {
		p.activeRPCCalls.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.log.V(7).Info("all active RPC calls completed")
	case <-time.After(10 * time.Second):
		p.log.Info("timeout waiting for active RPC calls to complete, proceeding with shutdown")
	}

	var shutdownResult interface{}
	err := p.rpc.Call(shutdownCtx, "shutdown", nil).Await(shutdownCtx, &shutdownResult)
	if err != nil {
		p.log.Error(err, "failed to send shutdown request to language server")
		return err
	}
	err = p.rpc.Notify(shutdownCtx, "exit", nil)
	if err != nil {
		p.log.Error(err, "failed to send exit notification to language server")
		return err
	}
	return nil
}

func isSafeErr(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() && (status.Signal() == syscall.SIGTERM || status.Signal() == syscall.SIGKILL) {
				return true
			}
		}
	}

	return false
}

func (p *javaServiceClient) initialization(ctx context.Context) {
	absLocation, err := filepath.Abs(p.config.Location)
	if err != nil {
		p.log.Error(err, "unable to get path to analyize")
		panic(1)
	}

	var absBundles []string
	for _, bundle := range p.bundles {
		abs, err := filepath.Abs(bundle)
		if err != nil {
			p.log.Error(err, "unable to get path to bundles")
			panic(1)
		}
		absBundles = append(absBundles, abs)

	}
	downloadSources := true
	if p.config.AnalysisMode == provider.SourceOnlyAnalysisMode {
		downloadSources = false
	}

	//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
	params := &protocol.InitializeParams{}
	params.RootURI = string(uri.File(absLocation))
	params.Capabilities = protocol.ClientCapabilities{}
	params.ExtendedClientCapilities = map[string]any{
		"classFileContentsSupport": true,
	}
	// See https://github.com/eclipse-jdtls/eclipse.jdt.ls/blob/1a3dd9323756113bf39cfab82746d57a2fd19474/org.eclipse.jdt.ls.core/src/org/eclipse/jdt/ls/core/internal/preferences/Preferences.java
	java8home := os.Getenv("JAVA8_HOME")
	params.InitializationOptions = map[string]any{
		"bundles":          absBundles,
		"workspaceFolders": []string{string(uri.File(absLocation))},
		"settings": map[string]any{
			"java": map[string]any{
				"configuration": map[string]any{
					"maven": map[string]any{
						"userSettings":   p.mvnSettingsFile,
						"globalSettings": p.globalSettings,
					},
				},
				"autobuild": map[string]any{
					"enabled": false,
				},
				"maven": map[string]any{
					"downloadSources": downloadSources,
				},
				"import": map[string]any{
					"gradle": map[string]any{
						"java": map[string]any{
							"home": java8home,
						},
					},
				},
			},
		},
	}

	// when neither pom or gradle build is present, the language server cannot initialize project
	// we have to trick it into initializing it by creating a .classpath and .project file if one doesn't exist
	//TODO: This needs to happen only when
	if p.buildTool == nil {
		err = createProjectAndClasspathFiles(p.config.Location, filepath.Base(p.config.Location))
		if err != nil {
			p.log.Error(err, "unable to create .classpath and .project files, analysis may be degraded")
		}
	}

	var result protocol.InitializeResult
	for i := 0; i < 10; i++ {
		err := p.rpc.Call(ctx, "initialize", params).Await(ctx, &result)
		if err != nil {
			if jsonrpc2.IsRPCClosed(err) {
				p.log.Error(err, "connection to the language server is closed, language server is not running")
			} else {
				p.log.Error(err, "initialize failed")
			}
			continue
		}
		break
	}
	if err := p.rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		p.log.Error(err, "initialize failed")
	}
	p.log.V(2).Info("java connection initialized")

}

func createProjectAndClasspathFiles(basePath string, projectName string) error {
	projectXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<projectDescription>
    <name>%s</name>
    <comment></comment>
    <projects></projects>
    <buildSpec></buildSpec>
    <natures>
        <nature>org.eclipse.jdt.core.javanature</nature>
    </natures>
</projectDescription>
`, projectName)

	if _, err := os.Stat(filepath.Join(basePath, ".project")); err != nil && os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(basePath, ".project"), []byte(projectXML), 0644); err != nil {
			return fmt.Errorf("failed to write .project: %w", err)
		}
	}

	classpathXML := `<?xml version="1.0" encoding="UTF-8"?>
<classpath>
    <classpathentry kind="src" path="."/>
    <classpathentry kind="con" path="org.eclipse.jdt.launching.JRE_CONTAINER"/>
    <classpathentry kind="output" path="bin"/>
</classpath>
`
	if _, err := os.Stat(filepath.Join(basePath, ".classpath")); err != nil && os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(basePath, ".classpath"), []byte(classpathXML), 0644); err != nil {
			return fmt.Errorf("failed to write .classpath: %w", err)
		}
	}
	return nil
}
