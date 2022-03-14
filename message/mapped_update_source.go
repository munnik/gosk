package message

import "github.com/google/uuid"

type Source struct {
	Label string    `json:"label"`
	Type  string    `json:"type"`
	Uuid  uuid.UUID `json:"uuid"`
}

func NewSource() *Source {
	return &Source{}
}

func (s *Source) WithLabel(l string) *Source {
	s.Label = l
	return s
}

func (s *Source) WithType(t string) *Source {
	s.Type = t
	return s
}
func (s *Source) WithUuid(u uuid.UUID) *Source {
	s.Uuid = u
	return s
}
