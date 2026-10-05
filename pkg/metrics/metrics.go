package metrics

import (
	"encoding/json"
	"runtime"
	"sync/atomic"
)

type Snapshot struct {
	RxBytes     uint64 `json:"rx_bytes"`
	TxBytes     uint64 `json:"tx_bytes"`
	RxPackets   uint64 `json:"rx_packets"`
	TxPackets   uint64 `json:"tx_packets"`
	ActivePorts uint64 `json:"active_ports"`
	Goroutines  int    `json:"goroutines"`
	HeapAlloc   uint64 `json:"heap_alloc"`
	HeapSys     uint64 `json:"heap_sys"`
}

type Metrics struct {
	RxBytes     uint64
	TxBytes     uint64
	RxPackets   uint64
	TxPackets   uint64
	ActivePorts uint64
}

func (m *Metrics) AddRx(n int) {
	if n > 0 {
		atomic.AddUint64(&m.RxBytes, uint64(n))
		atomic.AddUint64(&m.RxPackets, 1)
	}
}
func (m *Metrics) AddTx(n int) {
	if n > 0 {
		atomic.AddUint64(&m.TxBytes, uint64(n))
		atomic.AddUint64(&m.TxPackets, 1)
	}
}
func (m *Metrics) PortOpened() { atomic.AddUint64(&m.ActivePorts, 1) }
func (m *Metrics) PortClosed() {
	for {
		old := atomic.LoadUint64(&m.ActivePorts)
		if old == 0 || atomic.CompareAndSwapUint64(&m.ActivePorts, old, old-1) {
			return
		}
	}
}
func (m *Metrics) Snapshot() Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return Snapshot{atomic.LoadUint64(&m.RxBytes), atomic.LoadUint64(&m.TxBytes), atomic.LoadUint64(&m.RxPackets), atomic.LoadUint64(&m.TxPackets), atomic.LoadUint64(&m.ActivePorts), runtime.NumGoroutine(), ms.HeapAlloc, ms.HeapSys}
}
func (m *Metrics) JSON() []byte { b, _ := json.Marshal(m.Snapshot()); return b }
