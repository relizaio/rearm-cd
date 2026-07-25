package cli

import (
	"strings"
	"testing"
)

// The flag is an operator escape hatch for a wedged deployment, so the default
// must stay off: forcing conflicts makes ReARM CD silently overwrite fields
// another manager owns.
func TestHelmForceConflictsFlagDefaultsOff(t *testing.T) {
	orig := ForceConflicts
	defer func() { ForceConflicts = orig }()

	ForceConflicts = false
	if got := HelmForceConflictsFlag(); got != "" {
		t.Fatalf("expected no flag when disabled, got %q", got)
	}
}

func TestHelmForceConflictsFlagWhenEnabled(t *testing.T) {
	orig := ForceConflicts
	defer func() { ForceConflicts = orig }()

	ForceConflicts = true
	got := HelmForceConflictsFlag()
	if got != "--force-conflicts " {
		t.Fatalf("unexpected flag %q", got)
	}
	// Trailing space matters: the flag is concatenated directly ahead of the
	// release name, so losing it would produce "--force-conflictsrearm-watcher".
	if !strings.HasSuffix(got, " ") {
		t.Fatal("flag must carry a trailing separator")
	}
}

// Guards the concatenation shape at both call sites: "upgrade --install " +
// flag + <release-name>. A missing separator silently corrupts the command.
func TestHelmForceConflictsComposesIntoValidCommand(t *testing.T) {
	orig := ForceConflicts
	defer func() { ForceConflicts = orig }()

	for _, enabled := range []bool{true, false} {
		ForceConflicts = enabled
		cmd := "helm upgrade --install " + HelmForceConflictsFlag() + "rearm-watcher -n rearm-cd"
		if !strings.Contains(cmd, " rearm-watcher -n ") {
			t.Fatalf("release name got mangled (enabled=%v): %q", enabled, cmd)
		}
		if enabled && !strings.Contains(cmd, "--force-conflicts rearm-watcher") {
			t.Fatalf("expected flag immediately before release name: %q", cmd)
		}
		if !enabled && strings.Contains(cmd, "force-conflicts") {
			t.Fatalf("flag leaked while disabled: %q", cmd)
		}
	}
}
