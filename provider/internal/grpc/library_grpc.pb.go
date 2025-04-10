// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v6.30.2
// source: provider/internal/grpc/library.proto

package grpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	ProviderCodeLocationService_GetCodeSnip_FullMethodName = "/provider.ProviderCodeLocationService/GetCodeSnip"
)

// ProviderCodeLocationServiceClient is the client API for ProviderCodeLocationService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ProviderCodeLocationServiceClient interface {
	GetCodeSnip(ctx context.Context, in *GetCodeSnipRequest, opts ...grpc.CallOption) (*GetCodeSnipResponse, error)
}

type providerCodeLocationServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewProviderCodeLocationServiceClient(cc grpc.ClientConnInterface) ProviderCodeLocationServiceClient {
	return &providerCodeLocationServiceClient{cc}
}

func (c *providerCodeLocationServiceClient) GetCodeSnip(ctx context.Context, in *GetCodeSnipRequest, opts ...grpc.CallOption) (*GetCodeSnipResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetCodeSnipResponse)
	err := c.cc.Invoke(ctx, ProviderCodeLocationService_GetCodeSnip_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ProviderCodeLocationServiceServer is the server API for ProviderCodeLocationService service.
// All implementations must embed UnimplementedProviderCodeLocationServiceServer
// for forward compatibility.
type ProviderCodeLocationServiceServer interface {
	GetCodeSnip(context.Context, *GetCodeSnipRequest) (*GetCodeSnipResponse, error)
	mustEmbedUnimplementedProviderCodeLocationServiceServer()
}

// UnimplementedProviderCodeLocationServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedProviderCodeLocationServiceServer struct{}

func (UnimplementedProviderCodeLocationServiceServer) GetCodeSnip(context.Context, *GetCodeSnipRequest) (*GetCodeSnipResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCodeSnip not implemented")
}
func (UnimplementedProviderCodeLocationServiceServer) mustEmbedUnimplementedProviderCodeLocationServiceServer() {
}
func (UnimplementedProviderCodeLocationServiceServer) testEmbeddedByValue() {}

// UnsafeProviderCodeLocationServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ProviderCodeLocationServiceServer will
// result in compilation errors.
type UnsafeProviderCodeLocationServiceServer interface {
	mustEmbedUnimplementedProviderCodeLocationServiceServer()
}

func RegisterProviderCodeLocationServiceServer(s grpc.ServiceRegistrar, srv ProviderCodeLocationServiceServer) {
	// If the following call pancis, it indicates UnimplementedProviderCodeLocationServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&ProviderCodeLocationService_ServiceDesc, srv)
}

func _ProviderCodeLocationService_GetCodeSnip_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetCodeSnipRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderCodeLocationServiceServer).GetCodeSnip(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderCodeLocationService_GetCodeSnip_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderCodeLocationServiceServer).GetCodeSnip(ctx, req.(*GetCodeSnipRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ProviderCodeLocationService_ServiceDesc is the grpc.ServiceDesc for ProviderCodeLocationService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ProviderCodeLocationService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "provider.ProviderCodeLocationService",
	HandlerType: (*ProviderCodeLocationServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetCodeSnip",
			Handler:    _ProviderCodeLocationService_GetCodeSnip_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "provider/internal/grpc/library.proto",
}

const (
	ProviderDependencyLocationService_GetDependencyLocation_FullMethodName = "/provider.ProviderDependencyLocationService/GetDependencyLocation"
)

// ProviderDependencyLocationServiceClient is the client API for ProviderDependencyLocationService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ProviderDependencyLocationServiceClient interface {
	GetDependencyLocation(ctx context.Context, in *GetDependencyLocationRequest, opts ...grpc.CallOption) (*GetDependencyLocationResponse, error)
}

type providerDependencyLocationServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewProviderDependencyLocationServiceClient(cc grpc.ClientConnInterface) ProviderDependencyLocationServiceClient {
	return &providerDependencyLocationServiceClient{cc}
}

func (c *providerDependencyLocationServiceClient) GetDependencyLocation(ctx context.Context, in *GetDependencyLocationRequest, opts ...grpc.CallOption) (*GetDependencyLocationResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetDependencyLocationResponse)
	err := c.cc.Invoke(ctx, ProviderDependencyLocationService_GetDependencyLocation_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ProviderDependencyLocationServiceServer is the server API for ProviderDependencyLocationService service.
// All implementations must embed UnimplementedProviderDependencyLocationServiceServer
// for forward compatibility.
type ProviderDependencyLocationServiceServer interface {
	GetDependencyLocation(context.Context, *GetDependencyLocationRequest) (*GetDependencyLocationResponse, error)
	mustEmbedUnimplementedProviderDependencyLocationServiceServer()
}

// UnimplementedProviderDependencyLocationServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedProviderDependencyLocationServiceServer struct{}

func (UnimplementedProviderDependencyLocationServiceServer) GetDependencyLocation(context.Context, *GetDependencyLocationRequest) (*GetDependencyLocationResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDependencyLocation not implemented")
}
func (UnimplementedProviderDependencyLocationServiceServer) mustEmbedUnimplementedProviderDependencyLocationServiceServer() {
}
func (UnimplementedProviderDependencyLocationServiceServer) testEmbeddedByValue() {}

// UnsafeProviderDependencyLocationServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ProviderDependencyLocationServiceServer will
// result in compilation errors.
type UnsafeProviderDependencyLocationServiceServer interface {
	mustEmbedUnimplementedProviderDependencyLocationServiceServer()
}

func RegisterProviderDependencyLocationServiceServer(s grpc.ServiceRegistrar, srv ProviderDependencyLocationServiceServer) {
	// If the following call pancis, it indicates UnimplementedProviderDependencyLocationServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&ProviderDependencyLocationService_ServiceDesc, srv)
}

func _ProviderDependencyLocationService_GetDependencyLocation_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDependencyLocationRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderDependencyLocationServiceServer).GetDependencyLocation(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderDependencyLocationService_GetDependencyLocation_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderDependencyLocationServiceServer).GetDependencyLocation(ctx, req.(*GetDependencyLocationRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ProviderDependencyLocationService_ServiceDesc is the grpc.ServiceDesc for ProviderDependencyLocationService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ProviderDependencyLocationService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "provider.ProviderDependencyLocationService",
	HandlerType: (*ProviderDependencyLocationServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetDependencyLocation",
			Handler:    _ProviderDependencyLocationService_GetDependencyLocation_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "provider/internal/grpc/library.proto",
}

const (
	ProviderService_Capabilities_FullMethodName       = "/provider.ProviderService/Capabilities"
	ProviderService_Init_FullMethodName               = "/provider.ProviderService/Init"
	ProviderService_Evaluate_FullMethodName           = "/provider.ProviderService/Evaluate"
	ProviderService_Stop_FullMethodName               = "/provider.ProviderService/Stop"
	ProviderService_GetDependencies_FullMethodName    = "/provider.ProviderService/GetDependencies"
	ProviderService_GetDependenciesDAG_FullMethodName = "/provider.ProviderService/GetDependenciesDAG"
	ProviderService_NotifyFileChanges_FullMethodName  = "/provider.ProviderService/NotifyFileChanges"
)

// ProviderServiceClient is the client API for ProviderService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ProviderServiceClient interface {
	Capabilities(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*CapabilitiesResponse, error)
	Init(ctx context.Context, in *Config, opts ...grpc.CallOption) (*InitResponse, error)
	Evaluate(ctx context.Context, in *EvaluateRequest, opts ...grpc.CallOption) (*EvaluateResponse, error)
	Stop(ctx context.Context, in *ServiceRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	GetDependencies(ctx context.Context, in *ServiceRequest, opts ...grpc.CallOption) (*DependencyResponse, error)
	GetDependenciesDAG(ctx context.Context, in *ServiceRequest, opts ...grpc.CallOption) (*DependencyDAGResponse, error)
	NotifyFileChanges(ctx context.Context, in *NotifyFileChangesRequest, opts ...grpc.CallOption) (*NotifyFileChangesResponse, error)
}

type providerServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewProviderServiceClient(cc grpc.ClientConnInterface) ProviderServiceClient {
	return &providerServiceClient{cc}
}

func (c *providerServiceClient) Capabilities(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*CapabilitiesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CapabilitiesResponse)
	err := c.cc.Invoke(ctx, ProviderService_Capabilities_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *providerServiceClient) Init(ctx context.Context, in *Config, opts ...grpc.CallOption) (*InitResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InitResponse)
	err := c.cc.Invoke(ctx, ProviderService_Init_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *providerServiceClient) Evaluate(ctx context.Context, in *EvaluateRequest, opts ...grpc.CallOption) (*EvaluateResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(EvaluateResponse)
	err := c.cc.Invoke(ctx, ProviderService_Evaluate_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *providerServiceClient) Stop(ctx context.Context, in *ServiceRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, ProviderService_Stop_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *providerServiceClient) GetDependencies(ctx context.Context, in *ServiceRequest, opts ...grpc.CallOption) (*DependencyResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(DependencyResponse)
	err := c.cc.Invoke(ctx, ProviderService_GetDependencies_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *providerServiceClient) GetDependenciesDAG(ctx context.Context, in *ServiceRequest, opts ...grpc.CallOption) (*DependencyDAGResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(DependencyDAGResponse)
	err := c.cc.Invoke(ctx, ProviderService_GetDependenciesDAG_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *providerServiceClient) NotifyFileChanges(ctx context.Context, in *NotifyFileChangesRequest, opts ...grpc.CallOption) (*NotifyFileChangesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(NotifyFileChangesResponse)
	err := c.cc.Invoke(ctx, ProviderService_NotifyFileChanges_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ProviderServiceServer is the server API for ProviderService service.
// All implementations must embed UnimplementedProviderServiceServer
// for forward compatibility.
type ProviderServiceServer interface {
	Capabilities(context.Context, *emptypb.Empty) (*CapabilitiesResponse, error)
	Init(context.Context, *Config) (*InitResponse, error)
	Evaluate(context.Context, *EvaluateRequest) (*EvaluateResponse, error)
	Stop(context.Context, *ServiceRequest) (*emptypb.Empty, error)
	GetDependencies(context.Context, *ServiceRequest) (*DependencyResponse, error)
	GetDependenciesDAG(context.Context, *ServiceRequest) (*DependencyDAGResponse, error)
	NotifyFileChanges(context.Context, *NotifyFileChangesRequest) (*NotifyFileChangesResponse, error)
	mustEmbedUnimplementedProviderServiceServer()
}

// UnimplementedProviderServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedProviderServiceServer struct{}

func (UnimplementedProviderServiceServer) Capabilities(context.Context, *emptypb.Empty) (*CapabilitiesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Capabilities not implemented")
}
func (UnimplementedProviderServiceServer) Init(context.Context, *Config) (*InitResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Init not implemented")
}
func (UnimplementedProviderServiceServer) Evaluate(context.Context, *EvaluateRequest) (*EvaluateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Evaluate not implemented")
}
func (UnimplementedProviderServiceServer) Stop(context.Context, *ServiceRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Stop not implemented")
}
func (UnimplementedProviderServiceServer) GetDependencies(context.Context, *ServiceRequest) (*DependencyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDependencies not implemented")
}
func (UnimplementedProviderServiceServer) GetDependenciesDAG(context.Context, *ServiceRequest) (*DependencyDAGResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDependenciesDAG not implemented")
}
func (UnimplementedProviderServiceServer) NotifyFileChanges(context.Context, *NotifyFileChangesRequest) (*NotifyFileChangesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NotifyFileChanges not implemented")
}
func (UnimplementedProviderServiceServer) mustEmbedUnimplementedProviderServiceServer() {}
func (UnimplementedProviderServiceServer) testEmbeddedByValue()                         {}

// UnsafeProviderServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ProviderServiceServer will
// result in compilation errors.
type UnsafeProviderServiceServer interface {
	mustEmbedUnimplementedProviderServiceServer()
}

func RegisterProviderServiceServer(s grpc.ServiceRegistrar, srv ProviderServiceServer) {
	// If the following call pancis, it indicates UnimplementedProviderServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&ProviderService_ServiceDesc, srv)
}

func _ProviderService_Capabilities_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).Capabilities(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_Capabilities_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).Capabilities(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _ProviderService_Init_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Config)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).Init(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_Init_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).Init(ctx, req.(*Config))
	}
	return interceptor(ctx, in, info, handler)
}

func _ProviderService_Evaluate_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EvaluateRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).Evaluate(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_Evaluate_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).Evaluate(ctx, req.(*EvaluateRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ProviderService_Stop_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ServiceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).Stop(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_Stop_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).Stop(ctx, req.(*ServiceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ProviderService_GetDependencies_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ServiceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).GetDependencies(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_GetDependencies_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).GetDependencies(ctx, req.(*ServiceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ProviderService_GetDependenciesDAG_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ServiceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).GetDependenciesDAG(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_GetDependenciesDAG_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).GetDependenciesDAG(ctx, req.(*ServiceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ProviderService_NotifyFileChanges_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(NotifyFileChangesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProviderServiceServer).NotifyFileChanges(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ProviderService_NotifyFileChanges_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProviderServiceServer).NotifyFileChanges(ctx, req.(*NotifyFileChangesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ProviderService_ServiceDesc is the grpc.ServiceDesc for ProviderService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ProviderService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "provider.ProviderService",
	HandlerType: (*ProviderServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Capabilities",
			Handler:    _ProviderService_Capabilities_Handler,
		},
		{
			MethodName: "Init",
			Handler:    _ProviderService_Init_Handler,
		},
		{
			MethodName: "Evaluate",
			Handler:    _ProviderService_Evaluate_Handler,
		},
		{
			MethodName: "Stop",
			Handler:    _ProviderService_Stop_Handler,
		},
		{
			MethodName: "GetDependencies",
			Handler:    _ProviderService_GetDependencies_Handler,
		},
		{
			MethodName: "GetDependenciesDAG",
			Handler:    _ProviderService_GetDependenciesDAG_Handler,
		},
		{
			MethodName: "NotifyFileChanges",
			Handler:    _ProviderService_NotifyFileChanges_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "provider/internal/grpc/library.proto",
}
