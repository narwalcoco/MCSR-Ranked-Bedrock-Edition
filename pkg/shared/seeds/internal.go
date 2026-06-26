package seeds

import (
	"crypto/rand"
	"encoding/binary"
)

// randSeed returns a non-deterministic int64 for seeding math/rand. Falls back
// to a fixed value if crypto/rand fails (which should never happen in practice).
func randSeed() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively impossible — but if it ever does,
		// fall back to a non-zero constant instead of returning 0 (which would
		// give a fully deterministic stream in tests that forgot to inject one).
		return 0xC0FFEE
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}
