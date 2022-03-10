package message

type Source struct {
	Label string `json:"label"`
	Type  string `json:"type"`
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
