package cli

import (
	"errors"
	"strings"
	"testing"
)

// Classification drives what an operator sees on the Instance view, so the
// cases here are drawn from output ReARM CD actually produces.
func TestClassifyFailureRealWorldSignatures(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			// The failure that motivated this feature.
			name: "traefik cluster-scoped RBAC denial",
			in:   `UPGRADE FAILED: could not get information about the resource: clusterroles.rbac.authorization.k8s.io "traefik-traefik" is forbidden: User "system:serviceaccount:rearm-cd:rearm-cd" cannot get resource "clusterroles" in API group "rbac.authorization.k8s.io" at the cluster scope`,
			want: ClassRbacForbidden,
		},
		{name: "cannot list resource", in: `Error: pods is forbidden: User "x" cannot list resource "pods"`, want: ClassRbacForbidden},
		{name: "image pull backoff", in: `Back-off pulling image: ImagePullBackOff`, want: ClassImagePull},
		{name: "registry auth", in: `Error: unauthorized: authentication required`, want: ClassImagePull},
		{name: "helm timeout", in: `Error: UPGRADE FAILED: timed out waiting for the condition`, want: ClassTimeout},
		{name: "context deadline", in: `failed: context deadline exceeded`, want: ClassTimeout},
		{name: "sealed secrets absent", in: `Sealed-Secrets controller is not installed in the cluster`, want: ClassPreconditionMissing},
		{name: "missing CRD kind", in: `error validating data: no matches for kind "SealedSecret" in version "bitnami.com/v1alpha1"`, want: ClassPreconditionMissing},
		{name: "chart pull failure", in: `Error: failed to download chart for release`, want: ClassChartNotFound},
		{name: "yaml parse", in: `Error: error converting YAML to JSON: yaml: line 12: did not find expected key`, want: ClassValuesInvalid},
		{name: "template undefined", in: `template: rearm/templates/x.yaml:4:12: executing "rearm" at <.Values.missing>: nil pointer`, want: ClassValuesInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyFailure(c.in); got != c.want {
				t.Fatalf("ClassifyFailure(%.60q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

// An unrecognised failure must stay UNKNOWN. Guessing a cause sends an
// operator down the wrong path, which is worse than admitting ignorance.
func TestClassifyFailureDefaultsToUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "Error: exit status 1", "something entirely unexpected happened"} {
		if got := ClassifyFailure(in); got != ClassUnknown {
			t.Fatalf("ClassifyFailure(%q) = %s, want UNKNOWN", in, got)
		}
	}
}

// Ordering matters: output can contain several signatures at once, and the
// most specific/actionable cause should win.
func TestClassifyFailurePrefersSpecificCauseOverGenericNotFound(t *testing.T) {
	in := `Error: UPGRADE FAILED: could not get information about the resource: clusterroles "x" is forbidden; chart not found in cache`
	if got := ClassifyFailure(in); got != ClassRbacForbidden {
		t.Fatalf("expected RBAC_FORBIDDEN to win over chart-not-found, got %s", got)
	}
}

func TestNewPhaseErrorPrefersStderrAndCaps(t *testing.T) {
	e := NewPhaseError(PhaseHelmInstall, errors.New("exit status 1"), "some stdout", "the real reason")
	pe, ok := e.(*PhaseError)
	if !ok {
		t.Fatalf("expected *PhaseError, got %T", e)
	}
	if pe.Detail != "the real reason" {
		t.Fatalf("expected stderr to win, got %q", pe.Detail)
	}
	if pe.Phase != PhaseHelmInstall {
		t.Fatalf("phase not preserved: %q", pe.Phase)
	}
	if !errors.Is(e, e.(*PhaseError).Err) {
		t.Fatal("underlying error must remain unwrappable")
	}

	// Falls back to stdout when stderr is empty — some tools report on stdout.
	e2 := NewPhaseError(PhaseValuesMerge, errors.New("boom"), "stdout detail", "   ")
	if e2.(*PhaseError).Detail != "stdout detail" {
		t.Fatalf("expected stdout fallback, got %q", e2.(*PhaseError).Detail)
	}

	// Oversized output is capped at the agent boundary.
	e3 := NewPhaseError(PhaseHelmInstall, errors.New("boom"), "", strings.Repeat("x", maxDetailBytes*3))
	if len(e3.(*PhaseError).Detail) != maxDetailBytes {
		t.Fatalf("detail not capped: %d", len(e3.(*PhaseError).Detail))
	}
}

func TestNewPhaseErrorNilPassthrough(t *testing.T) {
	if NewPhaseError(PhaseHelmInstall, nil, "", "") != nil {
		t.Fatal("nil error must stay nil so success paths are unaffected")
	}
}

func TestRecordDeployFailureIgnoresNilAndBuffersOtherwise(t *testing.T) {
	DrainDeployFailures()
	RecordDeployFailure("app", "ns", PhaseHelmInstall, nil)
	if got := DrainDeployFailures(); len(got) != 0 {
		t.Fatalf("nil error must not be recorded, got %d", len(got))
	}

	RecordDeployFailure("traefik", "traefik", PhaseHelmInstall,
		NewPhaseError(PhaseHelmInstall, errors.New("exit status 1"), "", `clusterroles "x" is forbidden`))
	got := DrainDeployFailures()
	if len(got) != 1 {
		t.Fatalf("expected 1 buffered failure, got %d", len(got))
	}
	if got[0].FailureClass != ClassRbacForbidden {
		t.Fatalf("expected classification from stderr, got %s", got[0].FailureClass)
	}
	if got[0].DeploymentName != "traefik" || got[0].Namespace != "traefik" {
		t.Fatalf("identity not carried: %+v", got[0])
	}
	if len(DrainDeployFailures()) != 0 {
		t.Fatal("drain must clear the buffer so failures are not re-reported every cycle")
	}
}

// A PhaseError's own phase wins over the caller's guess, so the phase shown to
// an operator is the one that actually failed.
func TestRecordDeployFailurePrefersPhaseFromError(t *testing.T) {
	DrainDeployFailures()
	RecordDeployFailure("app", "ns", PhaseUnknown,
		NewPhaseError(PhaseValuesMerge, errors.New("bad values"), "", "error converting YAML to JSON"))
	got := DrainDeployFailures()
	if got[0].Phase != PhaseValuesMerge {
		t.Fatalf("expected phase from PhaseError, got %s", got[0].Phase)
	}
	if got[0].FailureClass != ClassValuesInvalid {
		t.Fatalf("expected VALUES_INVALID, got %s", got[0].FailureClass)
	}
}

// A plain error still yields a usable report rather than being dropped.
func TestRecordDeployFailureHandlesPlainError(t *testing.T) {
	DrainDeployFailures()
	RecordDeployFailure("app", "ns", PhaseSecrets, errors.New("hub unreachable: context deadline exceeded"))
	got := DrainDeployFailures()
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Phase != PhaseSecrets {
		t.Fatalf("expected caller phase to be used, got %s", got[0].Phase)
	}
	// Classified off the error text when there is no stderr.
	if got[0].FailureClass != ClassTimeout {
		t.Fatalf("expected TIMEOUT from error text, got %s", got[0].FailureClass)
	}
}

// The buffer must not grow without bound on a cluster where many deployments
// are broken; oldest entries are dropped.
func TestRecordDeployFailureBufferIsBounded(t *testing.T) {
	DrainDeployFailures()
	for i := 0; i < maxBufferedFailures+50; i++ {
		RecordDeployFailure("app", "ns", PhaseHelmInstall, errors.New("boom"))
	}
	got := DrainDeployFailures()
	if len(got) != maxBufferedFailures {
		t.Fatalf("expected buffer capped at %d, got %d", maxBufferedFailures, len(got))
	}
}

func TestFirstLineTrimsToSingleLine(t *testing.T) {
	if got := firstLine("first\nsecond\nthird"); got != "first" {
		t.Fatalf("got %q", got)
	}
	if got := firstLine(strings.Repeat("y", 900)); len(got) != 500 {
		t.Fatalf("expected 500-char cap, got %d", len(got))
	}
}

// Regression: a bare "unauthorized" is a registry-credential failure, not a
// Kubernetes RBAC denial. Classifying it as RBAC would send an operator to
// fix cluster permissions while the real fault is an image pull secret.
func TestClassifyFailureDistinguishesRegistryAuthFromRbac(t *testing.T) {
	if got := ClassifyFailure(`Error: unauthorized: authentication required`); got != ClassImagePull {
		t.Fatalf("registry auth misclassified as %s", got)
	}
	if got := ClassifyFailure(`Error: pull access denied for repo/image`); got != ClassImagePull {
		t.Fatalf("pull access denial misclassified as %s", got)
	}
	if got := ClassifyFailure(`clusterroles "x" is forbidden: User "y" cannot get resource "clusterroles"`); got != ClassRbacForbidden {
		t.Fatalf("genuine RBAC denial misclassified as %s", got)
	}
}
