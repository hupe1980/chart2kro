package audit

import (
	"context"
	"fmt"
	"os"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// PolicyFile represents a custom policy YAML file.
type PolicyFile struct {
	Rules []PolicyRule `json:"rules" yaml:"rules"`
}

// PolicyRule defines a single custom audit rule.
type PolicyRule struct {
	// ID is the unique rule identifier (e.g., "CUSTOM-001").
	ID string `json:"id" yaml:"id"`

	// Severity is the finding severity (critical, high, medium, low, info).
	SeverityStr string `json:"severity" yaml:"severity"`

	// Match restricts the rule to specific resource kinds.
	Match PolicyMatch `json:"match" yaml:"match"`

	// Condition is a simple check description for matching.
	// Supported: "no liveness probe", "no readiness probe",
	// "no resource limits", "uses latest tag", "privileged",
	// "host networking", "no seccomp profile".
	Condition string `json:"condition" yaml:"condition"`

	// Message is the finding message.
	Message string `json:"message" yaml:"message"`

	// Remediation suggests how to fix the issue.
	Remediation string `json:"remediation" yaml:"remediation"`
}

// PolicyMatch restricts which resources a rule applies to.
type PolicyMatch struct {
	Kind string `json:"kind" yaml:"kind"`
}

// LoadPolicyFile loads a custom policy file from disk.
func LoadPolicyFile(path string) (*PolicyFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is user-provided CLI arg, not attacker-controlled
	if err != nil {
		return nil, fmt.Errorf("reading policy file %s: %w", path, err)
	}

	var pf PolicyFile
	if err := sigsyaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing policy file %s: %w", path, err)
	}

	for _, r := range pf.Rules {
		if r.ID == "" {
			return nil, fmt.Errorf("policy file %s: rule missing required 'id' field", path)
		}

		if r.Message == "" {
			return nil, fmt.Errorf("policy file %s: rule %s missing required 'message' field", path, r.ID)
		}

		if r.SeverityStr != "" {
			if _, err := ParseSeverity(r.SeverityStr); err != nil {
				return nil, fmt.Errorf("policy file %s: rule %s: %w", path, r.ID, err)
			}
		}

		if r.Condition != "" {
			if !isKnownCondition(r.Condition) {
				return nil, fmt.Errorf("policy file %s: rule %s: unknown condition %q; supported: %s",
					path, r.ID, r.Condition, strings.Join(knownConditions(), ", "))
			}
		}
	}

	return &pf, nil
}

// ToChecks converts policy rules into audit checks.
func (pf *PolicyFile) ToChecks() []Check {
	var checks []Check

	for _, rule := range pf.Rules {
		checks = append(checks, &customRuleCheck{rule: rule})
	}

	return checks
}

// customRuleCheck implements Check for a custom policy rule.
type customRuleCheck struct {
	rule PolicyRule
}

func (c *customRuleCheck) ID() string { return c.rule.ID }

func (c *customRuleCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	for _, res := range resources {
		// Apply kind filter.
		if c.rule.Match.Kind != "" && res.Kind() != c.rule.Match.Kind {
			continue
		}

		if c.matchesCondition(res) {
			sev, _ := ParseSeverity(c.rule.SeverityStr)
			findings = append(findings, Finding{
				RuleID:       c.rule.ID,
				Severity:     sev,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      c.rule.Message,
				Remediation:  c.rule.Remediation,
			})
		}
	}

	return findings
}

// matchesCondition evaluates the rule condition against a resource.
func (c *customRuleCheck) matchesCondition(res *k8s.Resource) bool {
	if !isWorkload(res) {
		return false
	}

	podSpec := getPodSpec(res)
	if podSpec == nil {
		return false
	}

	condition := strings.ToLower(strings.TrimSpace(c.rule.Condition))

	switch condition {
	case "no liveness probe":
		return hasContainerWithout(podSpec, "livenessProbe")
	case "no readiness probe":
		return hasContainerWithout(podSpec, "readinessProbe")
	case "no resource limits":
		return hasContainerWithoutLimits(podSpec)
	case "uses latest tag":
		return hasContainerWithLatestTag(podSpec)
	case "privileged":
		return hasPrivilegedContainer(podSpec)
	case "host networking":
		if val, ok := podSpec["hostNetwork"].(bool); ok && val {
			return true
		}

		return false
	case "no seccomp profile":
		return hasContainerWithoutSeccomp(podSpec)
	default:
		// This is unreachable when conditions are validated at load time via
		// LoadPolicyFile, but we keep it as a safety net.
		return false
	}
}

// knownConditions returns the list of supported condition strings.
func knownConditions() []string {
	return []string{
		"no liveness probe",
		"no readiness probe",
		"no resource limits",
		"uses latest tag",
		"privileged",
		"host networking",
		"no seccomp profile",
	}
}

// isKnownCondition reports whether the given condition string is supported.
func isKnownCondition(cond string) bool {
	normalized := strings.ToLower(strings.TrimSpace(cond))
	for _, c := range knownConditions() {
		if c == normalized {
			return true
		}
	}

	return false
}

func hasContainerWithout(podSpec map[string]interface{}, field string) bool {
	for _, container := range getContainers(podSpec, "containers") {
		if _, ok := container[field]; !ok {
			return true
		}
	}

	return false
}

func hasContainerWithoutLimits(podSpec map[string]interface{}) bool {
	for _, container := range allContainers(podSpec) {
		res, _ := container["resources"].(map[string]interface{})
		limits, _ := res["limits"].(map[string]interface{})

		if len(limits) == 0 {
			return true
		}
	}

	return false
}

func hasContainerWithLatestTag(podSpec map[string]interface{}) bool {
	for _, container := range allContainers(podSpec) {
		image, _ := container["image"].(string)
		if image != "" && hasLatestTag(image) {
			return true
		}
	}

	return false
}

func hasPrivilegedContainer(podSpec map[string]interface{}) bool {
	for _, container := range allContainers(podSpec) {
		sc, _ := container["securityContext"].(map[string]interface{})
		if priv, ok := sc["privileged"].(bool); ok && priv {
			return true
		}
	}

	return false
}

func hasContainerWithoutSeccomp(podSpec map[string]interface{}) bool {
	podSC, _ := podSpec["securityContext"].(map[string]interface{})
	_, podHas := podSC["seccompProfile"]

	for _, container := range allContainers(podSpec) {
		sc, _ := container["securityContext"].(map[string]interface{})
		_, has := sc["seccompProfile"]

		if !podHas && !has {
			return true
		}
	}

	return false
}
