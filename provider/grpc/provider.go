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
	"github.com/konveyor/analyzer-lsp/provider"
	pb "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
	"github.com/phayes/freeport"
	"go.lsp.dev/uri"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type grpcProvider struct {
	Client pb.ProviderServiceClient
	log    logr.Logger
	ctx    context.Context
	conn   *grpc.ClientConn
	config provider.Config

	serviceClients []provider.ServiceClient
}

var _ provider.InternalProviderClient = &grpcProvider{}
var _ provider.Startable = &grpcProvider{}

func NewGRPCClient(config provider.Config, log logr.Logger) *grpcProvider {
	log = log.WithName(config.Name)
	log = log.WithValues("provider", "grpc")
	return &grpcProvider{
		config:         config,
		log:            log,
		serviceClients: []provider.ServiceClient{},
	}
}

func (g *grpcProvider) ProviderInit(ctx context.Context) error {
	for _, c := range g.config.InitConfig {
		s, err := g.Init(ctx, g.log, c)
		if err != nil {
			g.log.Error(err, "Error inside ProviderInit, after g.Init.")
			return err
		}
		g.serviceClients = append(g.serviceClients, s)
	}
	return nil
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

func (g *grpcProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, error) {
	s, err := structpb.NewStruct(config.ProviderSpecificConfig)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if !r.Successful {
		return nil, fmt.Errorf(r.Error)
	}
	return &grpcServiceClient{
		id:     r.Id,
		config: config,
		client: g.Client,
	}, nil
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
}

func (g *grpcProvider) Start(ctx context.Context) error {
	// Here the Provider will start the GRPC Server if a binary is set.
	if g.config.BinaryPath != "" {
		port, err := freeport.GetFreePort()
		if err != nil {
			return err
		}

		// For the generic external provider
		name := "generic"
		ic := g.config.InitConfig
		if len(ic) != 0 {
			if newName, ok := ic[0].ProviderSpecificConfig["lspServerName"].(string); ok {
				name = newName
			}
		}

		cmd := exec.CommandContext(ctx, g.config.BinaryPath, "--port", fmt.Sprintf("%v", port), "--name", name)
		// TODO: For each output line, log that line here, allows the server's to output to the main log file. Make sure we name this correctly
		// cmd will exit with the ending of the ctx.
		out, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		go g.LogProviderOut(ctx, out)

		err = cmd.Start()
		if err != nil {
			return err
		}
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%v", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		c := pb.NewProviderServiceClient(conn)
		g.conn = conn
		g.Client = c

		// Give the server some time to start up, assume that a server MUST provide 1 cap
		for {
			select {
			default:
				caps := g.Capabilities()
				if len(caps) != 0 {
					g.log.Error(nil, "Caps found", "caps", caps)
					return nil
				}
				time.Sleep(3 * time.Second)
			case <-time.After(time.Second * 30):
				return fmt.Errorf("no Capabilities for provider: %v", g.config.Name)
			}
		}
	}
	if g.config.Address != "" {
		conn, err := grpc.Dial(fmt.Sprintf(g.config.Address), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		c := pb.NewProviderServiceClient(conn)
		g.conn = conn
		g.Client = c

		return nil

	}
	return fmt.Errorf("must set Address or Binary Path for a GRPC provider")
}

func (g *grpcProvider) LogProviderOut(ctx context.Context, out io.ReadCloser) {
	scan := bufio.NewScanner(out)

	for scan.Scan() {
		g.log.V(3).Info(scan.Text())
	}
}
