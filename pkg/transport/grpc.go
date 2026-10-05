package transport

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"vswitch/pkg/core"
	pb "vswitch/tunnel"
)

type GRPC struct {
	pb.UnimplementedTunnelServiceServer
	Addr     string
	TLSCert  string
	TLSKey   string
	Switch   *core.L2Switch
	Server   *grpc.Server
	Listener net.Listener
}

func (g *GRPC) Start(ctx context.Context) error {
	l, e := net.Listen("tcp", g.Addr)
	if e != nil {
		return e
	}
	g.Listener = l
	opts := []grpc.ServerOption{grpc.KeepaliveParams(keepalive.ServerParameters{MaxConnectionIdle: 0, Time: 10, Timeout: 5})}
	if g.TLSCert != "" && g.TLSKey != "" {
		creds, e := credentials.NewServerTLSFromFile(g.TLSCert, g.TLSKey)
		if e != nil {
			return e
		}
		opts = append(opts, grpc.Creds(creds))
	}
	g.Server = grpc.NewServer(opts...)
	pb.RegisterTunnelServiceServer(g.Server, g)
	go func() { <-ctx.Done(); g.Server.GracefulStop() }()
	go g.Server.Serve(l)
	return nil
}
func (g *GRPC) Stream(s pb.TunnelService_StreamServer) error { return g.Switch.Stream(s) }
func (g *GRPC) Close() {
	if g.Server != nil {
		g.Server.GracefulStop()
	}
	if g.Listener != nil {
		_ = g.Listener.Close()
	}
}
