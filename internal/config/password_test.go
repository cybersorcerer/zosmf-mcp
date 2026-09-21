package config

import "testing"

func TestPasswordEnvVar(t *testing.T) {
	for _, tc := range []struct{ system, want string }{
		{"MAINT", "ZOSMF_MAINT_PASSWORD"},
		{"maint", "ZOSMF_MAINT_PASSWORD"},
		{"PROD-A", "ZOSMF_PROD_A_PASSWORD"},
		{"sys.1", "ZOSMF_SYS_1_PASSWORD"},
	} {
		if got := PasswordEnvVar(tc.system); got != tc.want {
			t.Errorf("PasswordEnvVar(%q) = %q, want %q", tc.system, got, tc.want)
		}
	}
}

const yamlTwoSystems = `
servers:
  - name: MAINT
    host: https://zos1.example.com:443
    user: IBMUSER
  - name: PROD
    host: https://zos2.example.com:443
    user: IBMUSER
`

func TestSystemEnvPasswordWins(t *testing.T) {
	unsetEnv(t, "ZOSMF_PASSWORD")
	t.Setenv("ZOSMF_MAINT_PASSWORD", "from-system-var")

	cfg := loadTestConfig(t, yamlTwoSystems)

	if got := cfg.Systems[0].Auth.Password; got != "from-system-var" {
		t.Errorf("MAINT password = %q, want from-system-var", got)
	}
	if got := cfg.Systems[0].PasswordSource; got != "system-env" {
		t.Errorf("MAINT source = %q, want system-env", got)
	}
	if got := cfg.Systems[1].Auth.Password; got != "" {
		t.Errorf("PROD password = %q, want empty — the variable is MAINT's", got)
	}
	if got := cfg.Systems[1].PasswordSource; got != "none" {
		t.Errorf("PROD source = %q, want none", got)
	}
}

func TestGlobalEnvPasswordStillApplies(t *testing.T) {
	unsetEnv(t, "ZOSMF_MAINT_PASSWORD")
	t.Setenv("ZOSMF_PASSWORD", "from-global-var")

	cfg := loadTestConfig(t, yamlTwoSystems)

	for i, sys := range cfg.Systems {
		if sys.Auth.Password != "from-global-var" {
			t.Errorf("system %d password = %q, want from-global-var", i, sys.Auth.Password)
		}
		if sys.PasswordSource != "global-env" {
			t.Errorf("system %d source = %q, want global-env", i, sys.PasswordSource)
		}
	}
}

// A system-specific variable beats the global one; both lose to the file.
func TestFilePasswordBeatsBothVariables(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "from-global-var")
	t.Setenv("ZOSMF_MAINT_PASSWORD", "from-system-var")

	cfg := loadTestConfig(t, `
servers:
  - name: MAINT
    host: https://zos1.example.com:443
    user: IBMUSER
    auth:
      password: in-the-file
`)

	if got := cfg.Systems[0].Auth.Password; got != "in-the-file" {
		t.Errorf("password = %q, want in-the-file", got)
	}
	if got := cfg.Systems[0].PasswordSource; got != "file" {
		t.Errorf("source = %q, want file", got)
	}
}
