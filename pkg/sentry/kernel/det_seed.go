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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

const detHKDFSalt = "bughunter-v1"

// DeriveSeed derives a sub-seed from a master seed using HKDF-SHA256.
// This matches the algorithm in internal/seed.DeriveFromMaster.
func DeriveSeed(master uint64, info string) uint64 {
	masterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(masterBytes, master)

	// HKDF-Extract: PRK = HMAC-SHA256(salt, IKM)
	h := hmac.New(sha256.New, []byte(detHKDFSalt))
	h.Write(masterBytes)
	prk := h.Sum(nil)

	// HKDF-Expand: OKM = HMAC-SHA256(PRK, info || 0x01)
	h = hmac.New(sha256.New, prk)
	h.Write([]byte(info))
	h.Write([]byte{0x01})
	okm := h.Sum(nil)

	return binary.BigEndian.Uint64(okm[:8])
}
