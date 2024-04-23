package provider

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	libgrpc "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
	"go.lsp.dev/uri"
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
	Client              BaseClient
	CodeSnipeResolver   engine.CodeSnip
	DepLocationResolver DependencyLocationResolver
	Log                 logr.Logger
	Port                int

	mutex   sync.RWMutex
	clients map[int64]clientMapItem
	rand    rand.Rand
	libgrpc.UnimplementedProviderCodeLocationServiceServer
	libgrpc.UnimplementedProviderDependencyLocationServiceServer
	libgrpc.UnimplementedProviderServiceServer
}

type clientMapItem struct {
	ctx    context.Context
	client ServiceClient
}

// Provider GRPC Service
// TOOD: HANDLE INIT CONFIG CHANGES
func NewServer(client BaseClient, port int, logger logr.Logger) Server {
	s := rand.NewSource(time.Now().Unix())

	var depLocationResolver DependencyLocationResolver
	var codeSnip engine.CodeSnip
	var ok bool
	depLocationResolver, ok = client.(DependencyLocationResolver)
	if !ok {
		depLocationResolver = nil
	}

	codeSnip, ok = client.(engine.CodeSnip)
	if !ok {
		codeSnip = nil
	}

	return &server{
		Client:                             client,
		Port:                               port,
		Log:                                logger,
		UnimplementedProviderServiceServer: libgrpc.UnimplementedProviderServiceServer{},
		mutex:                              sync.RWMutex{},
		clients:                            make(map[int64]clientMapItem),
		rand:                               *rand.New(s),
		DepLocationResolver:                depLocationResolver,
		CodeSnipeResolver:                  codeSnip,
	}
}

func (s *server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		s.Log.Error(err, "failed to listen")
		return err
	}
	gs := grpc.NewServer()
	if s.DepLocationResolver != nil {
		libgrpc.RegisterProviderDependencyLocationServiceServer(gs, s)
	}
	if s.CodeSnipeResolver != nil {
		libgrpc.RegisterProviderCodeLocationServiceServer(gs, s)
	}
	libgrpc.RegisterProviderServiceServer(gs, s)
	reflection.Register(gs)
	log.Printf("server listening at %v", lis.Addr())
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
	return nil
}

func (s *server) GetDependencyLocation(ctx context.Context, req *libgrpc.GetDependencyLocationRequest) (*libgrpc.GetDependencyLocationResponse, error) {
	if s.DepLocationResolver == nil {
		return nil, fmt.Errorf("Provider does not provide Dependency Location Resolution")
	}
	res, err := s.DepLocationResolver.GetLocation(ctx, konveyor.Dep{
		Name:               req.Dep.Name,
		Version:            req.Dep.Version,
		Classifier:         req.Dep.Classifier,
		Type:               req.Dep.Type,
		Indirect:           req.Dep.Indirect,
		ResolvedIdentifier: req.Dep.ResolvedIdentifier,
		Extras:             req.Dep.Extras.AsMap(),
		Labels:             req.Dep.Labels,
		FileURIPrefix:      req.Dep.FileURIPrefix,
	}, req.DepFile)
	if err != nil {
		return nil, err
	}

	return &libgrpc.GetDependencyLocationResponse{
		Location: &libgrpc.Location{
			StartPosition: &libgrpc.Position{
				Line:      float64(res.StartPosition.Line),
				Character: float64(res.StartPosition.Character),
			},
			EndPosition: &libgrpc.Position{
				Line:      float64(res.EndPosition.Line),
				Character: float64(res.EndPosition.Character),
			},
		},
	}, nil
}

