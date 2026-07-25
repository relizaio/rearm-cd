package cli

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Deployment failure reporting.
//
// Until this existed, a failed reconcile was logged to this pod's stdout and
// nowhere else: an operator only discovered a broken deploy by running
// kubectl logs. Failures are now also reported to ReARM (via
// `rearm devops instevent`) so they surface on the Instance view.
//
// Two rules govern everything here:
//
//  1. Reporting must never affect deployment. Every function below is
//     best-effort: buffer overflow drops oldest, a failed report is logged and
//     forgotten, and nothing propagates an error back into the reconcile.
//     A reporting outage must not become a deployment outage.
//  2. Command strings are never used as failure detail. Several helm
//     invocations embed --password on the command line, so only the
//     subprocess's own stderr/stdout is ever captured. The CLI redacts again
//     before transmit; this is the first of the two barriers, not the only one.

// Phase names must match the DeploymentPhase enum accepted by ReARM.
const (
	PhaseValuesMerge    = "VALUES_MERGE"
	PhaseTagReplace     = "TAG_REPLACE"
	PhaseSecrets        = "SECRETS"
	PhaseHelmInstall    = "HELM_INSTALL"
	PhaseHelmUninstall  = "HELM_UNINSTALL"
	PhaseWatcherInstall = "WATCHER_INSTALL"
	PhaseBackup         = "BACKUP"
	PhaseUnknown        = "UNKNOWN"
)

// Failure classes must match the DeploymentFailureClass enum accepted by ReARM.
const (
	ClassRbacForbidden      = "RBAC_FORBIDDEN"
	ClassChartNotFound      = "CHART_NOT_FOUND"
	ClassTimeout            = "TIMEOUT"
	ClassImagePull          = "IMAGE_PULL"
	ClassValuesInvalid      = "VALUES_INVALID"
	ClassPreconditionMissing = "PRECONDITION_MISSING"
	ClassUnknown            = "UNKNOWN"
)

// maxBufferedFailures bounds the per-loop buffer. A cluster with many broken
// deployments must not grow this without limit; oldest entries are dropped,
// since the newest reconcile's view is the accurate one.
const maxBufferedFailures = 200

// maxDetailBytes caps captured stderr at the agent boundary. The CLI truncates
// again; this keeps the buffer itself small.
const maxDetailBytes = 4096

// DeployFailure is one reported failure, shaped for `rearm devops instevent`.
type DeployFailure struct {
	DeploymentName string `json:"deploymentName"`
	Namespace      string `json:"namespace,omitempty"`
	Phase          string `json:"phase"`
	FailureClass   string `json:"failureClass"`
	Message        string `json:"message,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

// PhaseError carries the subprocess output that produced a failure, so the
// controller can classify and report it. The underlying error is preserved for
// callers that only care that something failed.
type PhaseError struct {
	Phase  string
	Detail string
	Err    error
}

func (e *PhaseError) Error() string {
	if e == nil || e.Err == nil {
		return "deployment phase failed"
	}
	return e.Err.Error()
}

func (e *PhaseError) Unwrap() error { return e.Err }

// NewPhaseError wraps err with the phase and the subprocess output. Pass only
// captured stdout/stderr -- never the command string, which may contain
// credentials.
func NewPhaseError(phase string, err error, stdout string, stderr string) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if len(detail) > maxDetailBytes {
		detail = detail[:maxDetailBytes]
	}
	return &PhaseError{Phase: phase, Detail: detail, Err: err}
}

var classPatterns = []struct {
	class string
	re    *regexp.Regexp
}{
	// Ordered by specificity: the first match wins, so narrow signatures come
	// before broad ones ("not found" appears in plenty of unrelated output).
	// Deliberately NOT matching a bare "unauthorized": registry auth failures
	// ("unauthorized: authentication required") say that too, and classifying
	// them as RBAC sends an operator to fix cluster permissions when the real
	// problem is image-pull credentials. Kubernetes RBAC denials are
	// identifiable by "is forbidden" / "cannot <verb> resource".
	{ClassRbacForbidden, regexp.MustCompile(`(?i)\bis forbidden\b|\bcannot (get|list|create|update|patch|delete|watch) resource\b|\bRBAC\b`)},
	{ClassImagePull, regexp.MustCompile(`(?i)ImagePullBackOff|ErrImagePull|\bpull access denied\b|\bmanifest unknown\b|\bunauthorized: authentication required\b`)},
	{ClassTimeout, regexp.MustCompile(`(?i)\btimed out\b|\bcontext deadline exceeded\b|\btimeout\b|\bDeadlineExceeded\b`)},
	{ClassPreconditionMissing, regexp.MustCompile(`(?i)sealed[- ]secrets.*not installed|no matches for kind|\bCRD\b.*not (found|installed)|could not find the requested resource`)},
	{ClassChartNotFound, regexp.MustCompile(`(?i)chart not found|failed to (download|pull|fetch).*chart|\bno chart (name|version) found\b|repo .* not found|\b404\b.*(chart|manifest)`)},
	{ClassValuesInvalid, regexp.MustCompile(`(?i)error converting YAML|failed to parse|cannot unmarshal|values don't meet the specifications|\byaml: line\b|invalid value|template: .*(executing|undefined)`)},
}

