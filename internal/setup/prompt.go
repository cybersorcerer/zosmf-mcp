package setup

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// AskAll walks the system catalog once, asking for every field, then offers
// the ZOAU group as a single yes/no gate before asking those questions too
// — a user who wants no ZOAU tools should not have to answer, or even see,
// questions about them.
func AskAll(fields, zoauFields []Field, v *Values, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	if err := askFields(fields, v, r, out); err != nil {
		return err
	}

	if !v.Has("csi") || !v.Has("zones") {
		_, _ = fmt.Fprintln(out, "\nNote: without csi and zones every SMP/E tool call has to name them,")
		_, _ = fmt.Fprintln(out, "or it fails with \"no csi_dataset provided\" / \"no zone provided\".")
	}

	_, _ = fmt.Fprintln(out, "\nConfigure the ZOAU tools for this system? They run over SSH, not z/OSMF.")
	_, _ = fmt.Fprint(out, "y/N> ")
	line, _ := r.ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(line), "y") {
		if err := askFields(zoauFields, v, r, out); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintln(out, "\nThe password is not stored in this file. Run `zosmf-mcp init env` next.")
	return nil
}

// askFields asks for every field in one group. An empty answer leaves an
// optional field unset (falling back to its default, if any); a required
// field is asked again.
func askFields(fields []Field, v *Values, r *bufio.Reader, out io.Writer) error {
	for _, f := range fields {
		for {
			prompt(out, f)
			line, err := r.ReadString('\n')
			if err != nil && line == "" {
				if f.Required {
					return fmt.Errorf("input ended before %s was answered", f.ID)
				}
				break
			}
			answer := strings.TrimSpace(line)
			if answer == "" {
				if f.Required {
					_, _ = fmt.Fprintf(out, "  %s is required.\n", f.ID)
					continue
				}
				if f.Default != "" {
					v.Set(f.ID, []string{f.Default})
				}
				break
			}
			if f.Cardinality == Many {
				var vals []string
				for _, part := range strings.Split(answer, ",") {
					if part = strings.TrimSpace(part); part != "" {
						vals = append(vals, part)
					}
				}
				v.Set(f.ID, vals)
			} else {
				v.Set(f.ID, []string{answer})
			}
			break
		}
	}
	return nil
}

// AskSecret reads one secret. The value is never echoed back by us; a
// terminal that echoes input is the caller's environment, not our doing.
func AskSecret(name string, in io.Reader, out io.Writer) (string, error) {
	_, _ = fmt.Fprintf(out, "\n%s\n  Secret. It goes into the environment file, never into the configuration.\n> ", name)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("input ended before %s was answered", name)
	}
	return strings.TrimSpace(line), nil
}

func prompt(out io.Writer, f Field) {
	marker := ""
	if f.Required {
		marker = " (required)"
	}
	_, _ = fmt.Fprintf(out, "\n%s%s\n  %s\n", f.Label, marker, f.Help)
	if f.Cardinality == Many {
		_, _ = fmt.Fprintln(out, "  several values, comma separated")
	}
	if f.Example != nil {
		_, _ = fmt.Fprintf(out, "  example: %v\n", f.Example)
	}
	if f.Default != "" {
		_, _ = fmt.Fprintf(out, "  default: %s\n", f.Default)
	}
	_, _ = fmt.Fprint(out, "> ")
}
