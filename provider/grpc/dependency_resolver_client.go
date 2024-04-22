package grpc

import (
	"context"

	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	pb "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type dependencyLocationResolverClient struct {
	client pb.ProviderDependencyLocationServiceClient
}

// GetLocation implements provider.DependencyLocationResolver.
func (d *dependencyLocationResolverClient) GetLocation(ctx context.Context, dep konveyor.Dep, depFile string) (engine.Location, error) {
	extras, err := structpb.NewStruct(dep.Extras)
	if err != nil {
		return engine.Location{}, err
	}

	res, err := d.client.GetDependencyLocation(context.TODO(), &pb.GetDependencyLocationRequest{
		Dep: &pb.Dependency{
			Name:               dep.Name,
			Version:            dep.Version,
			Classifier:         dep.Classifier,
			Type:               dep.Type,
			ResolvedIdentifier: dep.ResolvedIdentifier,
			FileURIPrefix:      dep.FileURIPrefix,
			Indirect:           dep.Indirect,
			Extras:             extras,
			Labels:             dep.Labels,
		},
		DepFile: depFile,
	})
	if res.Location == nil {
		return engine.Location{}, nil
	}
	loc := engine.Location{}
	if res.Location.StartPosition != nil {
		loc.StartPosition = engine.Position{}
		loc.StartPosition.Line = int(res.Location.StartPosition.Line)
		loc.StartPosition.Character = int(res.Location.StartPosition.Character)
	}
	if res.Location.EndPosition != nil {
		loc.EndPosition = engine.Position{}
		loc.EndPosition.Line = int(res.Location.EndPosition.Line)
		loc.EndPosition.Character = int(res.Location.EndPosition.Character)
	}

	return loc, nil
}

var _ provider.DependencyLocationResolver = &dependencyLocationResolverClient{}
