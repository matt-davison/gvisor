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
	"encoding/binary"
	mrand "math/rand"
	"sync"

	"golang.org/x/crypto/chacha20"
)

// DeterministicRNG implements io.Reader using ChaCha20 for deterministic output.
type DeterministicRNG struct {
	mu     sync.Mutex
	cipher *chacha20.Cipher
	seed   uint64
}

// NewDeterministicRNG creates a DeterministicRNG seeded from a uint64.
func NewDeterministicRNG(seed uint64) *DeterministicRNG {
	var key [32]byte
	binary.LittleEndian.PutUint64(key[:8], seed)
	var nonce [chacha20.NonceSize]byte
	c, err := chacha20.NewUnauthenticatedCipher(key[:], nonce[:])
	if err != nil {
		panic("chacha20: " + err.Error())
	}
	return &DeterministicRNG{
		cipher: c,
		seed:   seed,
	}
}

// Read implements io.Reader. It produces deterministic pseudorandom bytes.
func (d *DeterministicRNG) Read(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Zero the buffer, then XOR with keystream to get random bytes.
	for i := range p {
		p[i] = 0
	}
	d.cipher.XORKeyStream(p, p)
	return len(p), nil
}

// Reset re-initializes the RNG with a new seed.
func (d *DeterministicRNG) Reset(seed uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var key [32]byte
	binary.LittleEndian.PutUint64(key[:8], seed)
	var nonce [chacha20.NonceSize]byte
	c, err := chacha20.NewUnauthenticatedCipher(key[:], nonce[:])
	if err != nil {
		panic("chacha20: " + err.Error())
	}
	d.cipher = c
	d.seed = seed
}

// SetDeterministicRNG replaces the global Reader with a DeterministicRNG.
func SetDeterministicRNG(seed uint64) {
	Reader = NewDeterministicRNG(seed)
}

// detMathRand is a deterministic replacement for the math/rand global source.
// When non-nil, MathRandXxx functions use this instead of the auto-seeded global.
var detMathRand *mrand.Rand
var detMathRandMu sync.Mutex

// chacha20Source adapts DeterministicRNG as a math/rand.Source64.
type chacha20Source struct {
	rng *DeterministicRNG
}

func (s *chacha20Source) Int63() int64 {
	return int64(s.Uint64() & 0x7fffffffffffffff)
}

func (s *chacha20Source) Uint64() uint64 {
	var buf [8]byte
	s.rng.Read(buf[:])
	return binary.LittleEndian.Uint64(buf[:])
}

func (s *chacha20Source) Seed(seed int64) {
	s.rng.Reset(uint64(seed))
}

// SetDeterministicMathRand replaces the math/rand global functions with a
// deterministic source backed by ChaCha20. This is separate from
// SetDeterministicRNG (which replaces crypto/rand-style Reader).
func SetDeterministicMathRand(seed uint64) {
	detMathRandMu.Lock()
	defer detMathRandMu.Unlock()
	src := &chacha20Source{rng: NewDeterministicRNG(seed)}
	detMathRand = mrand.New(src)
}

// MathRandInt63n returns a deterministic int64 in [0, n) when in deterministic
// mode, otherwise falls back to math/rand.Int63n.
func MathRandInt63n(n int64) int64 {
	detMathRandMu.Lock()
	defer detMathRandMu.Unlock()
	if detMathRand != nil {
		return detMathRand.Int63n(n)
	}
	return mrand.Int63n(n)
}

// MathRandUint32 returns a deterministic uint32 when in deterministic mode,
// otherwise falls back to math/rand.Uint32.
func MathRandUint32() uint32 {
	detMathRandMu.Lock()
	defer detMathRandMu.Unlock()
	if detMathRand != nil {
		return detMathRand.Uint32()
	}
	return mrand.Uint32()
}

// MathRandIntn returns a deterministic int in [0, n) when in deterministic
// mode, otherwise falls back to math/rand.Intn.
func MathRandIntn(n int) int {
	detMathRandMu.Lock()
	defer detMathRandMu.Unlock()
	if detMathRand != nil {
		return detMathRand.Intn(n)
	}
	return mrand.Intn(n)
}

// MathRandFloat64 returns a deterministic float64 in [0.0, 1.0) when in
// deterministic mode, otherwise falls back to math/rand.Float64.
func MathRandFloat64() float64 {
	detMathRandMu.Lock()
	defer detMathRandMu.Unlock()
	if detMathRand != nil {
		return detMathRand.Float64()
	}
	return mrand.Float64()
}
