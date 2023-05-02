package grpc

import (
	"context"
	"fmt"
	"log"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	pb "github.com/konveyor/analyzer-lsp/provider/lib/grpc"
	"go.lsp.dev/uri"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type grpcProvider struct {
	Client pb.ProviderServiceClient
	log    logr.Logger
	ctx    context.Context
	conn   *grpc.ClientConn
}

func NewGRPCClient() *grpcProvider {
	conn, err := grpc.Dial("localhost:17902", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	c := pb.NewProviderServiceClient(conn)
	return &grpcProvider{
		Client: c,
		conn:   conn,
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

func (g *grpcProvider) Capabilities() []lib.Capability {
	r, err := g.Client.Capabilities(context.TODO(), &emptypb.Empty{})
	if err != nil {
		// Handle this smarter in the future, for now log and return empty
		g.log.V(5).Error(err, "grpc unable to get info")
		return nil
	}

	c := []lib.Capability{}
	for _, x := range r.Capabilities {
		v := lib.Capability{
			Name: x.Name,
			//TemplateContext: x.TemplateContext.AsMap(),
		}
		c = append(c, v)
	}
	return c
}

func (g *grpcProvider) Init(ctx context.Context, log logr.Logger) error {
	g.log = log.WithValues("provider", "grpc")
	g.ctx = ctx
	r, err := g.Client.Init(ctx, &emptypb.Empty{})

	if err != nil {
		return err
	}
	if !r.Successful {
		return fmt.Errorf(r.Error)
	}
	return nil
}

func (g *grpcProvider) Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error) {
	m := pb.EvaluateRequest{
		Cap:           cap,
		ConditionInfo: string(conditionInfo),
	}
	r, err := g.Client.Evaluate(g.ctx, &m)
	if err != nil {
		return lib.ProviderEvaluateResponse{}, err
	}

	if !r.Successful {
		return lib.ProviderEvaluateResponse{}, fmt.Errorf(r.Error)
	}

	if !r.Response.Matched {
		return lib.ProviderEvaluateResponse{
			Matched:         false,
			TemplateContext: r.Response.TemplateContext.AsMap(),
		}, nil
	}

	incs := []lib.IncidentContext{}
	for _, i := range r.Response.IncidentContexts {
		inc := lib.IncidentContext{
			FileURI:   uri.URI(i.FileURI),
			Variables: i.GetVariables().AsMap(),
		}
		if i.Effort != nil {
			num := int(*i.Effort)
			inc.Effort = &num
		}
		links := []lib.ExternalLinks{}
		for _, l := range i.Links {
			links = append(links, lib.ExternalLinks{
				URL:   l.Url,
				Title: l.Title,
			})
		}
		inc.Links = links
		if i.CodeLocation != nil {
			inc.CodeLocation = &lib.Location{
				StartPosition: lib.Position{
					Line:      i.CodeLocation.StartPosition.Line,
					Character: i.CodeLocation.StartPosition.Character,
				},
				EndPosition: lib.Position{
					Line:      i.CodeLocation.EndPosition.Line,
					Character: i.CodeLocation.EndPosition.Character,
				},
			}
		}
		incs = append(incs, inc)
	}

	return lib.ProviderEvaluateResponse{
		Matched:         true,
		Incidents:       incs,
		TemplateContext: r.Response.TemplateContext.AsMap(),
	}, nil
}

// We don't have dependencies
func (g *grpcProvider) GetDependencies() ([]dependency.Dep, uri.URI, error) {
	return nil, "", nil
}

// We don't have dependencies
func (g *grpcProvider) GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, uri.URI, error) {
	return nil, "", nil
}
