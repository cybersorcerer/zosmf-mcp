package setup

import "testing"

// One answer, three variables: nobody should have to know that LIBPATH
// points at lib.
func TestZOAUEnvironmentDerivesThreeVariables(t *testing.T) {
	entries := ZOAUEnvironment("/var/devops/zoautil")
	if len(entries) != 1 || entries[0].Key != "environment" {
		t.Fatalf("expected one environment entry, got %+v", entries)
	}
	env, ok := entries[0].Value.(map[string]string)
	if !ok {
		t.Fatalf("value is %T, want map[string]string", entries[0].Value)
	}
	for key, want := range map[string]string{
		"ZOAU_HOME": "/var/devops/zoautil",
		"PATH":      "/var/devops/zoautil/bin:/bin:/usr/bin",
		"LIBPATH":   "/var/devops/zoautil/lib:/lib:/usr/lib",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
}

func TestSSHFieldsCoverWhatRunSSHNeeds(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range SSHFields() {
		seen[f.ID] = true
		if f.Group != "zoau" {
			t.Errorf("field %s is in group %q, want zoau", f.ID, f.Group)
		}
	}
	for _, want := range []string{"ssh-host", "ssh-user", "ssh-key-file", "zoau-path"} {
		if !seen[want] {
			t.Errorf("SSH catalog is missing %q", want)
		}
	}
}
