package mutation

type Mutator struct {
	Mode string
}

func New(mode string) *Mutator {
	return &Mutator{Mode: mode}
}

func (m *Mutator) Apply(originalValue, payload string) string {
	switch m.Mode {
	case "suffix":
		return originalValue + payload
	case "prefix":
		return payload + originalValue
	default:
		return payload
	}
}
