package transform

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ---------------------------------------------------------------------------
// CEL expression builders
//
// chart2kro GENERATES CEL expression strings (e.g., "${schema.spec.replicas}")
// for KRO to evaluate at runtime inside the cluster. It does NOT evaluate CEL.
//
// Why NOT use cel-go or kro/pkg/cel?
//   - KRO's ${…} template syntax is a layer above standard CEL — cel-go
//     cannot parse it.
//   - kro/pkg/cel compiles and evaluates CEL against live K8s resources via
//     rest.Config — the wrong abstraction for an offline code-generator.
//   - The expressions chart2kro produces are simple field references and
//     comparisons — no type-checking or compilation benefit.
//   - Adding cel-go would bloat the binary by ~8 MB for zero runtime value.
//
// See docs/adr/001-no-kro-pkg-dependency.md for the full rationale.
// ---------------------------------------------------------------------------

// SchemaRef generates a CEL expression referencing a schema field.
// e.g., SchemaRef("spec", "replicas") => "${schema.spec.replicas}".
// Returns an error-indicative string if path is empty.
func SchemaRef(path ...string) string {
	if len(path) == 0 {
		return "${schema}"
	}

	return fmt.Sprintf("${schema.%s}", strings.Join(path, "."))
}

// ResourceRef generates a CEL expression referencing another resource's field.
// e.g., ResourceRef("deployment", "status", "availableReplicas") => "${deployment.status.availableReplicas}".
func ResourceRef(resourceID string, path ...string) string {
	if len(path) == 0 {
		return fmt.Sprintf("${%s}", resourceID)
	}

	return fmt.Sprintf("${%s.%s}", resourceID, strings.Join(path, "."))
}

// PathSegment represents a segment in a resource reference path, with an
// optional flag indicating the field may be absent.
type PathSegment struct {
	// Name is the field name (e.g., "status", "loadBalancer", "ingress[0]").
	Name string
	// Optional marks this segment with a "?" accessor for potentially absent fields.
	Optional bool
}

// ResourceRefWithOptional generates a CEL expression using optional "?" accessors
// for potentially absent status fields.
// e.g., ResourceRefWithOptional("svc", PathSegment{"status", false}, PathSegment{"loadBalancer", false},
//
//	PathSegment{"ingress[0]", true}, PathSegment{"hostname", true})
//
// => "${svc.status.loadBalancer.?ingress[0].?hostname}"
func ResourceRefWithOptional(resourceID string, segments ...PathSegment) string {
	if len(segments) == 0 {
		return fmt.Sprintf("${%s}", resourceID)
	}

	var b strings.Builder

	b.WriteString("${")
	b.WriteString(resourceID)

	for _, seg := range segments {
		b.WriteByte('.')

		if seg.Optional {
			b.WriteByte('?')
		}

		b.WriteString(seg.Name)
	}

	b.WriteByte('}')

	return b.String()
}

// SelfRef generates a CEL expression referencing the resource's own field.
// e.g., SelfRef("status", "availableReplicas") => "${self.status.availableReplicas}".
func SelfRef(path ...string) string {
	if len(path) == 0 {
		return "${self}"
	}

	return fmt.Sprintf("${self.%s}", strings.Join(path, "."))
}

// Interpolate wraps multiple expressions in a string interpolation.
// e.g., Interpolate("${schema.spec.image}", ":", "${schema.spec.tag}") => "${schema.spec.image}:${schema.spec.tag}".
func Interpolate(parts ...string) string {
	return strings.Join(parts, "")
}

// ReadyWhenCondition represents a readiness condition for a KRO resource.
type ReadyWhenCondition struct {
	// Key is the status field path to check.
	Key string
	// Operator is the comparison operator (==, !=, >, <, etc.).
	Operator string
	// Value is the expected value (can be another field reference or literal).
	Value string
}

// String returns the CEL expression for a readiness condition.
func (c ReadyWhenCondition) String() string {
	return fmt.Sprintf("${%s %s %s}", c.Key, c.Operator, c.Value)
}

// DefaultReadyWhen returns default readiness conditions for a given GVK.
func DefaultReadyWhen(gvk schema.GroupVersionKind) []ReadyWhenCondition {
	switch {
	case k8s.IsDeployment(gvk):
		return []ReadyWhenCondition{
			{Key: "self.status.availableReplicas", Operator: "==", Value: "self.status.replicas"},
		}
	case k8s.IsStatefulSet(gvk):
		return []ReadyWhenCondition{
			{Key: "self.status.readyReplicas", Operator: "==", Value: "self.status.replicas"},
		}
	case k8s.IsDaemonSet(gvk):
		return []ReadyWhenCondition{
			{Key: "self.status.numberReady", Operator: "==", Value: "self.status.desiredNumberScheduled"},
		}
	case k8s.IsService(gvk):
		return []ReadyWhenCondition{
			{Key: "self.spec.clusterIP", Operator: "!=", Value: `""`},
		}
	case k8s.IsJob(gvk):
		return []ReadyWhenCondition{
			{Key: "self.status.succeeded", Operator: ">", Value: "0"},
		}
	case k8s.IsPVC(gvk):
		return []ReadyWhenCondition{
			{Key: "self.status.phase", Operator: "==", Value: `"Bound"`},
		}
	default:
		return nil
	}
}

// StatusField represents a field to project from a resource's status.
type StatusField struct {
	// Name is the name of the field in the RGD status.
	Name string
	// CELExpression is the CEL expression to extract the value.
	CELExpression string
}

