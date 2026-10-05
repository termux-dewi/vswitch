// Code generated for vswitch/tunnel.proto. DO NOT EDIT.
package tunnel

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

const _ = grpc.SupportPackageIsVersion9

type TunnelServiceClient interface {
	Stream(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[Frame, Frame], error)
}
type tunnelServiceClient struct{ cc grpc.ClientConnInterface }

func NewTunnelServiceClient(cc grpc.ClientConnInterface) TunnelServiceClient {
	return &tunnelServiceClient{cc}
}
func (c *tunnelServiceClient) Stream(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[Frame, Frame], error) {
	stream, err := c.cc.NewStream(ctx, &TunnelService_ServiceDesc.Streams[0], "/tunnel.TunnelService/Stream", opts...)
	if err != nil {
		return nil, err
	}
	return &grpc.GenericClientStream[Frame, Frame]{ClientStream: stream}, nil
}

type TunnelServiceServer interface {
	Stream(TunnelService_StreamServer) error
	mustEmbedUnimplementedTunnelServiceServer()
}
type UnimplementedTunnelServiceServer struct{}

func (UnimplementedTunnelServiceServer) Stream(TunnelService_StreamServer) error {
	return status.Error(codes.Unimplemented, "method Stream not implemented")
}
func (UnimplementedTunnelServiceServer) mustEmbedUnimplementedTunnelServiceServer() {}

type UnsafeTunnelServiceServer interface{ mustEmbedUnimplementedTunnelServiceServer() }
type TunnelService_StreamServer interface {
	Send(*Frame) error
	Recv() (*Frame, error)
	grpc.ServerStream
}

func RegisterTunnelServiceServer(s grpc.ServiceRegistrar, srv TunnelServiceServer) {
	s.RegisterService(&TunnelService_ServiceDesc, srv)
}
func _TunnelService_Stream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(TunnelServiceServer).Stream(&tunnelServiceStreamServer{ServerStream: stream})
}

type tunnelServiceStreamServer struct{ grpc.ServerStream }

func (x *tunnelServiceStreamServer) Send(m *Frame) error { return x.ServerStream.SendMsg(m) }
func (x *tunnelServiceStreamServer) Recv() (*Frame, error) {
	m := new(Frame)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var TunnelService_ServiceDesc = grpc.ServiceDesc{ServiceName: "tunnel.TunnelService", HandlerType: (*TunnelServiceServer)(nil), Methods: []grpc.MethodDesc{}, Streams: []grpc.StreamDesc{{StreamName: "Stream", Handler: _TunnelService_Stream_Handler, ServerStreams: true, ClientStreams: true}}, Metadata: "tunnel.proto"}
