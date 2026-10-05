package netstack

import (
	"context"
	"encoding/binary"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"runtime"
	"vswitch/pkg/core"
)

type Bridge struct {
	Stack      *stack.Stack
	Endpoint   *channel.Endpoint
	Switch     *core.L2Switch
	GatewayIP  [4]byte
	GatewayMAC core.MACAddr
}

func New(sw *core.L2Switch, ip [4]byte, mac core.MACAddr) *Bridge {
	ep := channel.New(2048, 1500, tcpip.LinkAddress(mac[:]))
	st := stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol}, TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol4}})
	_ = st.CreateNIC(1, ep)
	_ = st.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: tcpip.AddrFrom4(ip).WithPrefix()}, stack.AddressProperties{})
	st.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})
	sack := tcpip.TCPSACKEnabled(true)
	_ = st.SetTransportProtocolOption(tcp.ProtocolNumber, &sack)
	mod := tcpip.TCPModerateReceiveBufferOption(true)
	_ = st.SetTransportProtocolOption(tcp.ProtocolNumber, &mod)
	return &Bridge{Stack: st, Endpoint: ep, Switch: sw, GatewayIP: ip, GatewayMAC: mac}
}
func (b *Bridge) Inject(payload []byte) {
	if len(payload) < 20 {
		return
	}
	p := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(payload)})
	b.Endpoint.InjectInbound(ipv4.ProtocolNumber, p)
}
func (b *Bridge) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			pkt := b.Endpoint.Read()
			if pkt == nil {
				runtime.Gosched()
				continue
			}
			n := pkt.NetworkHeader().Slice()
			if len(n) < 20 {
				continue
			}
			dst := header.IPv4(n).DestinationAddress().As4()
			buf := pkt.ToBuffer()
			b.Switch.DeliverIP(buf.Flatten(), dst)
		}
	}()
}
func BuildL2(dst, src core.MACAddr, payload []byte) []byte {
	b := make([]byte, 14+len(payload))
	copy(b[:6], dst[:])
	copy(b[6:12], src[:])
	binary.BigEndian.PutUint16(b[12:14], 0x0800)
	copy(b[14:], payload)
	return b
}