// DefaultStatusProjections returns default status fields to project for a given GVK.
// Fields that may be absent on newly created resources use optional "?" accessors.
func DefaultStatusProjections(gvk schema.GroupVersionKind, resourceID string) []StatusField {
	switch {
	case k8s.IsDeployment(gvk):
		return []StatusField{
			{Name: resourceID + "AvailableReplicas", CELExpression: ResourceRef(resourceID, "status", "availableReplicas")},
			{Name: resourceID + "ReadyReplicas", CELExpression: ResourceRef(resourceID, "status", "readyReplicas")},
		}
	case k8s.IsStatefulSet(gvk):
		return []StatusField{
			{Name: resourceID + "ReadyReplicas", CELExpression: ResourceRef(resourceID, "status", "readyReplicas")},
			{Name: resourceID + "CurrentReplicas", CELExpression: ResourceRef(resourceID, "status", "currentReplicas")},
		}
	case k8s.IsDaemonSet(gvk):
		return []StatusField{
			{Name: resourceID + "NumberReady", CELExpression: ResourceRef(resourceID, "status", "numberReady")},
			{Name: resourceID + "DesiredScheduled", CELExpression: ResourceRef(resourceID, "status", "desiredNumberScheduled")},
		}
	case k8s.IsService(gvk):
		return []StatusField{
			{Name: resourceID + "ClusterIP", CELExpression: ResourceRef(resourceID, "spec", "clusterIP")},
			{Name: resourceID + "LoadBalancerIP", CELExpression: ResourceRefWithOptional(resourceID,
				PathSegment{Name: "status"},
				PathSegment{Name: "loadBalancer"},
				PathSegment{Name: "ingress[0]", Optional: true},
				PathSegment{Name: "ip", Optional: true},
			)},
		}
	case k8s.IsJob(gvk):
		return []StatusField{
			{Name: resourceID + "Succeeded", CELExpression: ResourceRef(resourceID, "status", "succeeded")},
			{Name: resourceID + "Failed", CELExpression: ResourceRef(resourceID, "status", "failed")},
			{Name: resourceID + "CompletionTime", CELExpression: ResourceRefWithOptional(resourceID,
				PathSegment{Name: "status"},
				PathSegment{Name: "completionTime", Optional: true},
			)},
		}
	case k8s.IsPVC(gvk):
		return []StatusField{
			{Name: resourceID + "Phase", CELExpression: ResourceRef(resourceID, "status", "phase")},
		}
	default:
		return nil
	}
}

// IncludeWhenExpression generates a conditional inclusion expression.
// e.g., IncludeWhenExpression("spec", "monitoring", "enabled") => "${schema.spec.monitoring.enabled}".
func IncludeWhenExpression(path ...string) string {
	return SchemaRef(path...)
}

// IncludeCondition represents a single condition for compound includeWhen.
type IncludeCondition struct {
	// Path is the dot-separated Helm values path (e.g., "monitoring.enabled").
	Path string
	// Operator is the comparison operator (defaults to implicit truthiness if empty).
	Operator string
	// Value is the comparison value (e.g., "\"\""). Empty for truthiness checks.
	Value string
}

// CompoundIncludeWhen generates a compound CEL expression from multiple conditions.
// Conditions are joined with "&&" and enclosed in a single ${...} expression.
//
// Examples:
//
//	[{Path: "a"}]               → "${schema.spec.a}"
//	[{Path: "a"}, {Path: "b"}] → "${schema.spec.a && schema.spec.b}"
//	[{Path: "a", Operator: "!=", Value: "\"\""}] → "${schema.spec.a != \"\"}"
func CompoundIncludeWhen(conditions []IncludeCondition) string {
	if len(conditions) == 0 {
		return ""
	}

	if len(conditions) == 1 {
		return singleConditionCEL(conditions[0])
	}

	parts := make([]string, 0, len(conditions))

	for _, c := range conditions {
		parts = append(parts, conditionFragment(c))
	}

	return fmt.Sprintf("${%s}", strings.Join(parts, " && "))
}

// singleConditionCEL generates a CEL expression for a single condition.
func singleConditionCEL(c IncludeCondition) string {
	if c.Operator == "" {
		return SchemaRef("spec", c.Path)
	}

	return fmt.Sprintf("${schema.spec.%s %s %s}", c.Path, c.Operator, c.Value)
}

// conditionFragment generates the inner fragment (without ${...}) for one condition.
func conditionFragment(c IncludeCondition) string {
	if c.Operator == "" {
		return "schema.spec." + c.Path
	}

	return fmt.Sprintf("schema.spec.%s %s %s", c.Path, c.Operator, c.Value)
}

// ValidateExpression checks that a KRO CEL expression string has balanced
// ${...} delimiters and is non-empty. It does NOT compile or type-check
// the inner CEL — that is KRO's responsibility at apply time.
func ValidateExpression(expr string) error {
	if expr == "" {
		return fmt.Errorf("empty CEL expression")
	}

	depth := 0
	inExpr := false

	for i := 0; i < len(expr); i++ {
		if i+1 < len(expr) && expr[i] == '$' && expr[i+1] == '{' {
			depth++
			inExpr = true
			i++ // skip '{'

			continue
		}

		if inExpr && expr[i] == '}' {
			depth--
			if depth == 0 {
				inExpr = false
			}

			continue
		}
	}

	if depth != 0 {
		return fmt.Errorf("unbalanced CEL expression delimiters in %q", expr)
	}

	if !strings.Contains(expr, "${") {
		return fmt.Errorf("no CEL expression found in %q (expected ${...} syntax)", expr)
	}

	return nil
}
