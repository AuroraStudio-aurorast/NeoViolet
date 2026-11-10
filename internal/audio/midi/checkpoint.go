package midi

import (
	"slices"
	"sort"
	"sync"
)

// Clone copies a complete continuation while sharing the immutable timeline and
// SoundFont. The caller owns r and must exclude concurrent Render calls.
func (r *Renderer) Clone() *Renderer {
	c := *r
	c.ports = make(map[uint8]*port, len(r.ports))
	c.order = slices.Clone(r.order)
	c.diagnostics = slices.Clone(r.diagnostics)
	for id, p := range r.ports {
		x := *p
		x.synth = p.synth.Clone()
		c.ports[id] = &x
	}
	return &c
}

// StateBytes estimates the retained DSP snapshot size, excluding immutable
// assets and transient working copies. Padding includes scheduler/map metadata.
func (r *Renderer) StateBytes() int64 {
	n := int64(8192 + len(r.diagnostics)*512)
	for _, p := range r.ports {
		n += p.synth.StateBytes() + 1024
	}
	return n
}

// CheckpointStats describes the bounded retained cache, not whole-process RSS.
type CheckpointStats struct {
	Count                                        int
	Bytes, Budget, IntervalFrames, IndexedFrames int64
}

// Checkpoints retains immutable snapshots, sparsifying the entire timeline when
// the budget fills. Restore returns a private copy and never advances the cache.
type Checkpoints struct {
	mu                                     sync.Mutex
	entries                                []*Renderer
	bytes, budget, interval, indexed, cost int64
}

// NewCheckpoints starts at a five-second interval; zero is always reconstructed
// by the renderer factory, so it does not consume the retained cache budget.
func NewCheckpoints(rate int32, budget int64) *Checkpoints {
	return &Checkpoints{budget: max(0, budget), interval: max(1, int64(rate)) * 5}
}

// Capture clones only when the position reaches a new interval bucket. Called
// by the background owner; audio callbacks must never call this allocating path.
func (c *Checkpoints) Capture(r *Renderer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pos := r.PositionFrames()
	c.indexed = max(c.indexed, pos)
	if pos < c.interval {
		return
	}
	if c.cost == 0 {
		c.cost = r.StateBytes()
	}
	cost := c.cost
	if cost > c.budget {
		return
	}
	for {
		bucket := pos / c.interval
		at := sort.Search(len(c.entries), func(i int) bool { return c.entries[i].position/c.interval >= bucket })
		if at < len(c.entries) && c.entries[at].position/c.interval == bucket {
			return
		}
		if c.bytes+cost <= c.budget {
			snapshot := r.Clone()
			c.entries = slices.Insert(c.entries, at, snapshot)
			c.bytes += cost
			return
		}
		// Keep every other time bucket, rather than losing all early checkpoints.
		c.interval *= 2
		kept := c.entries[:0]
		c.bytes = 0
		for _, entry := range c.entries {
			if entry.position/c.interval*2 == entry.position/(c.interval/2) {
				kept = append(kept, entry)
				c.bytes += cost
			}
		}
		clear(c.entries[len(kept):])
		c.entries = kept
		if pos < c.interval {
			return
		}
	}
}

// Restore finds the nearest retained checkpoint at or before target.
func (c *Checkpoints) Restore(target int64) *Renderer {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := sort.Search(len(c.entries), func(i int) bool { return c.entries[i].position > target })
	if at == 0 {
		return nil
	}
	return c.entries[at-1].Clone()
}

// Stats returns a synchronized cache summary.
func (c *Checkpoints) Stats() CheckpointStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CheckpointStats{len(c.entries), c.bytes, c.budget, c.interval, c.indexed}
}
