package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"google.golang.org/grpc/metadata"
	"hash/fnv"
	"net"
	"sync"

	"vswitch/pkg/dhcp"
	"vswitch/pkg/emulation"
	"vswitch/pkg/metrics"
	pb "vswitch/tunnel"
)

type NetstackBridge interface {
	Inject(frame []byte)
	Start(context.Context)
}

type L2Switch struct {
	mu        sync.RWMutex
	Ports     map[net.Conn]*Port
	UDPPorts  map[string]*Port
	GRPCPorts map[pb.TunnelService_StreamServer]*Port
	MACTable  map[MACAddr]*Port
	Pool      sync.Pool
	Metrics   *metrics.Metrics
	ARP       *dhcp.ARPHandler
	DHCP      *dhcp.Server
	Netstack  NetstackBridge
	Emulation *emulation.Manager
}

func New(m *metrics.Metrics, arp *dhcp.ARPHandler, d *dhcp.Server) *L2Switch {
	s := &L2Switch{Ports: map[net.Conn]*Port{}, UDPPorts: map[string]*Port{}, GRPCPorts: map[pb.TunnelService_StreamServer]*Port{}, MACTable: map[MACAddr]*Port{}, Metrics: m, ARP: arp, DHCP: d, Emulation: emulation.NewManager()}
	s.Pool.New = func() any { return make([]byte, 2048) }
	return s
}
func (s *L2Switch) Register(c net.Conn) *Port {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := NewStreamPort(c, s.Metrics)
	p.EmuKey = "uds:" + c.RemoteAddr().String()
	p.ShapeHook = s.emuHook(p.EmuKey)
	s.Ports[c] = p
	s.Metrics.PortOpened()
	return p
}
func (s *L2Switch) Unregister(c net.Conn) {
	s.mu.Lock()
	p, ok := s.Ports[c]
	if ok {
		delete(s.Ports, c)
	}
	for m, x := range s.MACTable {
		if x == p {
			delete(s.MACTable, m)
		}
	}
	s.mu.Unlock()
	if ok {
		s.Emulation.RemoveProfile(p.EmuKey)
		p.Close()
		s.Metrics.PortClosed()
	}
}
func (s *L2Switch) RegisterUDP(c *net.UDPConn, a *net.UDPAddr) *Port {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := a.String()
	if p, ok := s.UDPPorts[key]; ok {
		return p
	}
	p := NewUDPPort(c, a, s.Metrics)
	p.EmuKey = "udp:" + key
	p.ShapeHook = s.emuHook(p.EmuKey)
	s.UDPPorts[key] = p
	s.Metrics.PortOpened()
	return p
}
func (s *L2Switch) RegisterGRPC(st pb.TunnelService_StreamServer) *Port {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := NewGRPCPort(st, s.Metrics)
	p.EmuKey = fmt.Sprintf("grpc:%p", st)
	p.ShapeHook = s.emuHook(p.EmuKey)
	s.GRPCPorts[st] = p
	s.Metrics.PortOpened()
	return p
}
func (s *L2Switch) UnregisterGRPC(st pb.TunnelService_StreamServer) {
	s.mu.Lock()
	p, ok := s.GRPCPorts[st]
	if ok {
		delete(s.GRPCPorts, st)
	}
	for m, x := range s.MACTable {
		if x == p {
			delete(s.MACTable, m)
		}
	}
	s.mu.Unlock()
	if ok {
		s.Emulation.RemoveProfile(p.EmuKey)
		p.Close()
		s.Metrics.PortClosed()
	}
}
func (s *L2Switch) Forward(frame []byte, src *Port) {
	if len(frame) < 14 {
		return
	}
	s.Metrics.AddRx(len(frame))
	dst := ToMAC(frame[0:6])
	srcm := ToMAC(frame[6:12])
	s.mu.Lock()
	s.MACTable[srcm] = src
	s.mu.Unlock()
	if dst == (MACAddr{255, 255, 255, 255, 255, 255}) {
		if s.ARP != nil {
			if r := s.ARP.Handle(frame); r != nil {
				src.Queue(r)
				return
			}
		}
		if s.DHCP != nil {
			if r := s.DHCP.Handle(frame); r != nil {
				src.Queue(r)
				return
			}
		}
		s.Flood(frame, src)
		return
	}
	if s.ARP != nil && dst == MACAddr(s.ARP.GatewayMAC) && len(frame) >= 14 && binary.BigEndian.Uint16(frame[12:14]) == 0x0800 && s.Netstack != nil {
		s.Netstack.Inject(frame[14:])
		return
	}
	s.mu.RLock()
	p, ok := s.MACTable[dst]
	s.mu.RUnlock()
	if ok {
		p.Queue(frame)
	} else {
		s.Flood(frame, src)
	}
}
func (s *L2Switch) Flood(frame []byte, src *Port) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Ports {
		if p != src {
			p.Queue(frame)
		}
	}
	for _, p := range s.UDPPorts {
		if p != src {
			p.Queue(frame)
		}
	}
	for _, p := range s.GRPCPorts {
		if p != src {
			p.Queue(frame)
		}
	}
}
func (s *L2Switch) PersistentService(service string) (MACAddr, [4]byte) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(service))
	v := h.Sum64()
	m := MACAddr{2, byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	if s.DHCP != nil && s.DHCP.Store != nil {
		if l, ok := s.DHCP.Store.Get(dhcp.MACAddr(m)); ok {
			var ip [4]byte
			_, e := fmt.Sscanf(l.IP, "%d.%d.%d.%d", &ip[0], &ip[1], &ip[2], &ip[3])
			if e == nil {
				return m, ip
			}
		}
	}
	s.mu.RLock()
	n := len(s.MACTable)
	s.mu.RUnlock()
	return m, [4]byte{10, 0, 0, byte(10 + n)}
}
func (s *L2Switch) PortSummary() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int{"uds_file": len(s.Ports), "udp": len(s.UDPPorts), "grpc": len(s.GRPCPorts), "mac_table": len(s.MACTable)}
}
func (s *L2Switch) DeliverIP(payload []byte, dst [4]byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for mac, p := range s.MACTable {
		if ip, ok := s.DHCPIP(mac); ok && ip == dst {
			p.Queue(BuildFrame(mac, MACAddr(s.ARP.GatewayMAC), payload))
			return
		}
	}
}
func (s *L2Switch) DHCPIP(mac MACAddr) ([4]byte, bool) {
	if s.DHCP == nil || s.DHCP.Store == nil {
		return [4]byte{}, false
	}
	l, ok := s.DHCP.Store.Get(dhcp.MACAddr(mac))
	if !ok {
		return [4]byte{}, false
	}
	var ip [4]byte
	_, e := fmt.Sscanf(l.IP, "%d.%d.%d.%d", &ip[0], &ip[1], &ip[2], &ip[3])
	return ip, e == nil
}
func (s *L2Switch) emuHook(key string) func([]byte) bool {
	return func(frame []byte) bool {
		return s.Emulation.Shape(key, frame)
	}
}

