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

	// The response is optional, if the provider says that it was successful but no response then nothing matched.
	if r.Response == nil {
		return provider.ProviderEvaluateResponse{
			Matched: false,
		}, nil
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
			FileURI:              uri.URI(i.FileURI),
			Variables:            i.GetVariables().AsMap(),
			IsDependencyIncident: i.IsDependencyIncident,
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
				Classifier:         d.Classifier,
				Type:               d.Type,
				Indirect:           d.Indirect,
				ResolvedIdentifier: d.ResolvedIdentifier,
				Extras:             d.Extras.AsMap(),
				Labels:             d.Labels,
				FileURIPrefix:      d.FileURIPrefix,
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
				Classifier:         x.Key.Classifier,
				Type:               x.Key.Type,
				Indirect:           x.Key.Indirect,
				ResolvedIdentifier: x.Key.ResolvedIdentifier,
				Extras:             x.Key.Extras.AsMap(),
				Labels:             x.Key.Labels,
				FileURIPrefix:      x.Key.FileURIPrefix,
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

func (g *grpcServiceClient) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	fileChanges := []*pb.FileChange{}

	for _, change := range changes {
		fileChanges = append(fileChanges, &pb.FileChange{
			Saved:   change.Saved,
			Uri:     change.Path,
			Content: change.Content,
		})
	}

	fileChangeResponse, err := g.client.NotifyFileChanges(ctx, &pb.NotifyFileChangesRequest{Changes: fileChanges, Id: g.id})
	if err != nil {
		return err
	}
	if fileChangeResponse.Error != "" {
		return fmt.Errorf(fileChangeResponse.Error)
	}
	return nil
}

func (g *grpcServiceClient) Stop() {
	g.client.Stop(context.TODO(), &pb.ServiceRequest{Id: g.id})
}

func (g *grpcServiceClient) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	conditionsByCapability := []*pb.ConditionsByCapability{}
	for _, condition := range conditionsByCap {
		conditions := []string{}
		for _, conditionInfo := range condition.Conditions {
			conditions = append(conditions, string(conditionInfo))
		}
		conditionsByCapability = append(conditionsByCapability, &pb.ConditionsByCapability{
			Cap:           condition.Cap,
			ConditionInfo: conditions,
		})
	}
	prepareRequest := &pb.PrepareRequest{
		Conditions: conditionsByCapability,
		Id:         g.id,
	}

	// Start progress streaming BEFORE calling Prepare so we don't miss any events
	streamDone := make(chan struct{})
	streamReady := make(chan struct{})
	if g.config.PrepareProgressReporter != nil {
		go func() {
			g.streamPrepareProgress(ctx, streamReady)
			close(streamDone)
		}()
		// Wait for stream to be established before calling Prepare
		<-streamReady
	}

	prepareResponse, err := g.client.Prepare(ctx, prepareRequest)
	if err != nil {
		return err
	}
	if prepareResponse.Error != "" {
		return fmt.Errorf(prepareResponse.Error)
	}

	// Wait for streaming to complete if it was started
	if g.config.PrepareProgressReporter != nil {
		<-streamDone
	}

	return nil
}

// streamPrepareProgress receives progress events from the GRPC server and forwards them
// to the configured PrepareProgressReporter.
func (g *grpcServiceClient) streamPrepareProgress(ctx context.Context, ready chan struct{}) {
	stream, err := g.client.StreamPrepareProgress(ctx, &pb.PrepareProgressRequest{Id: g.id})
	if err != nil {
		// Not an error - server might not support streaming or provider might not implement it
		close(ready) // Signal ready even on error so we don't block
		return
	}

	// Signal that the stream is ready
	close(ready)

	for {
		event, err := stream.Recv()
		if err != nil {
			// Stream ended (either normally or with error)
			return
		}

		// Forward the event to the progress reporter
		if g.config.PrepareProgressReporter != nil {
			g.config.PrepareProgressReporter.ReportProgress(
				event.ProviderName,
				int(event.FilesProcessed),
				int(event.TotalFiles),
			)
		}
	}
}
