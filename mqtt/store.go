package mqtt

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

// msgExt is the suffix paho's FileStore appends to every key it writes.
// It is unexported there, so it is repeated here to map a file back to the
// store key it belongs to.
const msgExt = ".msg"

// boundedFileStore is paho's FileStore with a size limit.
//
// The plain FileStore grows without bound: every QoS 1 message published
// while the broker is unreachable is persisted and only removed once it is
// acknowledged, so an outage long enough (a vessel out of coverage for
// weeks) fills the filesystem and takes down everything else on the box
// with it. Losing the oldest undelivered measurements is a far better
// outcome than that - and the oldest are also the ones most likely to have
// been superseded, or to have aged past the retention window at the other
// end by the time the link returns.
//
// Only outbound messages ("o." keys, see paho's store.go) are evicted;
// inbound ones are part of an in-progress QoS 2 handshake, are few, and
// dropping one would leave the peer waiting.
type boundedFileStore struct {
	*paho.FileStore

	directory string
	maxBytes  int64

	// evictMutex serializes eviction so two concurrent Puts cannot both
	// decide to delete the same file, and so the size scan below sees a
	// settled directory.
	evictMutex sync.Mutex
}

func newBoundedFileStore(directory string, maxBytes int64) *boundedFileStore {
	return &boundedFileStore{
		FileStore: paho.NewFileStore(directory),
		directory: directory,
		maxBytes:  maxBytes,
	}
}

func (s *boundedFileStore) Put(key string, message packets.ControlPacket) {
	s.FileStore.Put(key, message)
	s.evict()
}

// evict deletes the oldest outbound messages until the store is back
// within its limit.
func (s *boundedFileStore) evict() {
	if s.maxBytes <= 0 {
		return
	}

	s.evictMutex.Lock()
	defer s.evictMutex.Unlock()

	entries, err := os.ReadDir(s.directory)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not read the MQTT store directory, skipping this eviction round",
			zap.String("Directory", s.directory),
			zap.Error(err),
		)
		return
	}

	type stored struct {
		key   string
		size  int64
		mtime int64
	}
	var total int64
	candidates := make([]stored, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), msgExt) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // it was removed from under us, which is what we wanted anyway
		}
		total += info.Size()
		key := strings.TrimSuffix(entry.Name(), msgExt)
		if !strings.HasPrefix(key, "o.") {
			continue
		}
		candidates = append(candidates, stored{key: key, size: info.Size(), mtime: info.ModTime().UnixNano()})
	}

	if total <= s.maxBytes {
		return
	}

	// Oldest first. Message ids wrap around at 65535 and are reused, so
	// the file's modification time is the only usable ordering here.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].mtime < candidates[j].mtime })

	var dropped int
	var droppedBytes int64
	for _, candidate := range candidates {
		if total <= s.maxBytes {
			break
		}
		s.FileStore.Del(candidate.key)
		total -= candidate.size
		droppedBytes += candidate.size
		dropped++
	}

	if dropped > 0 {
		logger.GetLogger().Warn(
			"The MQTT store exceeded its size limit, dropped the oldest undelivered messages",
			zap.String("Directory", s.directory),
			zap.Int("Messages", dropped),
			zap.Int64("Bytes", droppedBytes),
			zap.Int64("MaxBytes", s.maxBytes),
		)
	}
}

// storeDirectory returns the per-client subdirectory of dir. paho's
// FileStore documents that a directory may be used by exactly one client,
// so clients sharing a configured store_dir each get their own.
func storeDirectory(dir, clientID string) string {
	return filepath.Join(dir, clientID)
}