func (s *server) GetCodeSnip(ctx context.Context, req *libgrpc.GetCodeSnipRequest) (*libgrpc.GetCodeSnipResponse, error) {
	if s.CodeSnipeResolver == nil {
		return nil, fmt.Errorf("Provider does not provide Code Snippet Resolution")
	}
	if req.CodeLocation == nil {
		return nil, nil

	}
	loc := engine.Location{}
	if req.CodeLocation.StartPosition != nil {
		loc.StartPosition = engine.Position{
			Line:      int(req.CodeLocation.StartPosition.Line),
			Character: int(req.CodeLocation.StartPosition.Character),
		}
	}
	if req.CodeLocation.EndPosition != nil {
		loc.EndPosition = engine.Position{
			Line:      int(req.CodeLocation.EndPosition.Line),
			Character: int(req.CodeLocation.EndPosition.Character),
		}
	}

	res, err := s.CodeSnipeResolver.GetCodeSnip(uri.URI(req.Uri), loc)
	if err != nil {
		return nil, err
	}
	return &libgrpc.GetCodeSnipResponse{
		Snip: res,
	}, nil
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

func (s *server) Init(ctx context.Context, config *libgrpc.Config) (*libgrpc.InitResponse, error) {
	//By default if nothing is set for analysis mode, in the config, we should default to full for external providers
	var a AnalysisMode = AnalysisMode(config.AnalysisMode)
	if a == AnalysisMode("") {
		a = FullAnalysisMode
	} else if !(a == FullAnalysisMode || a == SourceOnlyAnalysisMode) {
		return nil, fmt.Errorf("invalid Analysis Mode")
	}

	c := InitConfig{
		Location:       config.Location,
		DependencyPath: config.DependencyPath,
		AnalysisMode:   a,
		Proxy: &Proxy{
			HTTPProxy:  config.Proxy.HTTPProxy,
			HTTPSProxy: config.Proxy.HTTPSProxy,
			NoProxy:    config.Proxy.NoProxy,
		},
	}

	if config.ProviderSpecificConfig != nil {
		c.ProviderSpecificConfig = config.ProviderSpecificConfig.AsMap()
	}

	id := rand.Int63()
	log := s.Log.WithValues("client", id)
	newCtx := context.Background()

	client, err := s.Client.Init(newCtx, log, c)
	if err != nil {
		return &libgrpc.InitResponse{
			Error:      err.Error(),
			Successful: false,
		}, nil
	}
	s.mutex.Lock()
	s.clients[id] = clientMapItem{
		client: client,
		ctx:    ctx,
	}
	s.mutex.Unlock()

	return &libgrpc.InitResponse{
		Id:         id,
		Successful: true,
	}, nil
}

func (s *server) Evaluate(ctx context.Context, req *libgrpc.EvaluateRequest) (*libgrpc.EvaluateResponse, error) {

	s.mutex.RLock()
	client := s.clients[req.Id]
	s.mutex.RUnlock()

	r, err := client.client.Evaluate(ctx, req.Cap, []byte(req.ConditionInfo))

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
			FileURI:              string(i.FileURI),
			Variables:            variables,
			Links:                links,
			IsDependencyIncident: i.IsDependencyIncident,
		}
		if i.LineNumber != nil {
			lineNumber := int64(*i.LineNumber)
			inc.LineNumber = &lineNumber
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

func (s *server) Stop(ctx context.Context, in *libgrpc.ServiceRequest) (*emptypb.Empty, error) {
	s.mutex.Lock()
	client := s.clients[in.Id]
	delete(s.clients, in.Id)
	s.mutex.Unlock()
	client.client.Stop()
	return &emptypb.Empty{}, nil
}

func (s *server) GetDependencies(ctx context.Context, in *libgrpc.ServiceRequest) (*libgrpc.DependencyResponse, error) {
	s.mutex.RLock()
	client := s.clients[in.Id]
	s.mutex.RUnlock()
	deps, err := client.client.GetDependencies(ctx)
	if err != nil {
		return &libgrpc.DependencyResponse{
			Successful: false,
			Error:      err.Error(),
		}, nil
	}
	fileDeps := []*libgrpc.FileDep{}
	for f, ds := range deps {
		fd := libgrpc.FileDep{
			FileURI: string(f),
		}
		deps := []*libgrpc.Dependency{}
		for _, d := range ds {
			extras, err := structpb.NewStruct(d.Extras)
			if err != nil {
				return nil, err
			}
			deps = append(deps, &libgrpc.Dependency{
				Name:               d.Name,
				Version:            d.Version,
				Classifier:         d.Classifier,
				Type:               d.Type,
				ResolvedIdentifier: d.ResolvedIdentifier,
				Extras:             extras,
				Indirect:           d.Indirect,
				Labels:             d.Labels,
				FileURIPrefix:      d.FileURIPrefix,
			})
		}
		fd.List = &libgrpc.DependencyList{
			Deps: deps,
		}
		fileDeps = append(fileDeps, &fd)
	}

	return &libgrpc.DependencyResponse{
		Successful: true,
		FileDep:    fileDeps,
	}, nil
}

func recreateDAGAddedItems(items []DepDAGItem) []*libgrpc.DependencyDAGItem {
	deps := []*libgrpc.DependencyDAGItem{}
	for _, i := range items {
		extras, err := structpb.NewStruct(i.Dep.Extras)
		if err != nil {
			panic(err)
		}
		deps = append(deps, &libgrpc.DependencyDAGItem{
			Key: &libgrpc.Dependency{
				Name:               i.Dep.Name,
				Version:            i.Dep.Version,
				Classifier:         i.Dep.Classifier,
				Type:               i.Dep.Type,
				ResolvedIdentifier: i.Dep.ResolvedIdentifier,
				Extras:             extras,
				Labels:             i.Dep.Labels,
				Indirect:           false,
				FileURIPrefix:      i.Dep.FileURIPrefix,
			},
			AddedDeps: recreateDAGAddedItems(i.AddedDeps),
		})
	}
	return deps
}

func (s *server) GetDependenciesLinkedList(ctx context.Context, in *libgrpc.ServiceRequest) (*libgrpc.DependencyDAGResponse, error) {
	s.mutex.RLock()
	client := s.clients[in.Id]
	s.mutex.RUnlock()
	deps, err := client.client.GetDependenciesDAG(ctx)
	if err != nil {
		return &libgrpc.DependencyDAGResponse{
			Successful: false,
			Error:      err.Error(),
		}, nil
	}
	fileDagDeps := []*libgrpc.FileDAGDep{}
	for f, ds := range deps {
		l := recreateDAGAddedItems(ds)
		fileDagDeps = append(fileDagDeps, &libgrpc.FileDAGDep{
			FileURI: string(f),
			List:    l,
		})

	}

	return &libgrpc.DependencyDAGResponse{
		Successful: true,
		FileDagDep: fileDagDeps,
	}, nil
}