// ApplyEmulation installs (or updates) an emulation profile on a port key.
func (s *L2Switch) ApplyEmulation(key string, p emulation.Profile) error {
	port := s.portByKey(key)
	if port == nil {
		return fmt.Errorf("unknown port key: %s", key)
	}
	if p.IsZero() {
		s.Emulation.RemoveProfile(key)
		return nil
	}
	s.Emulation.SetProfile(key, p, port.SendRaw)
	return nil
}

// RemoveEmulation clears emulation from a port key.
func (s *L2Switch) RemoveEmulation(key string) { s.Emulation.RemoveProfile(key) }

func (s *L2Switch) portByKey(key string) *Port {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Ports {
		if p.EmuKey == key {
			return p
		}
	}
	for _, p := range s.UDPPorts {
		if p.EmuKey == key {
			return p
		}
	}
	for _, p := range s.GRPCPorts {
		if p.EmuKey == key {
			return p
		}
	}
	return nil
}

// PortKeys returns all active port emulation keys.
func (s *L2Switch) PortKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.Ports)+len(s.UDPPorts)+len(s.GRPCPorts))
	for _, p := range s.Ports {
		keys = append(keys, p.EmuKey)
	}
	for _, p := range s.UDPPorts {
		keys = append(keys, p.EmuKey)
	}
	for _, p := range s.GRPCPorts {
		keys = append(keys, p.EmuKey)
	}
	return keys
}

func BuildFrame(dst, src MACAddr, payload []byte) []byte {
	b := make([]byte, 14+len(payload))
	copy(b[:6], dst[:])
	copy(b[6:12], src[:])
	binary.BigEndian.PutUint16(b[12:14], 0x0800)
	copy(b[14:], payload)
	return b
}
func (s *L2Switch) ProcessStream(ctx context.Context, c net.Conn) {
	defer s.Unregister(c)
	p := s.Register(c)
	for {
		b, e := ReadLengthPrefixed(c, 1518)
		if e != nil {
			return
		}
		s.Forward(b, p)
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
func (s *L2Switch) Stream(st pb.TunnelService_StreamServer) error {
	service := "default-service"
	if md, ok := metadata.FromIncomingContext(st.Context()); ok {
		if v := md.Get("x-service-name"); len(v) > 0 {
			service = v[0]
		}
	}
	mac, ip := s.PersistentService(service)
	if s.DHCP != nil && s.DHCP.Store != nil {
		s.DHCP.Store.Upsert(dhcp.MACAddr(mac), ip, true)
		_ = s.DHCP.Store.Save()
	}
	p := s.RegisterGRPC(st)
	defer s.UnregisterGRPC(st)
	s.mu.Lock()
	s.MACTable[mac] = p
	s.mu.Unlock()
	for {
		req, e := st.Recv()
		if e != nil {
			return e
		}
		b := req.GetPayload()
		if len(b) < 14 || len(b) > 1518 {
			continue
		}
		s.Forward(b, p)
	}
}
