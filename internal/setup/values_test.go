package setup

import "testing"

// A flag value the user actually passed can still be overridden by an
// answer set afterwards — by the wizard, or anything else calling Set —
// which must win: resolve checks its own map before the flag bindings.
func TestSetOverridesFlagBinding(t *testing.T) {
	v := parse(t, "--reject-unauthorized", "false")

	if got := v.One("reject-unauthorized"); got != "false" {
		t.Fatalf("reject-unauthorized = %q, want the flag value %q", got, "false")
	}

	v.Set("reject-unauthorized", []string{"true"})

	if got := v.One("reject-unauthorized"); got != "true" {
		t.Errorf("reject-unauthorized = %q, want the wizard's answer %q", got, "true")
	}
}
