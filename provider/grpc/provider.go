package grpc

import (
	"context"
	"fmt"
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
	log = log.WithValues("provider", "grpc")
	return &grpcProvider{
		config:         config,
		log:            log,
		serviceClients: []provider.ServiceClient{},
	}
}

func (g *grpcProvider) ProviderInit(ctx context.Context) error {
	g.ctx = ctx
	for _, c := range g.config.InitConfig {
		s, err := g.Init(ctx, g.log, c)
		if err != nil {
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
		LspServerPath:          config.LSPServerPath,
		ProviderSpecificConfig: s,
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
		ctx:    ctx,
		config: config,
		client: g.Client,
	}, nil
}

func (g *grpcProvider) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.FullResponseFromServiceClients(g.serviceClients, cap, conditionInfo)
}

// TODO: Come back through and re-desing output to handle File A get X-Z deps and File B gets C-E
func (g *grpcProvider) GetDependencies() ([]provider.Dep, uri.URI, error) {
	return provider.FullDepsResponse(g.serviceClients)
}

// TODO: Come back through and re-desing output to handle File A get X-Z deps and File B gets C-E
func (g *grpcProvider) GetDependenciesDAG() ([]provider.DepDAGItem, uri.URI, error) {
	return provider.FullDepDAGResponse(g.serviceClients)
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
		cmd := exec.CommandContext(ctx, g.config.BinaryPath, "--port", fmt.Sprintf("%v", port))
		// TODO: For each output line, log that line here, allows the server's to output to the main log file. Make sure we name this correctly
		// cmd will exit with the ending of the ctx.
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
