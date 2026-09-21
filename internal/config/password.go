package config

import "strings"

// PasswordEnvVar is the environment variable a system's password is looked
// up under when the configuration file names none. The name derives from the
// system name so there is nothing to keep in sync by hand: uppercased, with
// every character outside A-Z and 0-9 replaced by an underscore.
func PasswordEnvVar(system string) string {
	var b strings.Builder
	b.WriteString("ZOSMF_")
	for _, r := range strings.ToUpper(system) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	b.WriteString("_PASSWORD")
	return b.String()
}
