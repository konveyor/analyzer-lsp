package grpc

import (
	"context"

	"github.com/konveyor/analyzer-lsp/engine"
	pb "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
	"go.lsp.dev/uri"
)

type codeSnipProviderClient struct {
	client pb.ProviderCodeLocationServiceClient
}

// GetCodeSnip implements engine.CodeSnip.
func (c *codeSnipProviderClient) GetCodeSnip(u uri.URI, loc engine.Location) (string, error) {
	resp, err := c.client.GetCodeSnip(context.TODO(), &pb.GetCodeSnipRequest{
		Uri: string(u),
		CodeLocation: &pb.Location{
			StartPosition: &pb.Position{
				Line:      float64(loc.StartPosition.Line),
				Character: float64(loc.StartPosition.Character),
			},
			EndPosition: &pb.Position{
				Line:      float64(loc.EndPosition.Line),
				Character: float64(loc.EndPosition.Character),
			},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Snip, nil
}

var _ engine.CodeSnip = &codeSnipProviderClient{}
