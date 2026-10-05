package dhcp

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type MACAddr [6]byte

func (m MACAddr) String() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", m[0], m[1], m[2], m[3], m[4], m[5])
}

type Lease struct {
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Static bool   `json:"static"`
}
type Store struct {
	mu     sync.RWMutex
	path   string
	leases map[string]Lease
}

func NewStore(path string) *Store { return &Store{path: path, leases: map[string]Lease{}} }
func (s *Store) Load() error {
	b, e := os.ReadFile(s.path)
	if e != nil {
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	var x map[string]Lease
	if e = json.Unmarshal(b, &x); e != nil {
		return e
	}
	s.mu.Lock()
	s.leases = x
	s.mu.Unlock()
	return nil
}
func (s *Store) Save() error {
	s.mu.RLock()
	b, e := json.MarshalIndent(s.leases, "", "  ")
	s.mu.RUnlock()
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0644); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
func (s *Store) Get(mac MACAddr) (Lease, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.leases[mac.String()]
	return x, ok
}
func (s *Store) Upsert(mac MACAddr, ip [4]byte, static bool) Lease {
	x := Lease{MAC: mac.String(), IP: fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]), Static: static}
	s.mu.Lock()
	s.leases[mac.String()] = x
	s.mu.Unlock()
	return x
}
func (s *Store) List() []Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Lease, 0, len(s.leases))
	for _, v := range s.leases {
		out = append(out, v)
	}
	return out
}
