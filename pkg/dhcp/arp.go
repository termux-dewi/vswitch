package dhcp

import (
	"encoding/binary"
)

type ARPHandler struct {
	GatewayIP  [4]byte
	GatewayMAC [6]byte
}

func (h *ARPHandler) Handle(frame []byte) []byte {
	if len(frame) < 42 || binary.BigEndian.Uint16(frame[12:14]) != 0x0806 {
		return nil
	}
	if binary.BigEndian.Uint16(frame[20:22]) != 1 {
		return nil
	}
	if !same(frame[38:42], h.GatewayIP[:]) {
		return nil
	}
	r := make([]byte, 42)
	copy(r[0:6], frame[6:12])
	copy(r[6:12], h.GatewayMAC[:])
	binary.BigEndian.PutUint16(r[12:14], 0x0806)
	binary.BigEndian.PutUint16(r[14:16], 1)
	binary.BigEndian.PutUint16(r[16:18], 0x0800)
	r[18] = 6
	r[19] = 4
	binary.BigEndian.PutUint16(r[20:22], 2)
	copy(r[22:28], h.GatewayMAC[:])
	copy(r[28:32], h.GatewayIP[:])
	copy(r[32:38], frame[22:28])
	copy(r[38:42], frame[28:32])
	return r
}
func same(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