// ClassifyFailure maps subprocess output to a coarse cause. Unmatched output
// is UNKNOWN rather than force-fit -- a wrong cause on the Instance view is
// worse than an honest "unknown", because it sends an operator the wrong way.
func ClassifyFailure(text string) string {
	if strings.TrimSpace(text) == "" {
		return ClassUnknown
	}
	for _, p := range classPatterns {
		if p.re.MatchString(text) {
			return p.class
		}
	}
	return ClassUnknown
}

var (
	failureMu     sync.Mutex
	bufferedFails []DeployFailure
)

// RecordDeployFailure buffers a failure for the current reconcile. Safe to
// call with a nil or non-PhaseError error: phase falls back to the supplied
// value and detail is simply absent.
func RecordDeployFailure(deploymentName string, namespace string, phase string, err error) {
	if err == nil {
		return
	}
	detail := ""
	if pe, ok := err.(*PhaseError); ok {
		detail = pe.Detail
		if pe.Phase != "" {
			phase = pe.Phase
		}
	}
	if phase == "" {
		phase = PhaseUnknown
	}
	// Classify on stderr when we have it, else on the error text itself.
	classifyOn := detail
	if classifyOn == "" {
		classifyOn = err.Error()
	}
	f := DeployFailure{
		DeploymentName: deploymentName,
		Namespace:      namespace,
		Phase:          phase,
		FailureClass:   ClassifyFailure(classifyOn),
		Message:        firstLine(err.Error()),
		Detail:         detail,
	}

	failureMu.Lock()
	defer failureMu.Unlock()
	bufferedFails = append(bufferedFails, f)
	if len(bufferedFails) > maxBufferedFailures {
		// Drop oldest: the most recent reconcile's picture is the accurate one.
		bufferedFails = bufferedFails[len(bufferedFails)-maxBufferedFailures:]
	}
}

// DrainDeployFailures returns and clears the buffer.
func DrainDeployFailures() []DeployFailure {
	failureMu.Lock()
	defer failureMu.Unlock()
	if len(bufferedFails) == 0 {
		return nil
	}
	out := bufferedFails
	bufferedFails = nil
	return out
}

// ReportDeployFailures ships the buffered failures to ReARM in one batched
// call. Best-effort by contract: any problem is logged and dropped, never
// returned, so a ReARM outage cannot stall or fail the reconcile loop.
func ReportDeployFailures() {
	failures := DrainDeployFailures()
	if len(failures) == 0 {
		return
	}
	payload, err := json.Marshal(failures)
	if err != nil {
		sugar.Errorw("Unable to serialize deployment failures for reporting", "error", err)
		return
	}
	f, err := os.CreateTemp("", "rearm-deploy-failures-*.json")
	if err != nil {
		sugar.Errorw("Unable to stage deployment failure report", "error", err)
		return
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(payload); err != nil {
		f.Close()
		sugar.Errorw("Unable to write deployment failure report", "error", err)
		return
	}
	f.Close()

	_, stderr, err := shellout(RearmCliApp + " devops instevent --eventsfile " + f.Name())
	if err != nil {
		// Deliberately a warning, not an error: failing to REPORT a failure is
		// not itself a deployment problem, and the real failure is already in
		// this pod's log above.
		sugar.Warnw("Unable to report deployment failures to ReARM",
			"count", len(failures), "error", err, "stderr", strings.TrimSpace(stderr))
		return
	}
	sugar.Infow("Reported deployment failures to ReARM", "count", len(failures))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
