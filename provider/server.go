package provider

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/go-logr/logr"
	libgrpc "github.com/konveyor/analyzer-lsp/provider/lib/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type Server interface {
	// This will start the GRPC server and will wait until the context is cancelled.
	Start(context.Context) error
}

type server struct {
	Client Client
	Log    logr.Logger
	Port   int
	libgrpc.UnimplementedProviderServiceServer
}

func NewServer(client Client, port int, logger logr.Logger) Server {
	return &server{
		Client:                             client,
		Port:                               port,
		Log:                                logger,
		UnimplementedProviderServiceServer: libgrpc.UnimplementedProviderServiceServer{},
	}
}

func (s *server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		s.Log.Error(err, "failed to listen")
		return err
	}
	gs := grpc.NewServer()
	libgrpc.RegisterProviderServiceServer(gs, s)
	reflection.Register(gs)
	log.Printf("server listening at %v", lis.Addr())
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
	return nil
}

func (s *server) Capabilities(ctx context.Context, _ *emptypb.Empty) (*libgrpc.CapabilitiesResponse, error) {
	caps := s.Client.Capabilities()

	var pbCaps []*libgrpc.Capability

	for _, c := range caps {
		pbCaps = append(pbCaps, &libgrpc.Capability{
			Name: c.Name,
		})
	}

	return &libgrpc.CapabilitiesResponse{
		Capabilities: pbCaps,
	}, nil
}

func (s *server) HasCapability(ctx context.Context, hcr *libgrpc.HasCapabilityRequest) (*libgrpc.HasCapabilityResponse, error) {
	caps := s.Client.Capabilities()

	for _, c := range caps {
		if c.Name == hcr.Capability {
			return &libgrpc.HasCapabilityResponse{
				HasCap: true,
			}, nil
		}
	}
	return &libgrpc.HasCapabilityResponse{
		HasCap: false,
	}, nil
}

func (s *server) Init(ctx context.Context, _ *emptypb.Empty) (*libgrpc.BasicResponse, error) {
	err := s.Client.Init(ctx, s.Log)
	if err != nil {
		return &libgrpc.BasicResponse{
			Error:      err.Error(),
			Successful: false,
		}, nil
	}

	return &libgrpc.BasicResponse{
		Successful: true,
	}, nil
}

func (s *server) Evaluate(ctx context.Context, req *libgrpc.EvaluateRequest) (*libgrpc.EvaluateResponse, error) {
	r, err := s.Client.Evaluate(req.Cap, []byte(req.ConditionInfo))
	if err != nil {
		return &libgrpc.EvaluateResponse{
			Error:      err.Error(),
			Successful: false,
		}, nil
	}

	templateContext, err := structpb.NewStruct(r.TemplateContext)
	if err != nil {
		return &libgrpc.EvaluateResponse{
			Error:      err.Error(),
			Successful: false,
		}, nil
	}

	resp := libgrpc.ProviderEvaluateResponse{
		Matched:         r.Matched,
		TemplateContext: templateContext,
	}

	incs := []*libgrpc.IncidentContext{}

	for _, i := range r.Incidents {
		links := []*libgrpc.ExternalLink{}
		for _, l := range i.Links {
			links = append(links, &libgrpc.ExternalLink{
				Url:   l.URL,
				Title: l.Title,
			})
		}

		variables, err := structpb.NewStruct(i.Variables)
		if err != nil {
			return &libgrpc.EvaluateResponse{
				Error:      err.Error(),
				Successful: false,
			}, nil
		}

		inc := &libgrpc.IncidentContext{
			FileURI:   string(i.FileURI),
			Variables: variables,
			Links:     links,
		}
		if i.Effort != nil {
			num := int64(*i.Effort)
			inc.Effort = &num
		}
		if i.CodeLocation != nil {
			inc.CodeLocation = &libgrpc.Location{
				StartPosition: &libgrpc.Position{
					Line:      i.CodeLocation.StartPosition.Line,
					Character: i.CodeLocation.StartPosition.Character,
				},
				EndPosition: &libgrpc.Position{
					Line:      i.CodeLocation.EndPosition.Line,
					Character: i.CodeLocation.EndPosition.Character,
				},
			}
		}
		incs = append(incs, inc)
	}

	resp.IncidentContexts = incs

	return &libgrpc.EvaluateResponse{
		Response:   &resp,
		Successful: true,
	}, nil
}

func (s *server) Stop(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	s.Client.Stop()
	return &emptypb.Empty{}, nil
}
