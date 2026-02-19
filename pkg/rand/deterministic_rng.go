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
