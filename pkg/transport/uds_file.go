package transport

import (
	"context"
	"net"
	"os"
	"vswitch/pkg/core"
)

type UDSFile struct {
	Path     string
	Switch   *core.L2Switch
	Listener net.Listener
}

func (u *UDSFile) Start(ctx context.Context) error {
	_ = os.Remove(u.Path)
	l, e := net.Listen("unix", u.Path)
	if e != nil {
		return e
	}
	u.Listener = l
	_ = os.Chmod(u.Path, 0777)
	go func() { <-ctx.Done(); _ = l.Close(); _ = os.Remove(u.Path) }()
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
func (u *UDSFile) Close() {
	if u.Listener != nil {
		_ = u.Listener.Close()
	}
	_ = os.Remove(u.Path)
}
