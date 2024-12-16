package grpc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"

	"github.com/go-logr/logr"
	reflectClient "github.com/jhump/protoreflect/grpcreflect"
	"github.com/konveyor/analyzer-lsp/provider"
	pb "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
	"github.com/phayes/freeport"
	"go.lsp.dev/uri"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type grpcProvider struct {
	Client    pb.ProviderServiceClient
	log       logr.Logger
	ctx       context.Context
	conn      *grpc.ClientConn
	config    provider.Config
	cancelCmd context.CancelFunc

	serviceClients []provider.ServiceClient
}

var _ provider.InternalProviderClient = &grpcProvider{}

func NewGRPCClient(config provider.Config, log logr.Logger) (provider.InternalProviderClient, error) {
	log = log.WithName(config.Name)
	log = log.WithValues("provider", "grpc")
	ctxCmd, cancelCmd := context.WithCancel(context.Background())
	conn, out, err := start(ctxCmd, config)
	if err != nil {
		return nil, err
	}
	refCltCtx, cancel := context.WithCancel(context.Background())
	refClt := reflectClient.NewClientAuto(refCltCtx, conn)
	defer cancel()

	services, err := checkServicesRunning(refClt, log)
	if err != nil {
		fmt.Printf("\n%v\n", err)
		return nil, err
	}
	foundCodeSnip := false
	foundDepResolve := false
	for _, s := range services {
		// TODO: Make consts
		if s == "provider.ProviderCodeLocationService" {
			foundCodeSnip = true
		}
		if s == "provider.ProviderDependencyLocationService" {
			foundDepResolve = true
		}
	}
	// Always need these
	provierClient := pb.NewProviderServiceClient(conn)
	gp := grpcProvider{
		Client:         provierClient,
		log:            log,
		ctx:            refCltCtx,
		conn:           conn,
		config:         config,
		cancelCmd:      cancelCmd,
		serviceClients: []provider.ServiceClient{},
	}
	if out != nil {
		go gp.LogProviderOut(context.Background(), out)
	}
	if foundCodeSnip && foundDepResolve {
		// create the clients, create the struct that will have all the methods

		cspc := pb.NewProviderCodeLocationServiceClient(conn)
		dlrc := pb.NewProviderDependencyLocationServiceClient(conn)

		return struct {
			*grpcProvider
			*codeSnipProviderClient
			*dependencyLocationResolverClient
		}{
			grpcProvider: &gp,
			codeSnipProviderClient: &codeSnipProviderClient{
				client: cspc,
			},
			dependencyLocationResolverClient: &dependencyLocationResolverClient{
				client: dlrc,
			},
		}, nil

	} else if foundCodeSnip && !foundDepResolve {
		// create the clients, create the struct that will have all the methods but dep resolve
		cspc := pb.NewProviderCodeLocationServiceClient(conn)

		return struct {
			*grpcProvider
			*codeSnipProviderClient
		}{
			grpcProvider: &gp,
			codeSnipProviderClient: &codeSnipProviderClient{
				client: cspc,
			},
		}, nil
	} else if !foundCodeSnip && foundDepResolve {

		dlrc := pb.NewProviderDependencyLocationServiceClient(conn)

		return struct {
			*grpcProvider
			*dependencyLocationResolverClient
		}{
			grpcProvider: &gp,
			dependencyLocationResolverClient: &dependencyLocationResolverClient{
				client: dlrc,
			},
		}, nil

	} else {
		// just create grpcProvider
		return &gp, nil
	}
}

func checkServicesRunning(refClt *reflectClient.Client, log logr.Logger) ([]string, error) {
	for {
		select {
		default:
			services, err := refClt.ListServices()
			if err == nil && len(services) != 0 {
				return services, nil
			}
			if err != nil {
				log.Error(err, "error for list services retrying")
			}
			time.Sleep(3 * time.Second)
		case <-time.After(time.Second * 30):
			return nil, fmt.Errorf("no services found")
		}
	}
}

func (g *grpcProvider) ProviderInit(ctx context.Context, additionalConfigs []provider.InitConfig) ([]provider.InitConfig, error) {
	builtinConfs := []provider.InitConfig{}
	if additionalConfigs != nil {
		g.config.InitConfig = append(g.config.InitConfig, additionalConfigs...)
	}
	for _, c := range g.config.InitConfig {
		s, builtinConf, err := g.Init(ctx, g.log, c)
		if err != nil {
			g.log.Error(err, "Error inside ProviderInit, after g.Init.")
			return nil, err
		}
		g.serviceClients = append(g.serviceClients, s)
		if builtinConf.Location != "" {
			builtinConfs = append(builtinConfs, builtinConf)
		}
	}
	return builtinConfs, nil
}

