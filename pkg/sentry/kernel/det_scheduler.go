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
	"encoding/binary"
	"math/rand"
	"sort"

	sentrytime "gvisor.dev/gvisor/pkg/sentry/time"
	"gvisor.dev/gvisor/pkg/sync"
	"golang.org/x/crypto/chacha20"
)

// chacha20Source implements rand.Source64 using ChaCha20.
type chacha20Source struct {
	cipher *chacha20.Cipher
	buf    [8]byte
}

func newChaCha20Source(seed uint64) *chacha20Source {
	var key [32]byte
	binary.LittleEndian.PutUint64(key[:8], seed)
	var nonce [chacha20.NonceSize]byte
	c, err := chacha20.NewUnauthenticatedCipher(key[:], nonce[:])
	if err != nil {
		panic("chacha20: " + err.Error())
	}
	return &chacha20Source{cipher: c}
}

func (s *chacha20Source) Seed(seed int64) {
	*s = *newChaCha20Source(uint64(seed))
}

func (s *chacha20Source) Int63() int64 {
	return int64(s.Uint64() & (1<<63 - 1))
}

func (s *chacha20Source) Uint64() uint64 {
	for i := range s.buf {
		s.buf[i] = 0
	}
	s.cipher.XORKeyStream(s.buf[:], s.buf[:])
	return binary.LittleEndian.Uint64(s.buf[:])
}

// DetScheduler controls deterministic task scheduling.
// It maintains per-task wake channels and selects which task to run
// using a PRNG seeded from the scheduling sub-seed.
type DetScheduler struct {
	mu sync.Mutex

	// rng is the PRNG for selecting which task to schedule next.
	rng *rand.Rand

	// clocks is the deterministic clock source.
	clocks *sentrytime.DeterministicClocks

	// tasks maps each registered task to its wake channel.
	tasks map[*Task]chan struct{}

	// runnable tracks which tasks are ready to run.
	runnable map[*Task]bool

	// running is the currently running task (holds the scheduling token).
	running *Task

	// seed is the original scheduling seed for Reset.
	seed uint64
}

// NewDetScheduler creates a DetScheduler with the given scheduling seed.
func NewDetScheduler(seed uint64, clocks *sentrytime.DeterministicClocks) *DetScheduler {
	return &DetScheduler{
		rng:      rand.New(newChaCha20Source(seed)),
		clocks:   clocks,
		tasks:    make(map[*Task]chan struct{}),
		runnable: make(map[*Task]bool),
		seed:     seed,
	}
}

// Register registers a task with the scheduler. Must be called before
// the task goroutine starts (from task_start.go).
func (ds *DetScheduler) Register(t *Task) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ch := make(chan struct{}, 1)
	ds.tasks[t] = ch
	ds.runnable[t] = true
}

// Unregister removes a task from the scheduler. Called on task exit.
func (ds *DetScheduler) Unregister(t *Task) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.tasks, t)
	delete(ds.runnable, t)
	if ds.running == t {
		ds.running = nil
	}
}

// Acquire blocks until this task is selected to run. The task goroutine
// calls this at the top of its run loop.
func (ds *DetScheduler) Acquire(t *Task) {
	ds.mu.Lock()
	ch, ok := ds.tasks[t]
	ds.mu.Unlock()
	if !ok {
		return
	}
	// Block until we receive the scheduling token.
	<-ch
}

// Release yields the scheduling token. The task calls this after completing
// a scheduling quantum. Advances virtual time by one quantum.
func (ds *DetScheduler) Release(t *Task) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.running == t {
		ds.running = nil
	}
	// Advance virtual time.
	if ds.clocks != nil {
		ds.clocks.Tick()
	}
	// Grant the next task.
	ds.grant()
}

// MarkRunnable marks a task as ready to be scheduled.
func (ds *DetScheduler) MarkRunnable(t *Task) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.runnable[t] = true
	// If no task is currently running, grant immediately.
	if ds.running == nil {
		ds.grant()
	}
}

// MarkBlocked marks a task as blocked (not ready to be scheduled).
func (ds *DetScheduler) MarkBlocked(t *Task) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.runnable[t] = false
	if ds.running == t {
		ds.running = nil
		ds.grant()
	}
}

// grant selects the next runnable task using PRNG and TID-sorted selection.
// Must be called with ds.mu held.
func (ds *DetScheduler) grant() {
	candidates := ds.sortedRunnableTasks()
	if len(candidates) == 0 {
		return
	}
	// Select using PRNG.
	idx := ds.rng.Intn(len(candidates))
	chosen := candidates[idx]
	ds.running = chosen
	// Send token to the chosen task's wake channel.
	ch := ds.tasks[chosen]
	select {
	case ch <- struct{}{}:
	default:
		// Channel already has a token (shouldn't happen, but be safe).
	}
}

// sortedRunnableTasks returns runnable tasks sorted by TID for determinism.
func (ds *DetScheduler) sortedRunnableTasks() []*Task {
	var tasks []*Task
	for t, runnable := range ds.runnable {
		if runnable {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].k.tasks.Root.tids[tasks[i]] < tasks[j].k.tasks.Root.tids[tasks[j]]
	})
	return tasks
}

// Reset resets the scheduler state with a new seed.
func (ds *DetScheduler) Reset(seed uint64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.rng = rand.New(newChaCha20Source(seed))
	ds.seed = seed
	ds.running = nil
	// Mark all tasks as runnable.
	for t := range ds.tasks {
		ds.runnable[t] = true
	}
}

// WakeChan returns the wake channel for a task, used by deterministic block().
func (ds *DetScheduler) WakeChan(t *Task) <-chan struct{} {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ch, ok := ds.tasks[t]; ok {
		return ch
	}
	return nil
}
