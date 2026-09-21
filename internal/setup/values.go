package setup

// Values holds the answers, whatever collected them — flags, the wizard or
// an agent. Every value is kept as a slice so a list field and a scalar go
// through the same path.
type Values struct {
	byID  map[string][]string
	bound map[string]binding
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

func (v *Values) Has(id string) bool { return len(v.resolve(id)) > 0 }

// One returns the single value of a scalar field, or "" when unset.
func (v *Values) One(id string) string {
	if vals := v.resolve(id); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (v *Values) Many(id string) []string { return v.resolve(id) }

// The flag package fills its targets during Parse, after BindFlags has
// returned, so the pointers are remembered and read on access.
type binding struct {
	scalar *string
	list   *[]string
}

func (v *Values) registerScalar(id string, p *string) {
	if v.bound == nil {
		v.bound = map[string]binding{}
	}
	v.bound[id] = binding{scalar: p}
}

func (v *Values) registerList(id string, p *[]string) {
	if v.bound == nil {
		v.bound = map[string]binding{}
	}
	v.bound[id] = binding{list: p}
}

// resolve folds the flag targets into byID on first access.
func (v *Values) resolve(id string) []string {
	if vals, ok := v.byID[id]; ok {
		return vals
	}
	b, ok := v.bound[id]
	if !ok {
		return nil
	}
	switch {
	case b.list != nil && len(*b.list) > 0:
		return *b.list
	case b.scalar != nil && *b.scalar != "":
		return []string{*b.scalar}
	}
	return nil
}
