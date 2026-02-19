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
	"io"
	"sync"
	"sync/atomic"
)

// DetTracer emits binary trace events in the bughunter TraceEvent format.
// Each event is 104 bytes, little-endian encoded.
type DetTracer struct {
	mu         sync.Mutex
	w          io.Writer
	syscallSeq uint64
	closed     bool
}

// Trace event flag constants matching bughunter's trace.Flag* values.
const (
	DetTraceFlagError    uint32 = 1
	DetTraceFlagBlocking uint32 = 2
	DetTraceFlagSignal   uint32 = 4
)

// Binary trace header constants.
var detTraceHeaderMagic = [8]byte{'B', 'H', 'T', 'R', 'A', 'C', 'E', '1'}

const detTraceHeaderVersion uint32 = 1
const detTraceHeaderSize = 16 // 8 magic + 4 version + 4 filter hash
const detTraceEventSize = 104

// NewDetTracer creates a DetTracer that writes to w.
// It writes the BHTRACE1 header immediately.
func NewDetTracer(w io.Writer, filterHash uint32) (*DetTracer, error) {
	var hdr [detTraceHeaderSize]byte
	copy(hdr[:8], detTraceHeaderMagic[:])
	binary.LittleEndian.PutUint32(hdr[8:12], detTraceHeaderVersion)
	binary.LittleEndian.PutUint32(hdr[12:16], filterHash)
	if _, err := w.Write(hdr[:]); err != nil {
		return nil, err
	}
	return &DetTracer{w: w}, nil
}

// DetTraceEvent represents a single trace event to emit.
type DetTraceEvent struct {
	SchedulingClock uint64
	TaskID          int32
	SyscallNum      uint32
	Args            [6]uint64
	ReturnValue     int64
	VirtualTime     uint64
	Duration        uint64
	Flags           uint32
}

// Emit writes a trace event in the 104-byte binary format.
func (dt *DetTracer) Emit(e *DetTraceEvent) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if dt.closed {
		return nil
	}

	seq := atomic.AddUint64(&dt.syscallSeq, 1) - 1

	var buf [detTraceEventSize]byte
	off := 0
	binary.LittleEndian.PutUint64(buf[off:], e.SchedulingClock)
	off += 8
	binary.LittleEndian.PutUint64(buf[off:], seq)
	off += 8
	binary.LittleEndian.PutUint32(buf[off:], uint32(e.TaskID))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], e.SyscallNum)
	off += 4
	for i := 0; i < 6; i++ {
		binary.LittleEndian.PutUint64(buf[off:], e.Args[i])
		off += 8
	}
	binary.LittleEndian.PutUint64(buf[off:], uint64(e.ReturnValue))
	off += 8
	binary.LittleEndian.PutUint64(buf[off:], e.VirtualTime)
	off += 8
	binary.LittleEndian.PutUint64(buf[off:], e.Duration)
	off += 8
	binary.LittleEndian.PutUint32(buf[off:], e.Flags)
	// off += 4, remaining 4 bytes are padding (zeros)

	_, err := dt.w.Write(buf[:])
	return err
}

// Close closes the tracer.
func (dt *DetTracer) Close() error {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.closed = true
	if c, ok := dt.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Reset resets the syscall sequence counter (for re-execution).
func (dt *DetTracer) Reset() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	atomic.StoreUint64(&dt.syscallSeq, 0)
}
