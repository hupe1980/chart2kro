package chart2kro

// ReportSchema identifies the report JSON format (a public contract).
const ReportSchema = "chart2kro.io/report/v1"

// Report explains a conversion: every chart value with its outcome.
type Report struct {
	SchemaVersion string           `json:"schemaVersion"`
	Chart         ChartRef         `json:"chart"`
	Kind          string           `json:"kind"`
	Group         string           `json:"group"`
	APIVersion    string           `json:"apiVersion"`
	KubeVersion   string           `json:"kubeVersion"`
	KroVersion    string           `json:"kroVersion"`
	Witnesses     int              `json:"witnesses"`
	Summary       Summary          `json:"summary"`
	Values        []ValueReport    `json:"values"`
	Resources     []ResourceReport `json:"resources"`
	Prerequisites []ObjectRef      `json:"prerequisites,omitempty"`
	Dropped       []DroppedObject  `json:"dropped,omitempty"`
	Inputs        []InputReport    `json:"inputs,omitempty"`
	Warnings      []string         `json:"warnings,omitempty"`
}

// ChartRef identifies the converted chart.
type ChartRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Digest  string `json:"digest,omitempty"`
}

// Summary counts value outcomes.
type Summary struct {
	Exposed int `json:"exposed"`
	Frozen  int `json:"frozen"`
	Failed  int `json:"failed"`
	Unused  int `json:"unused"`
}

// ValueReport is one chart value.
type ValueReport struct {
	Path    string   `json:"path"`
	Outcome string   `json:"outcome"`          // exposed | frozen | failed | unused
	Reason  string   `json:"reason,omitempty"` // stable code for frozen/failed
	Detail  string   `json:"detail,omitempty"`
	Kind    string   `json:"kind,omitempty"` // reference | interpolation | conditional | whole
	Type    string   `json:"type,omitempty"` // SimpleSchema type when exposed
	Default any      `json:"default,omitempty"`
	Targets []string `json:"targets,omitempty"` // "<resourceId>.<field path>"
	// Description comes from the chart's values.yaml comments.
	Description string `json:"description,omitempty"`
}

// ResourceReport is one resource of the RGD.
type ResourceReport struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Source      string   `json:"source"`
	IncludeWhen []string `json:"includeWhen,omitempty"`
	Isolated    bool     `json:"isolated,omitempty"`
	ForEach     string   `json:"forEach,omitempty"` // iterated list (collections only)
}

// ObjectRef names an object moved to prerequisites.
type ObjectRef struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// DroppedObject is an object left out, with the reason.
type DroppedObject struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// InputReport is a schema field without a default that every instance must set.
type InputReport struct {
	Path   string `json:"path"`
	Reason string `json:"reason"` // user-secret | generated
}
