// Copyright 2024 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package time

import (
	"fmt"

	"gvisor.dev/gvisor/pkg/sync"
)

const defaultQuantum = 10_000_000 // 10ms in nanoseconds

// DeterministicClocksOpts configures DeterministicClocks.
type DeterministicClocksOpts struct {
	// Quantum is the amount of time (in nanoseconds) that each Tick advances
	// the clocks. If zero, defaults to 10ms.
	Quantum int64
}

// DeterministicClocks implements the Clocks interface with manually advanced
// virtual time. Update() always returns ok=false for both clocks, which forces
// the VDSO fallback path (syscall) so that GetTime is always called.
type DeterministicClocks struct {
	mu           sync.Mutex
	monotonicNow int64
	realtimeNow  int64
	quantum      int64
	limit        int64  // max nanoseconds before stopping; 0 = unlimited
	ticks        uint64 // total ticks
}

// NewDeterministicClocks creates a new DeterministicClocks with the given options.
func NewDeterministicClocks(opts DeterministicClocksOpts) *DeterministicClocks {
	q := opts.Quantum
	if q <= 0 {
		q = defaultQuantum
	}
	return &DeterministicClocks{
		quantum: q,
	}
}

// Update implements Clocks.Update. It always returns ok=false for both clocks
// to bypass VDSO and force the syscall path through GetTime.
func (c *DeterministicClocks) Update() (monotonicParams Parameters, monotonicOk bool, realtimeParams Parameters, realtimeOk bool) {
	return Parameters{}, false, Parameters{}, false
}

// GetTime implements Clocks.GetTime.
func (c *DeterministicClocks) GetTime(id ClockID) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch id {
	case Monotonic:
		return c.monotonicNow, nil
	case Realtime:
		return c.realtimeNow, nil
	default:
		return 0, fmt.Errorf("unsupported clock ID: %v", id)
	}
}

// Tick advances both clocks by quantum nanoseconds and increments the tick
// counter. It returns false if the time limit has been exceeded.
func (c *DeterministicClocks) Tick() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.monotonicNow += c.quantum
	c.realtimeNow += c.quantum
	c.ticks++
	if c.limit > 0 && c.monotonicNow > c.limit {
		return false
	}
	return true
}

// Reset resets both clocks to zero and clears the tick counter.
func (c *DeterministicClocks) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.monotonicNow = 0
	c.realtimeNow = 0
	c.ticks = 0
}

// SetLimit sets the maximum nanoseconds before Tick returns false.
// A limit of 0 means unlimited.
func (c *DeterministicClocks) SetLimit(ns int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limit = ns
}

// Now returns the current monotonic and realtime values in nanoseconds.
func (c *DeterministicClocks) Now() (mono int64, real int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.monotonicNow, c.realtimeNow
}

// Ticks returns the total number of ticks that have occurred.
func (c *DeterministicClocks) Ticks() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ticks
}
