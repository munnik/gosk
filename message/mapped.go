package message

type Mapped struct {
	Context string   `json:"context"` // indicates what the data is about
	Origin  string   `json:"origin"`  // indicates the creator of the data
	Updates []Update `json:"updates"`
}

func NewMapped() *Mapped {
	return &Mapped{
		Updates: make([]Update, 0),
	}
}

func (m *Mapped) WithContext(c string) *Mapped {
	m.Context = c
	return m
}

func (m *Mapped) WithOrigin(o string) *Mapped {
	m.Origin = o
	return m
}

func (m *Mapped) AddUpdate(u *Update) *Mapped {
	m.Updates = append(m.Updates, *u)
	return m
}

func (m Mapped) Equals(other Mapped) bool {
	if m.Context != other.Context {
		return false
	}

	if m.Origin != other.Origin {
		return false
	}

	if len(m.Updates) != len(other.Updates) {
		return false
	}

	for _, u := range m.Updates {
		found := false
		for _, ou := range other.Updates {
			if u.Equals(ou) {
				found = true
				continue
			}
		}
		if !found {
			return false
		}
	}

	return true
}
