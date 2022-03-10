package message

import (
	"encoding/json"
	"sync"
)

type Buffer struct {
	Deltas []Mapped `json:"deltas"`
	lock   *sync.RWMutex
}

func NewBuffer() *Buffer {
	return &Buffer{
		Deltas: make([]Mapped, 0),
		lock:   &sync.RWMutex{},
	}
}

func (b *Buffer) Lock() *Buffer {
	b.lock.Lock()
	return b
}

func (b *Buffer) Unlock() *Buffer {
	b.lock.Unlock()
	return b
}

func (b *Buffer) Empty() *Buffer {
	b.Deltas = make([]Mapped, 0)
	return b
}

func (b *Buffer) Append(m Mapped) *Buffer {
	b.Deltas = append(b.Deltas, m)
	return b
}

func (b Buffer) MarshalJSON() ([]byte, error) {
	var result map[string]interface{} = make(map[string]interface{})
	result["deltas"] = b.Deltas
	return json.Marshal(result)
}
