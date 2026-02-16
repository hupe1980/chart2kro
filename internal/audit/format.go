package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Formatter writes audit results to a writer.
type Formatter interface {
	Format(w io.Writer, result *Result) error
}

// NewFormatter returns a formatter for the given format name.
// Supported: "table" (default), "json", "sarif".
func NewFormatter(format string) (Formatter, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "table":
		return &TableFormatter{}, nil
	case "json":
		return &JSONFormatter{}, nil
	case "sarif":
		return &SARIFFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported output format %q: use table, json, or sarif", format)
	}
}

// --- Table Formatter ---

// TableFormatter writes findings as a human-readable table.
type TableFormatter struct{}

// Format writes the result as a human-readable table.
func (f *TableFormatter) Format(w io.Writer, result *Result) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintln(tw, "SEVERITY\tRULE\tRESOURCE\tMESSAGE")
	_, _ = fmt.Fprintln(tw, "--------\t----\t--------\t-------")

	for _, finding := range result.Findings {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			strings.ToUpper(finding.Severity.String()),
			finding.RuleID,
			finding.ResourceID,
			finding.Message,
		)
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Findings: %d total", len(result.Findings))

	parts := []string{}
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		if count, ok := result.Summary[sev.String()]; ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, sev.String()))
		}
	}

	if len(parts) > 0 {
		_, _ = fmt.Fprintf(w, " (%s)", strings.Join(parts, ", "))
	}

	_, _ = fmt.Fprintln(w)

	return nil
}

// --- JSON Formatter ---

// JSONFormatter writes findings as JSON.
type JSONFormatter struct{}

// Format writes the result as JSON.
func (f *JSONFormatter) Format(w io.Writer, result *Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	type jsonFinding struct {
		RuleID       string `json:"ruleId"`
		Severity     string `json:"severity"`
		ResourceID   string `json:"resourceId"`
		ResourceKind string `json:"resourceKind"`
		Message      string `json:"message"`
		Remediation  string `json:"remediation"`
	}

	type jsonResult struct {
		Findings []jsonFinding  `json:"findings"`
		Summary  map[string]int `json:"summary"`
		Total    int            `json:"total"`
	}

	findings := make([]jsonFinding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, jsonFinding{
			RuleID:       f.RuleID,
			Severity:     f.Severity.String(),
			ResourceID:   f.ResourceID,
			ResourceKind: f.ResourceKind,
			Message:      f.Message,
			Remediation:  f.Remediation,
		})
	}

	summary := result.Summary
	if summary == nil {
		summary = make(map[string]int)
	}

	return enc.Encode(jsonResult{
		Findings: findings,
		Summary:  summary,
		Total:    len(result.Findings),
	})
}

// --- SARIF v2.1.0 Formatter ---

// SARIFFormatter writes findings in SARIF v2.1.0 format.
type SARIFFormatter struct{}

// Format writes the result in SARIF v2.1.0 format.
func (f *SARIFFormatter) Format(w io.Writer, result *Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(f.toSARIF(result))
}

// sarifLog is the top-level SARIF object.
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string             `json:"id"`
	ShortDescription sarifMessage       `json:"shortDescription"`
	DefaultConfig    sarifDefaultConfig `json:"defaultConfiguration"`
}

type sarifDefaultConfig struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifLocation struct {
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations"`
}

type sarifLogicalLocation struct {
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Kind               string `json:"kind"`
}

func (f *SARIFFormatter) toSARIF(result *Result) sarifLog {
	// Collect unique rules.
	ruleMap := make(map[string]bool)

	var rules []sarifRule

	for _, finding := range result.Findings {
		if !ruleMap[finding.RuleID] {
			ruleMap[finding.RuleID] = true
			rules = append(rules, sarifRule{
				ID:               finding.RuleID,
				ShortDescription: sarifMessage{Text: finding.Message},
				DefaultConfig:    sarifDefaultConfig{Level: severityToSARIFLevel(finding.Severity)},
			})
		}
	}

	// Build results.
	var results []sarifResult

	for _, finding := range result.Findings {
		r := sarifResult{
			RuleID:  finding.RuleID,
			Level:   severityToSARIFLevel(finding.Severity),
			Message: sarifMessage{Text: finding.Message},
		}

		if finding.ResourceID != "" {
			r.Locations = []sarifLocation{{
				LogicalLocations: []sarifLogicalLocation{{
					Name:               finding.ResourceID,
					FullyQualifiedName: finding.ResourceID,
					Kind:               finding.ResourceKind,
				}},
			}}
		}

		results = append(results, r)
	}

	return sarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "chart2kro-audit",
					Version:        "1.0.0",
					InformationURI: "https://github.com/hupe1980/chart2kro",
					Rules:          rules,
				},
			},
			Results: results,
		}},
	}
}

// severityToSARIFLevel maps our severity to SARIF level values.
func severityToSARIFLevel(s Severity) string {
	switch s {
	case SeverityCritical, SeverityHigh:
		return "error"
	case SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}
