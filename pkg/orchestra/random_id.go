package orchestra

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"
)

// random_id.go mints the random identifiers retained code depends on: run and
// review ids (NewSessionID), the run correlation id of a config that has none
// (ensureRunID), and short nonces for run ids, relay directories, and debate
// sentinels (randomHex).

var sessionFallbackCounter atomic.Uint64

// ensureRunID returns cfg.RunID, first assigning a timestamped random one
// when the caller left it empty.
func ensureRunID(cfg *OrchestraConfig) string {
	if cfg.RunID != "" {
		return cfg.RunID
	}
	cfg.RunID = fmt.Sprintf("run-%d-%s", time.Now().UnixMilli(), randomHex())
	return cfg.RunID
}

// NewSessionID uses 128 random bits and a collision-resistant fallback if the
// operating system random source fails.
func NewSessionID() string {
	return newSessionID(rand.Reader)
}

func newSessionID(randomSource io.Reader) string {
	randomBytes := make([]byte, 16)
	var readErr error
	if randomSource == nil {
		readErr = errors.New("nil random source")
	} else {
		_, readErr = io.ReadFull(randomSource, randomBytes)
	}
	if readErr != nil {
		counter := sessionFallbackCounter.Add(1)
		seed := fmt.Sprintf("%d:%d:%d:%p", time.Now().UnixNano(), os.Getpid(), counter, &randomBytes)
		digest := sha256.Sum256([]byte(seed))
		copy(randomBytes, digest[:16])
	}
	return "orch-" + hex.EncodeToString(randomBytes)
}

// randomHex returns an 8-character random hex string.
// SEC-005: falls back to timestamp-based value on rand.Read failure.
func randomHex() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return fmt.Sprintf("%x", b)
}
