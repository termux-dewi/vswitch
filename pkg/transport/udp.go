package transport

import (
	"context"
	"encoding/binary"
	"net"
	"vswitch/pkg/core"
)

type UDP struct {
	Addr   string
	Switch *core.L2Switch
	Conn   *net.UDPConn
}

func (u *UDP) Start(ctx context.Context) error {
	a, e := net.ResolveUDPAddr("udp", u.Addr)
	if e != nil {
		return e
	}
	c, e := net.ListenUDP("udp", a)
	if e != nil {
		return e
	}
	u.Conn = c
	_ = c.SetReadBuffer(2 * 1024 * 1024)
	_ = c.SetWriteBuffer(2 * 1024 * 1024)
	go func() { <-ctx.Done(); _ = c.Close() }()
	go u.loop()
	return nil
}
func (u *UDP) loop() {
	buf := make([]byte, 2048)
	for {
		n, a, e := u.Conn.ReadFromUDP(buf)
		if e != nil {
			return
		}
		if n < 14 {
			continue
		}
		p := u.Switch.RegisterUDP(u.Conn, a)
		frame := buf[:n]
		if n >= 18 && binary.BigEndian.Uint32(buf[:4]) == uint32(n-4) {
			frame = buf[4:n]
		}
		b := make([]byte, len(frame))
		copy(b, frame)
		u.Switch.Forward(b, p)
	}
}
func (u *UDP) Close() {
	if u.Conn != nil {
		_ = u.Conn.Close()
	}
}
