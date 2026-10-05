package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"

	"vswitch/pkg/metrics"
	pb "vswitch/tunnel"
)

type Port struct {
	Conn      net.Conn
	UDPConn   *net.UDPConn
	UDPAddr   *net.UDPAddr
	GRPC      pb.TunnelService_StreamServer
	Send      chan []byte
	Ctx       context.Context
	Cancel    context.CancelFunc
	Mode      string
	once      sync.Once
	Metrics   *metrics.Metrics
	EmuKey    string
	ShapeHook func([]byte) bool
}

func NewStreamPort(c net.Conn, m *metrics.Metrics) *Port {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Port{Conn: c, Send: make(chan []byte, 4096), Ctx: ctx, Cancel: cancel, Mode: "stream", Metrics: m}
	go p.writeStream()
	return p
}
func NewUDPPort(c *net.UDPConn, a *net.UDPAddr, m *metrics.Metrics) *Port {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Port{UDPConn: c, UDPAddr: a, Send: make(chan []byte, 4096), Ctx: ctx, Cancel: cancel, Mode: "udp", Metrics: m}
	go p.writeUDP()
	return p
}
func NewGRPCPort(s pb.TunnelService_StreamServer, m *metrics.Metrics) *Port {
	ctx, cancel := context.WithCancel(s.Context())
	p := &Port{GRPC: s, Send: make(chan []byte, 4096), Ctx: ctx, Cancel: cancel, Mode: "grpc", Metrics: m}
	go p.writeGRPC()
	return p
}
func (p *Port) Close() {
	p.once.Do(func() {
		p.Cancel()
		if p.Conn != nil {
			_ = p.Conn.Close()
		}
	})
}
func (p *Port) Queue(frame []byte) {
	n := len(frame)
	if n == 0 {
		return
	}
	if p.ShapeHook != nil && p.ShapeHook(frame) {
		return
	}
	p.SendRaw(frame)
}

// SendRaw pushes a frame straight onto the send channel, bypassing any emulation hook.
func (p *Port) SendRaw(frame []byte) {
	n := len(frame)
	if n == 0 {
		return
	}
	var out []byte
	if p.Mode == "stream" {
		out = make([]byte, n+4)
		binary.BigEndian.PutUint32(out[:4], uint32(n))
		copy(out[4:], frame)
	} else {
		out = make([]byte, n)
		copy(out, frame)
	}
	select {
	case p.Send <- out:
	default:
	}
}
func (p *Port) writeStream() {
	defer p.Close()
	w := bufio.NewWriterSize(p.Conn, 64*1024)
	for {
		select {
		case b, ok := <-p.Send:
			if !ok {
				return
			}
			if _, e := w.Write(b); e != nil {
				return
			}
			if len(p.Send) == 0 {
				if e := w.Flush(); e != nil {
					return
				}
			}
			p.Metrics.AddTx(len(b))
		case <-p.Ctx.Done():
			return
		}
	}
}
func (p *Port) writeUDP() {
	for {
		select {
		case b, ok := <-p.Send:
			if !ok {
				return
			}
			if _, e := p.UDPConn.WriteToUDP(b, p.UDPAddr); e != nil {
				return
			}
			p.Metrics.AddTx(len(b))
		case <-p.Ctx.Done():
			return
		}
	}
}
func (p *Port) writeGRPC() {
	for {
		select {
		case b, ok := <-p.Send:
			if !ok {
				return
			}
			if e := p.GRPC.Send(&pb.Frame{Payload: b}); e != nil {
				return
			}
			p.Metrics.AddTx(len(b))
		case <-p.Ctx.Done():
			return
		}
	}
}
func ReadLengthPrefixed(r io.Reader, max int) ([]byte, error) {
	var h [4]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return nil, e
	}
	n := int(binary.BigEndian.Uint32(h[:]))
	if n < 14 || n > max {
		return nil, io.ErrShortBuffer
	}
	b := make([]byte, n)
	_, e := io.ReadFull(r, b)
	return b, e
}
