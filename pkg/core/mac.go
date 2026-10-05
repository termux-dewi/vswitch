package core

import "fmt"

type MACAddr [6]byte

func ToMAC(b []byte) MACAddr {
	if len(b) < 6 {
		return MACAddr{}
	}
	return MACAddr{b[0], b[1], b[2], b[3], b[4], b[5]}
}
func (m MACAddr) String() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", m[0], m[1], m[2], m[3], m[4], m[5])
}
