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

package kernel

import (
	"testing"

	sentrytime "gvisor.dev/gvisor/pkg/sentry/time"
)

func newTestDetScheduler(seed uint64) *DetScheduler {
	clocks := sentrytime.NewDeterministicClocks(sentrytime.DeterministicClocksOpts{})
	return NewDetScheduler(seed, clocks)
}

// newTestTaskForScheduler creates a minimal Task suitable for DetScheduler
// tests. It sets up just enough structure to be registered with the scheduler.
func newTestTaskForScheduler(k *Kernel, tid ThreadID) *Task {
	t := &Task{
		k: k,
	}
	// Register the task in the root PID namespace so that TID-sorted
	// selection works.
	k.tasks.Root.tids[t] = tid
	k.tasks.Root.tasks[tid] = t
	return t
}

// newTestKernelForScheduler creates a minimal Kernel with a root PIDNamespace
// suitable for scheduler tests.
func newTestKernelForScheduler() *Kernel {
	k := &Kernel{}
	ns := &PIDNamespace{
		tasks: make(map[ThreadID]*Task),
		tids:  make(map[*Task]ThreadID),
	}
	k.tasks = &TaskSet{
		Root: ns,
	}
	ns.owner = k.tasks
	return k
}

func TestRegisterUnregister(t *testing.T) {
	ds := newTestDetScheduler(42)
	k := newTestKernelForScheduler()
	task1 := newTestTaskForScheduler(k, 1)
	task2 := newTestTaskForScheduler(k, 2)

	ds.Register(task1)
	ds.Register(task2)

	ds.mu.Lock()
	if len(ds.tasks) != 2 {
		t.Errorf("expected 2 registered tasks, got %d", len(ds.tasks))
	}
	ds.mu.Unlock()

	ds.Unregister(task1)

	ds.mu.Lock()
	if len(ds.tasks) != 1 {
		t.Errorf("expected 1 registered task after unregister, got %d", len(ds.tasks))
	}
	if _, ok := ds.tasks[task1]; ok {
		t.Error("task1 should have been unregistered")
	}
	ds.mu.Unlock()
}

func TestAcquireReleaseCycle(t *testing.T) {
	ds := newTestDetScheduler(42)
	k := newTestKernelForScheduler()
	task1 := newTestTaskForScheduler(k, 1)

	ds.Register(task1)

	// Grant the first task so Acquire doesn't block forever.
	ds.mu.Lock()
	ds.grant()
	ds.mu.Unlock()

	done := make(chan struct{})
	go func() {
		ds.Acquire(task1)
		close(done)
	}()

	<-done // Should complete without blocking forever.

	// Release should advance time.
	ds.Release(task1)

	ds.mu.Lock()
	if ds.running != nil {
		// After release with only one task, the scheduler should have
		// re-granted to task1, making it running again.
		if ds.running != task1 {
			t.Error("expected task1 to be re-granted after release")
		}
	}
	ds.mu.Unlock()
}

func TestPRNGDeterminism(t *testing.T) {
	const seed uint64 = 12345

	// Run the same sequence twice with the same seed.
	var sequences [2][]ThreadID
	for run := 0; run < 2; run++ {
		ds := newTestDetScheduler(seed)
		k := newTestKernelForScheduler()
		tasks := make([]*Task, 3)
		for i := 0; i < 3; i++ {
			tasks[i] = newTestTaskForScheduler(k, ThreadID(i+1))
			ds.Register(tasks[i])
		}

		var seq []ThreadID
		for i := 0; i < 10; i++ {
			ds.mu.Lock()
			ds.grant()
			chosen := ds.running
			ds.mu.Unlock()

			if chosen != nil {
				seq = append(seq, k.tasks.Root.tids[chosen])
				ds.mu.Lock()
				ds.running = nil
				ds.mu.Unlock()
			}
		}
		sequences[run] = seq
	}

	if len(sequences[0]) != len(sequences[1]) {
		t.Fatalf("sequence lengths differ: %d vs %d", len(sequences[0]), len(sequences[1]))
	}
	for i := range sequences[0] {
		if sequences[0][i] != sequences[1][i] {
			t.Errorf("sequences differ at index %d: %d vs %d", i, sequences[0][i], sequences[1][i])
		}
	}
}

func TestMarkBlockedRunnable(t *testing.T) {
	ds := newTestDetScheduler(42)
	k := newTestKernelForScheduler()
	task1 := newTestTaskForScheduler(k, 1)
	task2 := newTestTaskForScheduler(k, 2)

	ds.Register(task1)
	ds.Register(task2)

	// Mark task1 as blocked.
	ds.mu.Lock()
	ds.running = task1
	ds.mu.Unlock()

	ds.MarkBlocked(task1)

	ds.mu.Lock()
	if ds.runnable[task1] {
		t.Error("task1 should be blocked")
	}
	ds.mu.Unlock()

	// Mark task1 as runnable again.
	ds.MarkRunnable(task1)

	ds.mu.Lock()
	if !ds.runnable[task1] {
		t.Error("task1 should be runnable again")
	}
	ds.mu.Unlock()
}

func TestReset(t *testing.T) {
	ds := newTestDetScheduler(42)
	k := newTestKernelForScheduler()
	task1 := newTestTaskForScheduler(k, 1)

	ds.Register(task1)
	ds.MarkBlocked(task1)

	ds.Reset(99)

	ds.mu.Lock()
	if !ds.runnable[task1] {
		t.Error("after Reset, task1 should be runnable")
	}
	if ds.seed != 99 {
		t.Errorf("expected seed 99, got %d", ds.seed)
	}
	if ds.running != nil {
		t.Error("after Reset, running should be nil")
	}
	ds.mu.Unlock()
}

func TestChaCha20Source(t *testing.T) {
	// Verify that the same seed produces the same sequence.
	s1 := newChaCha20Source(42)
	s2 := newChaCha20Source(42)

	for i := 0; i < 100; i++ {
		v1 := s1.Uint64()
		v2 := s2.Uint64()
		if v1 != v2 {
			t.Fatalf("ChaCha20 sources diverged at iteration %d: %d vs %d", i, v1, v2)
		}
	}

	// Different seeds should produce different sequences.
	s3 := newChaCha20Source(43)
	s1 = newChaCha20Source(42)
	same := true
	for i := 0; i < 10; i++ {
		if s1.Uint64() != s3.Uint64() {
			same = false
			break
		}
	}
	if same {
		t.Error("different seeds produced identical sequences")
	}
}
