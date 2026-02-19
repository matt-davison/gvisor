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
	"testing"
)

func TestDeterministicClocksInitialState(t *testing.T) {
	c := NewDeterministicClocks(DeterministicClocksOpts{})
	mono, real := c.Now()
	if mono != 0 || real != 0 {
		t.Errorf("initial Now() = (%d, %d), want (0, 0)", mono, real)
	}
	if ticks := c.Ticks(); ticks != 0 {
		t.Errorf("initial Ticks() = %d, want 0", ticks)
	}
}

func TestDeterministicClocksTick(t *testing.T) {
	c := NewDeterministicClocks(DeterministicClocksOpts{})
	ok := c.Tick()
	if !ok {
		t.Error("Tick() returned false, want true")
	}
	mono, real := c.Now()
	if mono != defaultQuantum {
		t.Errorf("mono after 1 tick = %d, want %d", mono, defaultQuantum)
	}
	if real != defaultQuantum {
		t.Errorf("real after 1 tick = %d, want %d", real, defaultQuantum)
	}
	if ticks := c.Ticks(); ticks != 1 {
		t.Errorf("Ticks() after 1 tick = %d, want 1", ticks)
	}
}

func TestDeterministicClocksCustomQuantum(t *testing.T) {
	quantum := int64(5_000_000) // 5ms
	c := NewDeterministicClocks(DeterministicClocksOpts{Quantum: quantum})
	c.Tick()
	c.Tick()
	mono, real := c.Now()
	want := quantum * 2
	if mono != want {
		t.Errorf("mono after 2 ticks = %d, want %d", mono, want)
	}
	if real != want {
		t.Errorf("real after 2 ticks = %d, want %d", real, want)
	}
}

func TestDeterministicClocksReset(t *testing.T) {
	c := NewDeterministicClocks(DeterministicClocksOpts{})
	c.Tick()
	c.Tick()
	c.Reset()
	mono, real := c.Now()
	if mono != 0 || real != 0 {
		t.Errorf("Now() after Reset = (%d, %d), want (0, 0)", mono, real)
	}
	if ticks := c.Ticks(); ticks != 0 {
		t.Errorf("Ticks() after Reset = %d, want 0", ticks)
	}
}

func TestDeterministicClocksSetLimit(t *testing.T) {
	quantum := int64(10_000_000)
	c := NewDeterministicClocks(DeterministicClocksOpts{Quantum: quantum})
	c.SetLimit(25_000_000) // limit at 25ms

	// First tick: 10ms - within limit
	if ok := c.Tick(); !ok {
		t.Error("Tick() 1 returned false, want true")
	}
	// Second tick: 20ms - within limit
	if ok := c.Tick(); !ok {
		t.Error("Tick() 2 returned false, want true")
	}
	// Third tick: 30ms - exceeds limit
	if ok := c.Tick(); ok {
		t.Error("Tick() 3 returned true, want false (exceeded limit)")
	}
}

func TestDeterministicClocksUpdate(t *testing.T) {
	c := NewDeterministicClocks(DeterministicClocksOpts{})
	_, monoOk, _, realOk := c.Update()
	if monoOk {
		t.Error("Update() monotonicOk = true, want false")
	}
	if realOk {
		t.Error("Update() realtimeOk = true, want false")
	}
}

func TestDeterministicClocksGetTime(t *testing.T) {
	c := NewDeterministicClocks(DeterministicClocksOpts{})
	c.Tick()

	mono, err := c.GetTime(Monotonic)
	if err != nil {
		t.Fatalf("GetTime(Monotonic) error: %v", err)
	}
	if mono != defaultQuantum {
		t.Errorf("GetTime(Monotonic) = %d, want %d", mono, defaultQuantum)
	}

	real, err := c.GetTime(Realtime)
	if err != nil {
		t.Fatalf("GetTime(Realtime) error: %v", err)
	}
	if real != defaultQuantum {
		t.Errorf("GetTime(Realtime) = %d, want %d", real, defaultQuantum)
	}

	_, err = c.GetTime(ClockID(99))
	if err == nil {
		t.Error("GetTime(99) expected error, got nil")
	}
}
