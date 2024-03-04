package grpc

import (
	"context"
	"fmt"

	"github.com/konveyor/analyzer-lsp/provider"
	pb "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
	"go.lsp.dev/uri"
)

type grpcServiceClient struct {
	id     int64
	config provider.InitConfig
	client pb.ProviderServiceClient
}

var _ provider.ServiceClient = &grpcServiceClient{}

func (g *grpcServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	m := pb.EvaluateRequest{
		Cap:           cap,
		ConditionInfo: string(conditionInfo),
		Id:            g.id,
	}

	r, err := g.client.Evaluate(ctx, &m)
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
		if i.LineNumber != nil {
			lineNumber := int(*i.LineNumber)
			inc.LineNumber = &lineNumber
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
func (g *grpcServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	d, err := g.client.GetDependencies(ctx, &pb.ServiceRequest{Id: g.id})
	if err != nil {
		return nil, err
	}
	if !d.Successful {
		return nil, fmt.Errorf(d.Error)
	}

	provs := map[uri.URI][]*provider.Dep{}
	for _, x := range d.FileDep {
		u, err := uri.Parse(x.FileURI)
		if err != nil {
			u = uri.URI(x.FileURI)
		}
		deps := []*provider.Dep{}
		for _, d := range x.List.Deps {
			deps = append(deps, &provider.Dep{
				Name:               d.Name,
				Version:            d.Version,
				Type:               d.Type,
				Indirect:           d.Indirect,
				ResolvedIdentifier: d.ResolvedIdentifier,
				Extras:             d.Extras.AsMap(),
				Labels:             d.Labels,
			})
		}
		provs[u] = deps
	}

	return provs, nil

}

func recreateDAGAddedItems(items []*pb.DependencyDAGItem) []provider.DepDAGItem {

	deps := []provider.DepDAGItem{}
	for _, x := range items {
		deps = append(deps, provider.DepDAGItem{
			Dep: provider.Dep{
				Name:               x.Key.Name,
				Version:            x.Key.Version,
				Type:               x.Key.Type,
				Indirect:           x.Key.Indirect,
				ResolvedIdentifier: x.Key.ResolvedIdentifier,
				Extras:             x.Key.Extras.AsMap(),
				Labels:             x.Key.Labels,
			},
			AddedDeps: recreateDAGAddedItems(x.AddedDeps),
		})
	}
	return deps
}

// We don't have dependencies
func (g *grpcServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	d, err := g.client.GetDependenciesDAG(ctx, &pb.ServiceRequest{Id: g.id})
	if err != nil {
		return nil, err
	}
	if !d.Successful {
		return nil, fmt.Errorf(d.Error)
	}
	m := map[uri.URI][]provider.DepDAGItem{}
	for _, x := range d.FileDagDep {
		u, err := uri.Parse(x.FileURI)
		if err != nil {
			return nil, fmt.Errorf(d.Error)
		}
		deps := recreateDAGAddedItems(x.List)
		m[u] = deps
	}

	return m, nil

}

func (g *grpcServiceClient) Stop() {
	g.client.Stop(context.TODO(), &pb.ServiceRequest{Id: g.id})
}
