package dotnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type dotnetProvider struct {
	Log logr.Logger
}

var _ provider.BaseClient = &dotnetProvider{}

func NewDotnetProvider(log logr.Logger) *dotnetProvider {
	return &dotnetProvider{
		Log: log,
	}
}

type stdioRWCloser struct {
	io.Reader
	io.Writer
}

type dotnetCondition struct {
	Referenced referenceCondition `yaml:"referenced"`
}

// Example:
// dotnet.referenced:
//
//	namespace: System.Web.Mvc
//	pattern: HttpNotFound
type referenceCondition struct {
	Namespace string `yaml:"namespace"`
	Pattern   string `yaml:"pattern"`
}

func (r *stdioRWCloser) Close() error {
	return nil
}

func (p *dotnetProvider) Capabilities() []provider.Capability {
	r := openapi3.NewReflector()
	caps := []provider.Capability{}
	refCap, err := provider.ToProviderCap(r, p.Log, dotnetCondition{}, "referenced")
	if err != nil {
		p.Log.Error(err, "failed to registery capability")
	} else {
		caps = append(caps, refCap)
	}
	return caps
}

func (p *dotnetProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	var mode provider.AnalysisMode = provider.AnalysisMode(config.AnalysisMode)
	if mode != provider.SourceOnlyAnalysisMode {
		return nil, provider.InitConfig{}, fmt.Errorf("only full analysis is supported")
	}

	// handle proxy settings
	for k, v := range config.Proxy.ToEnvVars() {
		os.Setenv(k, v)
	}

	codePath, err := filepath.Abs(config.Location)
	if err != nil {
		log.Error(err, "unable to get path to analyze")
		return nil, provider.InitConfig{}, err
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	log = log.WithValues("provider", "dotnet")
	sentLog := &sent{l: log.WithValues("stdio", "sent")}
	recvLog := &received{l: log.WithValues("stdio", "recv")}
	handlerLog := log.WithValues("stdio", "replyHandler")

	lspServerPath, ok := config.ProviderSpecificConfig[provider.LspServerPathConfigKey].(string)
	if !ok || lspServerPath == "" {
		cancelFunc()
		return nil, provider.InitConfig{}, fmt.Errorf("invalid lspServerPath provided, unable to init dotnet provider")
	}

	cmd := exec.CommandContext(ctx, lspServerPath)
	cmd.Dir = codePath // At a minimum, 'csharp-ls' doesn't respect URI @initialization
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancelFunc()
		return nil, provider.InitConfig{}, err
	}
	clientWriter := io.MultiWriter(stdin, sentLog)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelFunc()
		return nil, provider.InitConfig{}, err
	}
	clientReader := io.TeeReader(stdout, recvLog)
	if err := cmd.Start(); err != nil {
		log.Error(err, "failed to start language server process")
		cancelFunc()
		return nil, provider.InitConfig{}, err
	}
	log.V(2).Info("language server started")

	// Unlike the golang-external-provider, we need to be able to respond
	// to requests from the server. This requires us to startup a server
	// to handle those requests.
	serverChannel := make(chan int)
	h := &handler{
		log: &handlerLog,
		ch:  serverChannel,
	}
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(&stdioRWCloser{
		Reader: clientReader,
		Writer: clientWriter,
	}))
	go func() {
		err := jsonrpc2.HandlerServer(jsonrpc2.ReplyHandler(h.replyHandler)).ServeStream(ctx, conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				handlerLog.Info("received eof", "canceled", errors.Is(ctx.Err(), context.Canceled))
				return
			}
			handlerLog.Error(err, "something bad happened to our client side server")
			return
		}
	}()
	log.V(2).Info("language server connection established")

	log.V(2).Info("initializing language server")
	var initializeResult protocol.InitializeResult
	for {
		if _, err := conn.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{
			RootURI: uri.File(codePath),
			Capabilities: protocol.ClientCapabilities{
				TextDocument: &protocol.TextDocumentClientCapabilities{
					DocumentSymbol: &protocol.DocumentSymbolClientCapabilities{
						HierarchicalDocumentSymbolSupport: true,
					},
				},
				Workspace: &protocol.WorkspaceClientCapabilities{
					DidChangeWatchedFiles: &protocol.DidChangeWatchedFilesWorkspaceClientCapabilities{
						DynamicRegistration: false,
					},
					// WorkspaceFolders: true,
				},
			},
			// WorkspaceFolders: []protocol.WorkspaceFolder{
			// 	protocol.WorkspaceFolder{
			// 		URI: "/opt/app-root/src",
			// 		Name: "workspace",
			// 	},
			// },
		}, &initializeResult); err != nil {
			log.Error(err, "initialize failed, will try again")
			continue
		}
		break
	}
	log.V(2).Info("language server initialized")

	if err := conn.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{}); err != nil {
		log.Error(err, "initialized notification failed")
		cancelFunc()
		return nil, provider.InitConfig{}, err
	}

	log.Info("waiting for language server to load the project")
	<-serverChannel
	log.Info("project loaded")

	return &dotnetServiceClient{
		rpc:        conn,
		ctx:        ctx,
		cancelFunc: cancelFunc,
		cmd:        cmd,
		log:        log,
		config:     config,
	}, provider.InitConfig{}, nil
}
