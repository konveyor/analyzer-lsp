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
}

var _ provider.Client = &grpcProvider{}
var _ provider.Startable = &grpcProvider{}

func NewGRPCClient(config provider.Config, log logr.Logger) *grpcProvider {
	return &grpcProvider{
		config: config,
		log:    log,
	}
}

func (g *grpcProvider) Stop() {
	g.Client.Stop(context.TODO(), &emptypb.Empty{})
	g.conn.Close()
}

func (g *grpcProvider) HasCapability(name string) bool {
	m := pb.HasCapabilityRequest{
		Capability: name,
	}

	r, err := g.Client.HasCapability(context.TODO(), &m)
	if err != nil {
		// Handle this smarter in the future, for now log and return empty
		g.log.V(5).Error(err, "grpc unable to get info")
		return false
	}
	return r.HasCap
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

func (g *grpcProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (int, error) {
	g.log = log.WithValues("provider", "grpc")
	g.ctx = ctx

	m := map[string]interface{}{}
	for k, v := range config.ProviderSpecificConfig {
		m[k] = v
	}

	s, err := structpb.NewStruct(m)

	c := pb.Config{
		Location:               config.Location,
		DependencyPath:         config.DependencyPath,
		LspServerPath:          config.LSPServerPath,
		ProviderSpecificConfig: s,
	}

	r, err := g.Client.Init(ctx, &c)

	if err != nil {
		return 0, err
	}
	if !r.Successful {
		return 0, fmt.Errorf(r.Error)
	}
	return int(r.Id), nil
}

func (g *grpcProvider) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	m := pb.EvaluateRequest{
		Cap:           cap,
		ConditionInfo: string(conditionInfo),
	}
	r, err := g.Client.Evaluate(g.ctx, &m)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	if !r.Successful {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf(r.Error)
	}

	if !r.Response.Matched {
		return provider.ProviderEvaluateResponse{
			Matched:         false,
			TemplateContext: r.Response.TemplateContext.AsMap(),
		}, nil
	}

	incs := []provider.IncidentContext{}
	for _, i := range r.Response.IncidentContexts {
		inc := provider.IncidentContext{
			FileURI:   uri.URI(i.FileURI),
			Variables: i.GetVariables().AsMap(),
		}
		if i.Effort != nil {
			num := int(*i.Effort)
			inc.Effort = &num
		}
		links := []provider.ExternalLinks{}
		for _, l := range i.Links {
			links = append(links, provider.ExternalLinks{
				URL:   l.Url,
				Title: l.Title,
			})
		}
		inc.Links = links
		if i.CodeLocation != nil {
			inc.CodeLocation = &provider.Location{
				StartPosition: provider.Position{
					Line:      i.CodeLocation.StartPosition.Line,
					Character: i.CodeLocation.StartPosition.Character,
				},
				EndPosition: provider.Position{
					Line:      i.CodeLocation.EndPosition.Line,
					Character: i.CodeLocation.EndPosition.Character,
				},
			}
		}
		incs = append(incs, inc)
	}

	return provider.ProviderEvaluateResponse{
		Matched:         true,
		Incidents:       incs,
		TemplateContext: r.Response.TemplateContext.AsMap(),
	}, nil
}

// We don't have dependencies
func (g *grpcProvider) GetDependencies() ([]provider.Dep, uri.URI, error) {
	d, err := g.Client.GetDependencies(g.ctx, &emptypb.Empty{})
	if err != nil {
		return nil, uri.URI(""), err
	}
	if !d.Successful {
		return nil, uri.URI(""), fmt.Errorf(d.Error)
	}

	provs := []provider.Dep{}
	for _, x := range d.List.Deps {
		provs = append(provs, provider.Dep{
			Name:     x.Name,
			Version:  x.Version,
			Type:     x.Type,
			Indirect: x.Indirect,
			SHA:      x.Sha,
		})
	}

	u, err := uri.Parse(d.FileURI)
	if err != nil {
		return nil, uri.URI(""), fmt.Errorf(d.Error)
	}

	return provs, u, nil

}

// We don't have dependencies
func (g *grpcProvider) GetDependenciesLinkedList() (map[provider.Dep][]provider.Dep, uri.URI, error) {
	d, err := g.Client.GetDependenciesLinkedList(g.ctx, &emptypb.Empty{})
	if err != nil {
		return nil, uri.URI(""), err
	}
	if !d.Successful {
		return nil, uri.URI(""), fmt.Errorf(d.Error)
	}

	m := map[provider.Dep][]provider.Dep{}
	for _, v := range d.List {
		keyDep := provider.Dep{
			Name:     v.Key.Name,
			Version:  v.Key.Version,
			Type:     v.Key.Type,
			Indirect: v.Key.Indirect,
			SHA:      v.Key.Sha,
		}
		values := []provider.Dep{}
		for _, x := range v.Value.Deps {
			values = append(values, provider.Dep{
				Name:     x.Name,
				Version:  x.Version,
				Type:     x.Type,
				Indirect: x.Indirect,
				SHA:      x.Sha,
			})
		}
		m[keyDep] = values
	}

	u, err := uri.Parse(d.FileURI)
	if err != nil {
		return nil, uri.URI(""), fmt.Errorf(d.Error)
	}

	return m, u, nil

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