func (g *grpcProvider) Capabilities() []provider.Capability {
	r, err := g.Client.Capabilities(context.TODO(), &emptypb.Empty{})
	if err != nil {
		// Handle this smarter in the future, for now log and return empty
		g.log.V(5).Error(err, "grpc unable to get info")
		return nil
	}

	c := []provider.Capability{}
	for _, x := range r.Capabilities {
		v := provider.Capability{
			Name: x.Name,
			//TemplateContext: x.TemplateContext.AsMap(),
		}
		c = append(c, v)
	}
	return c
}

func (g *grpcProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	s, err := structpb.NewStruct(config.ProviderSpecificConfig)
	if err != nil {
		return nil, provider.InitConfig{}, err
	}

	c := pb.Config{
		Location:               config.Location,
		DependencyPath:         config.DependencyPath,
		AnalysisMode:           string(config.AnalysisMode),
		ProviderSpecificConfig: s,
		Proxy: &pb.Proxy{
			HTTPProxy:  config.Proxy.HTTPProxy,
			HTTPSProxy: config.Proxy.HTTPSProxy,
			NoProxy:    config.Proxy.NoProxy,
		},
	}

	r, err := g.Client.Init(ctx, &c)

	if err != nil {
		return nil, provider.InitConfig{}, err
	}
	if !r.Successful {
		return nil, provider.InitConfig{}, fmt.Errorf(r.Error)
	}
	additionalBuiltinConfig := provider.InitConfig{}
	if r.BuiltinConfig != nil {
		additionalBuiltinConfig.Location = r.BuiltinConfig.Location
	}
	return &grpcServiceClient{
		id:     r.Id,
		config: config,
		client: g.Client,
	}, additionalBuiltinConfig, nil
}

func (g *grpcProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.FullResponseFromServiceClients(ctx, g.serviceClients, cap, conditionInfo)
}

func (g *grpcProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return provider.FullDepsResponse(ctx, g.serviceClients)
}

func (g *grpcProvider) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return provider.FullDepDAGResponse(ctx, g.serviceClients)
}

func (g *grpcProvider) Stop() {
	for _, c := range g.serviceClients {
		c.Stop()
	}
	g.conn.Close()
	g.cancelCmd()
}

func start(ctx context.Context, config provider.Config) (*grpc.ClientConn, io.ReadCloser, error) {
	// Here the Provider will start the GRPC Server if a binary is set.
	if config.BinaryPath != "" {
		port, err := freeport.GetFreePort()
		if err != nil {
			return nil, nil, err
		}

		ic := config.InitConfig
		// For the generic external provider
		name := "generic"
		if len(ic) != 0 {
			if newName, ok := ic[0].ProviderSpecificConfig["lspServerName"].(string); ok {
				name = newName
			}
		}

		cmd := exec.CommandContext(ctx, config.BinaryPath, "--port", fmt.Sprintf("%v", port), "--name", name)
		// TODO: For each output line, log that line here, allows the server's to output to the main log file. Make sure we name this correctly
		// cmd will exit with the ending of the ctx.
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}

		err = cmd.Start()
		if err != nil {
			return nil, nil, err
		}
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%v", port),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(provider.MAX_MESSAGE_SIZE)),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		return conn, out, nil
	}
	if config.Address != "" {
		if config.CertPath == "" {
			conn, err := grpc.Dial(fmt.Sprintf(config.Address),
				grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(provider.MAX_MESSAGE_SIZE)),
				grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			return conn, nil, nil
		} else {
			creds, err := credentials.NewClientTLSFromFile(config.CertPath, "")
			if err != nil {
				return nil, nil, err
			}
			if config.JWTToken == "" {
				conn, err := grpc.Dial(fmt.Sprintf(config.Address),
					grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(provider.MAX_MESSAGE_SIZE)),
					grpc.WithTransportCredentials(creds))
				if err != nil {
					log.Fatalf("did not connect: %v", err)
				}
				return conn, nil, nil

			} else {
				i := &jwtTokeInterceptor{
					Token: config.JWTToken,
				}
				conn, err := grpc.Dial(fmt.Sprintf(config.Address),
					grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(provider.MAX_MESSAGE_SIZE)),
					grpc.WithTransportCredentials(creds), grpc.WithUnaryInterceptor(i.unaryInterceptor))
				if err != nil {
					log.Fatalf("did not connect: %v", err)
				}
				return conn, nil, nil

			}
		}
	}
	return nil, nil, fmt.Errorf("must set Address or Binary Path for a GRPC provider")
}

func (g *grpcProvider) LogProviderOut(ctx context.Context, out io.ReadCloser) {
	scan := bufio.NewScanner(out)

	for scan.Scan() {
		g.log.V(3).Info(scan.Text())
	}
}

type jwtTokeInterceptor struct {
	Token string
}

func (t *jwtTokeInterceptor) unaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if t.Token != "" {
		opts = append(opts, grpc.PerRPCCredentials(oauth.TokenSource{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: t.Token}),
		}))
	}
	err := invoker(ctx, method, req, reply, cc, opts...)
	return err
}
