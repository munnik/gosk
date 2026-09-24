package mqtt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

func publishPacket(messageID uint16, payloadSize int) *packets.PublishPacket {
	p := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	p.Qos = 1
	p.TopicName = "vessels/test"
	p.MessageID = messageID
	p.Payload = make([]byte, payloadSize)
	return p
}

// storedSize is the total size of the store's .msg files, the same figure
// evict works against.
func storedSize(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("could not read the store directory: %v", err)
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("could not stat %s: %v", entry.Name(), err)
		}
		total += info.Size()
	}
	return total
}

func TestBoundedFileStoreEvictsOldestOutboundFirst(t *testing.T) {
	dir := t.TempDir()
	store := newBoundedFileStore(dir, 1<<30) // effectively unbounded while filling
	store.Open()
	defer store.Close()

	const messages = 10
	for i := 1; i <= messages; i++ {
		store.Put(fmt.Sprintf("o.%d", i), publishPacket(uint16(i), 1024))
	}
	// One inbound message, which eviction must leave alone: it belongs to
	// an in-progress handshake the peer is waiting on.
	store.Put("i.1", publishPacket(1, 1024))

	// Eviction orders by modification time, and files written this close
	// together can share one. Space them out explicitly so the test is
	// about the ordering rather than about filesystem timestamp
	// resolution. o.1 is the oldest.
	base := time.Now().Add(-time.Hour)
	for i := 1; i <= messages; i++ {
		name := filepath.Join(dir, fmt.Sprintf("o.%d%s", i, msgExt))
		stamp := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatalf("could not set the modification time of %s: %v", name, err)
		}
	}
	inbound := filepath.Join(dir, "i.1"+msgExt)
	if err := os.Chtimes(inbound, base, base); err != nil { // oldest of all
		t.Fatalf("could not set the modification time of %s: %v", inbound, err)
	}

	full := storedSize(t, dir)
	store.maxBytes = full / 2
	store.evict()

	if got := storedSize(t, dir); got > store.maxBytes {
		t.Errorf("store is %d bytes after eviction, over the %d byte limit", got, store.maxBytes)
	}

	if store.Get("i.1") == nil {
		t.Error("the inbound message was evicted, only outbound ones may be")
	}

	// Whatever survived has to be a suffix of the outbound sequence: the
	// oldest go first, so a surviving message implies every newer one
	// survived too.
	var oldestSurvivor int
	for i := 1; i <= messages; i++ {
		if store.Get(fmt.Sprintf("o.%d", i)) != nil {
			oldestSurvivor = i
			break
		}
	}
	if oldestSurvivor == 0 {
		t.Fatal("every outbound message was evicted, expected the newest ones to survive")
	}
	if oldestSurvivor == 1 {
		t.Fatal("no outbound message was evicted, expected the store to be brought under its limit")
	}
	for i := oldestSurvivor; i <= messages; i++ {
		if store.Get(fmt.Sprintf("o.%d", i)) == nil {
			t.Errorf("o.%d was evicted while the older o.%d was kept", i, oldestSurvivor)
		}
	}
}

func TestBoundedFileStoreKeepsEverythingUnderTheLimit(t *testing.T) {
	dir := t.TempDir()
	store := newBoundedFileStore(dir, 1<<30)
	store.Open()
	defer store.Close()

	for i := 1; i <= 5; i++ {
		store.Put(fmt.Sprintf("o.%d", i), publishPacket(uint16(i), 1024))
	}

	for i := 1; i <= 5; i++ {
		if store.Get(fmt.Sprintf("o.%d", i)) == nil {
			t.Errorf("o.%d was evicted from a store that is nowhere near its limit", i)
		}
	}
}

func TestBoundedFileStoreUnlimitedWhenMaxBytesIsNotSet(t *testing.T) {
	dir := t.TempDir()
	store := newBoundedFileStore(dir, 0)
	store.Open()
	defer store.Close()

	for i := 1; i <= 5; i++ {
		store.Put(fmt.Sprintf("o.%d", i), publishPacket(uint16(i), 1024))
	}

	for i := 1; i <= 5; i++ {
		if store.Get(fmt.Sprintf("o.%d", i)) == nil {
			t.Errorf("o.%d was evicted although no limit was configured", i)
		}
	}
}
