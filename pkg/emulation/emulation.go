package emulation

import (
	"math/rand"
	"sync"
	"time"
)

// Profile defines network emulation parameters for a link.
type Profile struct {
	LatencyMs        int     `json:"latency_ms"`
	JitterMs         int     `json:"jitter_ms"`
	LossPercent      float64 `json:"loss_percent"`
	BandwidthKbps    int64   `json:"bandwidth_kbps"` // 0 = unlimited
	DuplicatePercent float64 `json:"duplicate_percent"`
	CorruptPercent   float64 `json:"corrupt_percent"`
	ReorderPercent   float64 `json:"reorder_percent"`
}

func (p Profile) IsZero() bool {
	return p.LatencyMs == 0 && p.JitterMs == 0 && p.LossPercent == 0 &&
		p.BandwidthKbps == 0 && p.DuplicatePercent == 0 && p.CorruptPercent == 0 && p.ReorderPercent == 0
}

// Preset returns a named emulation profile.
func Preset(name string) Profile {
	switch name {
	case "satellite":
		return Profile{LatencyMs: 600, JitterMs: 50, LossPercent: 0.5, BandwidthKbps: 1024}
	case "3g":
		return Profile{LatencyMs: 200, JitterMs: 40, LossPercent: 2.0, BandwidthKbps: 512}
	case "lossy_wifi":
		return Profile{LatencyMs: 10, JitterMs: 20, LossPercent: 8.0, BandwidthKbps: 0}
	case "congested":
		return Profile{LatencyMs: 100, JitterMs: 80, LossPercent: 5.0, BandwidthKbps: 256, ReorderPercent: 3.0}
	case "intermittent":
		return Profile{LatencyMs: 50, JitterMs: 30, LossPercent: 15.0, BandwidthKbps: 0}
	default:
		return Profile{}
	}
}

// PresetNames returns all available preset names.
func PresetNames() []string {
	return []string{"satellite", "3g", "lossy_wifi", "congested", "intermittent"}
}

// Manager manages per-port emulation state.
type Manager struct {
	mu      sync.RWMutex
	shapers map[string]*Shaper
	rng     *rand.Rand
}

func NewManager() *Manager {
	return &Manager{
		shapers: make(map[string]*Shaper),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SetProfile applies an emulation profile to a port identified by key.
func (m *Manager) SetProfile(key string, p Profile, deliver func([]byte)) *Shaper {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.shapers[key]; ok {
		existing.Update(p)
		return existing
	}
	s := NewShaper(p, deliver, m.rng)
	m.shapers[key] = s
	s.Start()
	return s
}

// RemoveProfile removes emulation from a port.
func (m *Manager) RemoveProfile(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.shapers[key]; ok {
		s.Stop()
		delete(m.shapers, key)
	}
}

// GetProfile returns the current profile for a port.
func (m *Manager) GetProfile(key string) (Profile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.shapers[key]; ok {
		return s.Current(), true
	}
	return Profile{}, false
}

// ListProfiles returns all active emulation profiles.
func (m *Manager) ListProfiles() map[string]Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]Profile, len(m.shapers))
	for k, s := range m.shapers {
		out[k] = s.Current()
	}
	return out
}

// Shape processes a frame through emulation for the given port key.
// Returns true if the frame was consumed by emulation (caller should not send directly).
func (m *Manager) Shape(key string, frame []byte) bool {
	m.mu.RLock()
	s, ok := m.shapers[key]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	s.Enqueue(frame)
	return true
}

// Stats returns emulation statistics for all ports.
func (m *Manager) Stats() map[string]ShaperStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]ShaperStats, len(m.shapers))
	for k, s := range m.shapers {
		out[k] = s.Stats()
	}
	return out
}

// StopAll stops all shapers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.shapers {
		s.Stop()
		delete(m.shapers, k)
	}
}
