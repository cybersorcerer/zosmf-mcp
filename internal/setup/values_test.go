package setup

import "testing"

// A flag can supply a default, but an answer set afterwards — by the
// wizard, or anything else calling Set — must win: resolve checks its own
// map before the flag bindings.
func TestSetOverridesFlagBinding(t *testing.T) {
	v := parse(t)

	if got := v.One("reject-unauthorized"); got != "true" {
		t.Fatalf("flag default reject-unauthorized = %q, want %q", got, "true")
	}

	v.Set("reject-unauthorized", []string{"false"})

	if got := v.One("reject-unauthorized"); got != "false" {
		t.Errorf("reject-unauthorized = %q, want the wizard's answer %q", got, "false")
	}
}
