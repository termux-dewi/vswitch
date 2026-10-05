package emulation

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ShaperStats tracks emulation counters.
type ShaperStats struct {
	Enqueued   uint64 `json:"enqueued"`
	Delivered  uint64 `json:"delivered"`
	Dropped    uint64 `json:"dropped"`
	Duplicated uint64 `json:"duplicated"`
	Corrupted  uint64 `json:"corrupted"`
	Reordered  uint64 `json:"reordered"`
	Throttled  uint64 `json:"throttled"`
	QueueDepth int    `json:"queue_depth"`
}

type delayedFrame struct {
	frame     []byte
	deliverAt time.Time
	seq       uint64
}

// Shaper implements per-port network emulation.
type Shaper struct {
	mu       sync.RWMutex
	profile  Profile
	deliver  func([]byte)
	rng      *rand.Rand
	incoming chan []byte
	stop     chan struct{}
	stopped  sync.Once

	// token bucket for bandwidth limiting
	tbTokens   float64
	tbLastTime time.Time

	// stats
	enqueued   uint64
	delivered  uint64
	dropped    uint64
	duplicated uint64
	corrupted  uint64
	reordered  uint64
	throttled  uint64

	seq uint64
}

func NewShaper(p Profile, deliver func([]byte), rng *rand.Rand) *Shaper {
	return &Shaper{
		profile:    p,
		deliver:    deliver,
		rng:        rng,
		incoming:   make(chan []byte, 4096),
		stop:       make(chan struct{}),
		tbTokens:   float64(p.BandwidthKbps) * 128, // Kbps -> bytes/s (Kbps * 1000/8)
		tbLastTime: time.Now(),
	}
}

func (s *Shaper) Start() {
	go s.loop()
}

func (s *Shaper) Stop() {
	s.stopped.Do(func() { close(s.stop) })
}

func (s *Shaper) Update(p Profile) {
	s.mu.Lock()
	s.profile = p
	s.mu.Unlock()
}

func (s *Shaper) Current() Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profile
}

func (s *Shaper) Enqueue(frame []byte) {
	cp := make([]byte, len(frame))
	copy(cp, frame)
	select {
	case s.incoming <- cp:
	default:
		atomic.AddUint64(&s.dropped, 1)
	}
}

func (s *Shaper) Stats() ShaperStats {
	return ShaperStats{
		Enqueued:   atomic.LoadUint64(&s.enqueued),
		Delivered:  atomic.LoadUint64(&s.delivered),
		Dropped:    atomic.LoadUint64(&s.dropped),
		Duplicated: atomic.LoadUint64(&s.duplicated),
		Corrupted:  atomic.LoadUint64(&s.corrupted),
		Reordered:  atomic.LoadUint64(&s.reordered),
		Throttled:  atomic.LoadUint64(&s.throttled),
		QueueDepth: len(s.incoming),
	}
}

func (s *Shaper) loop() {
	var pending []delayedFrame
	var timer *time.Timer
	var timerC <-chan time.Time

	fire := func() {
		if timer != nil {
			timer.Stop()
		}
		if len(pending) > 0 {
			next := pending[0].deliverAt.Sub(time.Now())
			if next <= 0 {
				next = 0
			}
			timer = time.NewTimer(next)
			timerC = timer.C
		} else {
			timerC = nil
		}
	}

	for {
		fire()
		select {
		case <-s.stop:
			if timer != nil {
				timer.Stop()
			}
			return

		case frame := <-s.incoming:
			atomic.AddUint64(&s.enqueued, 1)
			s.processFrame(frame, &pending)

		case <-timerC:
			s.deliverReady(&pending)
		}
	}
}

func (s *Shaper) processFrame(frame []byte, pending *[]delayedFrame) {
	p := s.Current()

	// packet loss
	if p.LossPercent > 0 && s.rng.Float64()*100 < p.LossPercent {
		atomic.AddUint64(&s.dropped, 1)
		return
	}

	// corruption
	if p.CorruptPercent > 0 && s.rng.Float64()*100 < p.CorruptPercent {
		frame = s.corruptFrame(frame)
		atomic.AddUint64(&s.corrupted, 1)
	}

	// duplication
	if p.DuplicatePercent > 0 && s.rng.Float64()*100 < p.DuplicatePercent {
		dup := make([]byte, len(frame))
		copy(dup, frame)
		s.scheduleDelivery(dup, p, pending)
		atomic.AddUint64(&s.duplicated, 1)
	}

	// reorder: deliver immediately with small probability to cause reordering
	if p.ReorderPercent > 0 && s.rng.Float64()*100 < p.ReorderPercent {
		atomic.AddUint64(&s.reordered, 1)
		s.deliverDirect(frame, p)
		return
	}

	s.scheduleDelivery(frame, p, pending)
}

func (s *Shaper) scheduleDelivery(frame []byte, p Profile, pending *[]delayedFrame) {
	delay := time.Duration(p.LatencyMs) * time.Millisecond
	if p.JitterMs > 0 {
		jitter := time.Duration(s.rng.Intn(p.JitterMs)) * time.Millisecond
		delay += jitter
	}

	if delay == 0 {
		s.deliverDirect(frame, p)
		return
	}

	s.seq++
	*pending = append(*pending, delayedFrame{
		frame:     frame,
		deliverAt: time.Now().Add(delay),
		seq:       s.seq,
	})
}

func (s *Shaper) deliverDirect(frame []byte, p Profile) {
	// bandwidth throttling
	if p.BandwidthKbps > 0 {
		if !s.consumeTokens(len(frame)) {
			atomic.AddUint64(&s.throttled, 1)
			// delay instead of drop
			time.Sleep(time.Duration(float64(len(frame))/(float64(p.BandwidthKbps)*128)) * time.Second)
		}
	}
	s.deliver(frame)
	atomic.AddUint64(&s.delivered, 1)
}

func (s *Shaper) deliverReady(pending *[]delayedFrame) {
	now := time.Now()
	p := s.Current()
	var remaining []delayedFrame
	for _, df := range *pending {
		if df.deliverAt.After(now) {
			remaining = append(remaining, df)
			continue
		}
		if p.BandwidthKbps > 0 {
			if !s.consumeTokens(len(df.frame)) {
				atomic.AddUint64(&s.throttled, 1)
			}
		}
		s.deliver(df.frame)
		atomic.AddUint64(&s.delivered, 1)
	}
	*pending = remaining
}

func (s *Shaper) consumeTokens(n int) bool {
	if s.profile.BandwidthKbps <= 0 {
		return true
	}
	now := time.Now()
	elapsed := now.Sub(s.tbLastTime).Seconds()
	s.tbLastTime = now
	rate := float64(s.profile.BandwidthKbps) * 128 // bytes per second
	s.tbTokens += elapsed * rate
	maxTokens := rate * 0.1 // allow 100ms burst
	if s.tbTokens > maxTokens {
		s.tbTokens = maxTokens
	}
	if s.tbTokens >= float64(n) {
		s.tbTokens -= float64(n)
		return true
	}
	return false
}

func (s *Shaper) corruptFrame(frame []byte) []byte {
	cp := make([]byte, len(frame))
	copy(cp, frame)
	// flip 1-3 random bits
	nFlips := 1 + s.rng.Intn(3)
	for i := 0; i < nFlips && len(cp) > 14; i++ {
		idx := 14 + s.rng.Intn(len(cp)-14) // corrupt payload only, keep headers
		cp[idx] ^= 1 << uint(s.rng.Intn(8))
	}
	return cp
}
