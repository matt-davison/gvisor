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

package rand

import (
	"bytes"
	"sync"
	"testing"
)

func TestDeterministicRNGSameSeed(t *testing.T) {
	rng1 := NewDeterministicRNG(42)
	rng2 := NewDeterministicRNG(42)

	buf1 := make([]byte, 256)
	buf2 := make([]byte, 256)

	if _, err := rng1.Read(buf1); err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if _, err := rng2.Read(buf2); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(buf1, buf2) {
		t.Error("same seed produced different output")
	}
}

func TestDeterministicRNGDifferentSeeds(t *testing.T) {
	rng1 := NewDeterministicRNG(42)
	rng2 := NewDeterministicRNG(99)

	buf1 := make([]byte, 256)
	buf2 := make([]byte, 256)

	if _, err := rng1.Read(buf1); err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if _, err := rng2.Read(buf2); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if bytes.Equal(buf1, buf2) {
		t.Error("different seeds produced same output")
	}
}

func TestDeterministicRNGReset(t *testing.T) {
	rng := NewDeterministicRNG(42)

	buf1 := make([]byte, 256)
	if _, err := rng.Read(buf1); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	rng.Reset(42)

	buf2 := make([]byte, 256)
	if _, err := rng.Read(buf2); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(buf1, buf2) {
		t.Error("Reset did not restore the same sequence")
	}
}

func TestDeterministicRNGConcurrent(t *testing.T) {
	rng := NewDeterministicRNG(42)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			if _, err := rng.Read(buf); err != nil {
				t.Errorf("concurrent Read failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestSetDeterministicMathRand(t *testing.T) {
	SetDeterministicMathRand(42)
	defer func() {
		detMathRandMu.Lock()
		detMathRand = nil
		detMathRandMu.Unlock()
	}()

	// Same seed must produce same sequence.
	vals1 := make([]int64, 10)
	for i := range vals1 {
		vals1[i] = MathRandInt63n(1000000)
	}

	SetDeterministicMathRand(42)
	vals2 := make([]int64, 10)
	for i := range vals2 {
		vals2[i] = MathRandInt63n(1000000)
	}

	for i := range vals1 {
		if vals1[i] != vals2[i] {
			t.Errorf("MathRandInt63n: index %d: got %d, want %d", i, vals2[i], vals1[i])
		}
	}

	// Different seed must produce different sequence.
	SetDeterministicMathRand(99)
	vals3 := make([]int64, 10)
	for i := range vals3 {
		vals3[i] = MathRandInt63n(1000000)
	}
	same := true
	for i := range vals1 {
		if vals1[i] != vals3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different seeds produced same MathRandInt63n sequence")
	}
}

func TestMathRandFallback(t *testing.T) {
	// When detMathRand is nil, functions should not panic.
	detMathRandMu.Lock()
	detMathRand = nil
	detMathRandMu.Unlock()

	_ = MathRandInt63n(100)
	_ = MathRandUint32()
	_ = MathRandIntn(100)
	_ = MathRandFloat64()
}

func TestDeterministicRNGFillsBuffer(t *testing.T) {
	rng := NewDeterministicRNG(42)

	for _, size := range []int{1, 16, 64, 256, 1024, 4096} {
		buf := make([]byte, size)
		n, err := rng.Read(buf)
		if err != nil {
			t.Fatalf("Read(%d) failed: %v", size, err)
		}
		if n != size {
			t.Errorf("Read(%d) returned %d bytes", size, n)
		}
		// Verify not all zeros (extremely unlikely with ChaCha20).
		allZero := true
		for _, b := range buf {
			if b != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Errorf("Read(%d) returned all zeros", size)
		}
	}
}
