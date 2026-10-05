package transport

import (
	"context"
	"net"
	"vswitch/pkg/core"
)

type UDSAbstract struct {
	Name     string
	Switch   *core.L2Switch
	Listener net.Listener
}

func (u *UDSAbstract) Start(ctx context.Context) error {
	l, e := net.Listen("unix", u.Name)
	if e != nil {
		return e
	}
	u.Listener = l
	go func() { <-ctx.Done(); _ = l.Close() }()
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go u.Switch.ProcessStream(ctx, c)
		}
	}()
	return nil
}
func (u *UDSAbstract) Close() {
	if u.Listener != nil {
		_ = u.Listener.Close()
	}
}
