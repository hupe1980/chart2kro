// Package audit provides security analysis and best-practice checks for
// Kubernetes resources. It supports built-in rules, custom policy files,
// and multiple output formats (table, JSON, SARIF).
package audit

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hupe1980/chart2kro/internal/harden"
	"github.com/hupe1980/chart2kro/internal/k8s"
)

// Severity ranks the impact of a finding.
type Severity int

const (
	// SeverityInfo is purely informational.
	SeverityInfo Severity = iota
	// SeverityLow indicates a minor concern.
	SeverityLow
	// SeverityMedium indicates a moderate concern.
	SeverityMedium
	// SeverityHigh indicates a serious issue.
	SeverityHigh
	// SeverityCritical indicates an immediate security risk.
	SeverityCritical
)

// String returns the lowercase label for the severity.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// ParseSeverity parses a severity string (case-insensitive).
// Returns an error for unrecognised values.
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical, nil
	case "high":
		return SeverityHigh, nil
	case "medium":
		return SeverityMedium, nil
	case "low":
		return SeverityLow, nil
	case "info":
		return SeverityInfo, nil
	default:
		return SeverityInfo, fmt.Errorf("unknown severity %q, valid values: critical, high, medium, low, info", s)
	}
}

// Finding represents a single audit result.
type Finding struct {
	RuleID       string   `json:"ruleId"`
	Severity     Severity `json:"severity"`
	ResourceID   string   `json:"resourceId"`
	ResourceKind string   `json:"resourceKind"`
	Message      string   `json:"message"`
	Remediation  string   `json:"remediation"`
}

// Options configures the audit run.
type Options struct {
	// Level constrains which built-in checks are relevant (matches harden level).
	Level harden.SecurityLevel
	// PolicyPaths lists additional custom-policy YAML files.
	PolicyPaths []string
}

// Check is the interface every audit rule must implement.
type Check interface {
	// ID returns the unique rule identifier (e.g. "SEC-001").
	ID() string
	// Run evaluates the resources and returns any findings.
	Run(ctx context.Context, resources []*k8s.Resource) []Finding
}

// Result aggregates findings from all checks.
type Result struct {
	Findings []Finding      `json:"findings"`
	Summary  map[string]int `json:"summary"`
}

// Passed returns true when no finding meets or exceeds the threshold severity.
func (r *Result) Passed(threshold Severity) bool {
	for _, f := range r.Findings {
		if f.Severity >= threshold {
			return false
		}
	}

	return true
}

// Auditor orchestrates a set of checks against resources.
type Auditor struct {
	checks []Check
}

// New creates an Auditor with the given checks.
func New(checks ...Check) *Auditor {
	return &Auditor{checks: checks}
}

// Run executes every registered check and returns the result.
func (a *Auditor) Run(ctx context.Context, resources []*k8s.Resource) *Result {
	var all []Finding

	for _, chk := range a.checks {
		all = append(all, chk.Run(ctx, resources)...)
	}

	// Sort: severity descending, then rule ID ascending.
	sort.Slice(all, func(i, j int) bool {
		if all[i].Severity != all[j].Severity {
			return all[i].Severity > all[j].Severity
		}

		return all[i].RuleID < all[j].RuleID
	})

	summary := make(map[string]int)
	for _, f := range all {
		summary[f.Severity.String()]++
	}

	return &Result{Findings: all, Summary: summary}
}

// CheckLevel describes the minimum PSS level that activates a check.
type CheckLevel int

const (
	// CheckLevelBestPractice runs regardless of PSS level.
	CheckLevelBestPractice CheckLevel = iota
	// CheckLevelBaseline runs at baseline and above.
	CheckLevelBaseline
	// CheckLevelRestricted runs only at restricted level.
	CheckLevelRestricted
)

// checkEntry pairs a check with the minimum PSS level it requires.
type checkEntry struct {
	check Check
	level CheckLevel
}

// allChecks defines every built-in check with its PSS level category.
var allChecks = []checkEntry{
	// Baseline-level PSS checks (also run at restricted).
	{&RunAsRootCheck{}, CheckLevelBaseline},
	{&PrivilegedCheck{}, CheckLevelBaseline},
	{&HostNamespaceCheck{}, CheckLevelBaseline},
	{&DangerousCapabilitiesCheck{}, CheckLevelBaseline},

	// Restricted-level PSS checks.
	{&ReadOnlyRootFSCheck{}, CheckLevelRestricted},
	{&SeccompProfileCheck{}, CheckLevelRestricted},

	// Best-practice checks (always included).
	{&ResourceLimitsCheck{}, CheckLevelBestPractice},
	{&LatestTagCheck{}, CheckLevelBestPractice},
	{&NetworkPolicyCheck{}, CheckLevelBestPractice},
	{&BroadSelectorCheck{}, CheckLevelBestPractice},
	{&ProbeCheck{}, CheckLevelBestPractice},
	{&IngressTLSCheck{}, CheckLevelBestPractice},
}

// DefaultChecks returns the built-in security checks appropriate for the
// given PSS level:
//   - "none":       best-practice checks only (no PSS enforcement)
//   - "baseline":   best-practice + baseline PSS checks
//   - "restricted": all checks including restricted PSS rules
func DefaultChecks(level harden.SecurityLevel) []Check {
	var maxLevel CheckLevel

	switch level {
	case harden.SecurityLevelRestricted:
		maxLevel = CheckLevelRestricted
	case harden.SecurityLevelBaseline:
		maxLevel = CheckLevelBaseline
	default: // "none" or empty
		maxLevel = CheckLevelBestPractice
	}

	var checks []Check

	for _, entry := range allChecks {
		if entry.level <= maxLevel {
			checks = append(checks, entry.check)
		}
	}

	return checks
}
