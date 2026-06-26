package matchmaker

import (
	"crypto/rand"
	"encoding/binary"
)

// seedRandomIndex returns a uniformly random integer in [0, n).
// Falls back to n-1 if crypto/rand fails (effectively unreachable) so
// the loop doesn't get stuck.
func seedRandomIndex(n int) int {
	if n <= 0 {
		return 0
	}
	if n == 1 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return n - 1
	}
	return int(binary.LittleEndian.Uint64(b[:]) % uint64(n))
}
