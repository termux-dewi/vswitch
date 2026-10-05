package dhcp

import (
	"encoding/binary"
	"fmt"
)

type Server struct {
	GatewayIP  [4]byte
	GatewayMAC [6]byte
	Store      *Store
}

func (s *Server) Handle(frame []byte) []byte {
	if len(frame) < 282 || binary.BigEndian.Uint16(frame[12:14]) != 0x0800 || frame[23] != 17 {
		return nil
	}
	src := MACAddr{frame[6], frame[7], frame[8], frame[9], frame[10], frame[11]}
	lease, ok := s.Store.Get(src)
	if !ok {
		ip := [4]byte{10, 0, 0, byte(10 + len(s.Store.List()))}
		lease = s.Store.Upsert(src, ip, false)
		_ = s.Store.Save()
	}
	var ip [4]byte
	var p [4]byte
	_, _ = fmtIP(lease.IP, &ip)
	p = s.GatewayIP
	dhcpLen := 240 + 28
	r := make([]byte, 14+20+8+dhcpLen)
	copy(r[:6], []byte{255, 255, 255, 255, 255, 255})
	copy(r[6:12], s.GatewayMAC[:])
	binary.BigEndian.PutUint16(r[12:14], 0x0800)
	i := 14
	r[i] = 0x45
	binary.BigEndian.PutUint16(r[i+2:i+4], uint16(20+8+dhcpLen))
	r[i+8] = 64
	r[i+9] = 17
	copy(r[i+12:i+16], s.GatewayIP[:])
	copy(r[i+16:i+20], []byte{255, 255, 255, 255})
	binary.BigEndian.PutUint16(r[i+10:i+12], checksum(r[i:i+20]))
	u := 34
	binary.BigEndian.PutUint16(r[u:u+2], 67)
	binary.BigEndian.PutUint16(r[u+2:u+4], 68)
	binary.BigEndian.PutUint16(r[u+4:u+6], uint16(8+dhcpLen))
	d := 42
	r[d] = 2
	r[d+1] = 1
	r[d+2] = 6
	copy(r[d+4:d+8], frame[d+4:d+8])
	copy(r[d+16:d+20], ip[:])
	copy(r[d+20:d+24], p[:])
	copy(r[d+28:d+34], src[:])
	copy(r[d+236:d+240], []byte{0x63, 0x82, 0x53, 0x63})
	o := r[d+240:]
	o[0], o[1], o[2] = 53, 1, 5
	o[3], o[4], o[5], o[6], o[7], o[8] = 1, 4, 255, 255, 255, 0
	o[9], o[10], o[11], o[12], o[13], o[14] = 3, 4, p[0], p[1], p[2], p[3]
	o[15], o[16], o[17], o[18], o[19], o[20] = 6, 4, p[0], p[1], p[2], p[3]
	o[21] = 255
	return r
}
func fmtIP(v string, o *[4]byte) (bool, error) {
	var a, b, c, d byte
	_, e := fmt.Sscanf(v, "%d.%d.%d.%d", &a, &b, &c, &d)
	if e != nil {
		return false, e
	}
	*o = [4]byte{a, b, c, d}
	return true, nil
}
func checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}
