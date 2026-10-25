// Package chart2kro converts Helm charts into kro ResourceGraphDefinitions.
//
// The conversion renders the chart with the Helm SDK, probes every value with typed
// inputs, exposes only values whose effect an offline kro instance reproduces exactly
// (validated at the listed witnesses, not proven equivalent), and validates the
// result with kro's own admission schema and graph builder.
package chart2kro

import (
	"fmt"
	"io"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"
)

// Options configure a conversion. The zero value means Helm's defaults.
type Options struct {
	Chart    string // directory, .tgz, oci://…, or repo/chart
	Version  string // chart version (OCI and repository sources)
	RepoURL  string
	Username string
	Password string
	CAFile   string
	CertFile string
	KeyFile  string

	ValueFiles []string // -f: merged values become the API's defaults
	Set        []string
	SetString  []string
	SetFile    []string

	Kind       string // default: PascalCase chart name
	Group      string // default: kro.run
	APIVersion string // default: v1alpha1

	Expose []string // value-path globs to expose (default: everything exposable); strict
	Freeze []string // value-path globs never to expose

	Exclude          []string // resource ids to drop
	ExcludeSubcharts []string
	IncludeHooks     bool

	KubeVersion string        // default: newest embedded catalog
	CRDFiles    []string      // extra CRDs for validation
	Timeout     time.Duration // per render; default 30s
	Parallelism int           // concurrent renders; default GOMAXPROCS
	Logger      *slog.Logger  // default: discard
}

var (
	kindRe       = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]{0,62}$`)
	apiVersionRe = regexp.MustCompile(`^v[0-9]+(alpha[0-9]+|beta[0-9]+)?$`)
	groupRe      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

func (o *Options) validate() error {
	if o.Chart == "" {
		return fmt.Errorf("%w: a chart reference is required", ErrUsage)
	}

	if o.Kind != "" && !kindRe.MatchString(o.Kind) {
		return fmt.Errorf("%w: --kind %q must match %s", ErrUsage, o.Kind, kindRe)
	}

	if o.APIVersion != "" && !apiVersionRe.MatchString(o.APIVersion) {
		return fmt.Errorf("%w: --api-version %q must match %s", ErrUsage, o.APIVersion, apiVersionRe)
	}

	if o.Group != "" && !groupRe.MatchString(o.Group) {
		return fmt.Errorf("%w: --group %q must be a DNS subdomain", ErrUsage, o.Group)
	}

	for _, g := range append(append([]string{}, o.Expose...), o.Freeze...) {
		for _, seg := range strings.Split(g, ".") {
			if _, err := path.Match(seg, ""); err != nil {
				return fmt.Errorf("%w: invalid glob %q: %w", ErrUsage, g, err)
			}
		}
	}

	return nil
}

func (o *Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}

	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
