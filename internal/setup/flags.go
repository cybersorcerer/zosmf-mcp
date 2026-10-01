package setup

import (
	"github.com/cybersorcerer/zosmf-mcp/internal/config"

	"flag"
	"fmt"
	"strings"
)

// listValue collects a repeatable flag and splits comma-separated input, so
// --csi A --csi B and --csi A,B mean the same thing.
type listValue struct{ out *[]string }

func (l listValue) String() string { return strings.Join(*l.out, ",") }

func (l listValue) Set(s string) error {
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*l.out = append(*l.out, part)
		}
	}
	return nil
}

// BindFlags registers one flag per catalog entry and returns the container
// the parsed answers land in.
func BindFlags(fs *flag.FlagSet, fields []Field) *Values {
	v := NewValues()
	for _, f := range fields {
		field := f
		usage := field.Help
		if field.Example != nil {
			usage = fmt.Sprintf("%s (e.g. %v)", usage, field.Example)
		}
		if field.Cardinality == Many {
			var out []string
			fs.Var(listValue{&out}, field.ID, usage)
			v.registerList(field.ID, &out)
			continue
		}
		// The flag's own default is "", not field.Default: Values.Has must
		// mean "the user answered this flag", so a rerun that omits a flag
		// leaves the existing file value alone instead of overwriting it
		// with the catalog default (e.g. rejectUnauthorized flipping back
		// to true). The wizard applies field.Default itself; --questions
		// still publishes it for documentation.
		s := fs.String(field.ID, "", usage)
		v.registerScalar(field.ID, s)
	}
	return v
}

// Validate reports every missing required field at once, naming the flag and
// an example, so one correction is enough.
func Validate(fields []Field, v *Values) error {
	var problems []string
	for _, f := range fields {
		if !f.Required || v.Has(f.ID) {
			continue
		}
		msg := fmt.Sprintf("missing --%s (%s)", f.ID, strings.ToLower(strings.TrimSuffix(f.Label, ".")))
		if f.Example != nil {
			msg = fmt.Sprintf("%s, e.g. %v", msg, f.Example)
		}
		problems = append(problems, msg)
	}
	for _, f := range fields {
		if f.ID == "host" && v.Has("host") {
			if err := config.CheckHost(v.One("host")); err != nil {
				problems = append(problems, "--host: "+err.Error())
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}
