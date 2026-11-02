package mqtt

import (
	"hash/fnv"
	"strings"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
)

// trackedRoles are the roles whose owner sends a publish again itself when
// the far end stays silent, and so has to know which of its publishes the
// broker has not confirmed yet (Client.Holds): the outbox sender. Its rows
// sit in transfer_outbox until the far end acknowledges them, and a bucket
// nothing was heard about is read from there and sent again after
// retry_after (transfer.Sender.isInFlight).
//
// What it must not do is send a bucket again while its last delivery is
// still in this client's hands. paho keeps every QoS 1 publish the broker
// has not confirmed and re-sends all of them - up to maxResumePubInFlight
// at once - on every reconnect, for as long as the process lives, and
// offers no way to withdraw one. A sender that retried on a timer alone
// added a copy of each unacknowledged bucket per retry_after on top of the
// copies paho already held; on a link that dropped every couple of
// minutes each reconnect then spent itself re-sending copies of buckets
// the far end had long acknowledged, so fewer new deliveries completed, so
// more were retried. That was a vessel sending all day and draining
// nothing. Hence the question, and the store that can answer it.
var trackedRoles = map[string]bool{
	"transferSend": true,
}

// trackedStore is paho's message store - what it keeps between a publish
// and the broker's confirmation of it, and re-sends from on a reconnect -
// with one addition: it can say whether it holds an outbound publish with
// a given payload (Holds), and how many it holds (Unconfirmed). It keeps
// what paho's MemoryStore keeps, and differs from it only in not refusing
// to work while closed, which nothing here relies on. paho's own
// redelivery is untouched, since the alternative - a store that kept no
// publishes - leaks a message id for every publish paho never gets to
// confirm, and paho has 65535 of them.
//
// Publishes are told apart by a fingerprint of their payload rather than
// by paho's message id, which paho reuses as soon as a publish is
// confirmed; a payload here carries a uuid, so no two are alike.
type trackedStore struct {
	mu       sync.Mutex
	messages map[string]packets.ControlPacket
	// fingerprints is the fingerprint of each outbound publish held, by
	// its key, and held how many of each fingerprint there are.
	fingerprints map[string]uint64
	held         map[uint64]int
}

// outboundKeyPrefix is how paho keys the packets it sent, as opposed to the
// ones it received, in a store (paho's outboundKeyFromMID).
const outboundKeyPrefix = "o."

func newTrackedStore() *trackedStore {
	return &trackedStore{
		messages:     make(map[string]packets.ControlPacket),
		fingerprints: make(map[string]uint64),
		held:         make(map[uint64]int),
	}
}

// Fingerprint identifies a payload to Holds.
func Fingerprint(payload []byte) uint64 {
	h := fnv.New64a()
	h.Write(payload)
	return h.Sum64()
}

func (s *trackedStore) Open()  {}
func (s *trackedStore) Close() {}

func (s *trackedStore) Put(key string, message packets.ControlPacket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forget(key)
	s.messages[key] = message
	if publish, ok := message.(*packets.PublishPacket); ok && strings.HasPrefix(key, outboundKeyPrefix) {
		fingerprint := Fingerprint(publish.Payload)
		s.fingerprints[key] = fingerprint
		s.held[fingerprint]++
	}
}

func (s *trackedStore) Get(key string) packets.ControlPacket {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[key]
}

func (s *trackedStore) All() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.messages))
	for key := range s.messages {
		keys = append(keys, key)
	}
	return keys
}

func (s *trackedStore) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forget(key)
}

// forget drops key and its fingerprint, under the lock.
func (s *trackedStore) forget(key string) {
	delete(s.messages, key)
	if fingerprint, ok := s.fingerprints[key]; ok {
		delete(s.fingerprints, key)
		if s.held[fingerprint]--; s.held[fingerprint] <= 0 {
			delete(s.held, fingerprint)
		}
	}
}

func (s *trackedStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = make(map[string]packets.ControlPacket)
	s.fingerprints = make(map[string]uint64)
	s.held = make(map[uint64]int)
}

// holds reports whether an outbound publish with this fingerprint is in
// the store: handed to paho and not confirmed by the broker yet.
func (s *trackedStore) holds(fingerprint uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[fingerprint] > 0
}

// unconfirmed is how many outbound publishes the store holds.
func (s *trackedStore) unconfirmed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.fingerprints)
}

var _ paho.Store = (*trackedStore)(nil)
