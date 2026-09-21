package setup

// Values holds the answers, whatever collected them — flags, the wizard or
// an agent. Every value is kept as a slice so a list field and a scalar go
// through the same path.
type Values struct {
	byID map[string][]string
}

func NewValues() *Values {
	return &Values{byID: map[string][]string{}}
}

func (v *Values) Set(id string, vals []string) {
	if v.byID == nil {
		v.byID = map[string][]string{}
	}
	v.byID[id] = vals
}

func (v *Values) Has(id string) bool {
	return len(v.byID[id]) > 0
}

// One returns the single value of a scalar field, or "" when unset.
func (v *Values) One(id string) string {
	if vals := v.byID[id]; len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (v *Values) Many(id string) []string {
	return v.byID[id]
}
