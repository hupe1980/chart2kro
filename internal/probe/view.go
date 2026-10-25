package probe

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"

	"github.com/hupe1980/chart2kro/internal/chart"
	"github.com/hupe1980/chart2kro/internal/manifest"
)

// Obj is a rendered object in comparison form: numbers normalized and Secret data
// base64-decoded (B64 records which paths were decoded) so substitutions are visible.
type Obj struct {
	Key    string
	Source string
	Hook   []string
	CRD    bool
	Value  map[string]any
	B64    map[string]bool // path keys of decoded Secret data entries
}

// Kind returns the object's kind.
func (o *Obj) Kind() string { s, _ := o.Value["kind"].(string); return s }

// APIVersion returns the object's apiVersion.
func (o *Obj) APIVersion() string { s, _ := o.Value["apiVersion"].(string); return s }

// Name returns metadata.name.
func (o *Obj) Name() string {
	md, _ := o.Value["metadata"].(map[string]any)
	s, _ := md["name"].(string)

	return s
}

// View is a render in comparison form, keyed by stable identity.
type View struct {
	Objects map[string]*Obj
	Order   []string // keys in render order
}

// NewView converts rendered objects.
func NewView(objs []chart.Object) *View {
	v := &View{Objects: map[string]*Obj{}}

	for _, o := range objs {
		key := o.Key()
		if o.CRD {
			key = "crd#" + o.Source
		}

		val := manifest.NormalizeMap(o.Obj)
		ob := &Obj{Key: key, Source: o.Source, Hook: o.Hook, CRD: o.CRD, Value: val, B64: map[string]bool{}}

		if val["kind"] == "Secret" {
			if data, ok := val["data"].(map[string]any); ok {
				for k, raw := range data {
					s, ok := raw.(string)
					if !ok {
						continue
					}

					dec, err := base64.StdEncoding.DecodeString(s)
					if err != nil || !utf8.Valid(dec) {
						continue
					}

					data[k] = string(dec)
					ob.B64[manifest.Path{"data", k}.Key()] = true
				}
			}
		}

		if _, dup := v.Objects[key]; !dup {
			v.Order = append(v.Order, key)
		}

		v.Objects[key] = ob
	}

	return v
}

// Noise holds paths that differ between two identical renders (generated values).
type Noise map[string]map[string]bool // object key → path key → true

// Has reports whether p (or an ancestor of p) is noise in object key.
func (n Noise) Has(key string, p manifest.Path) bool {
	m := n[key]
	if len(m) == 0 {
		return false
	}

	for i := len(p); i >= 0; i-- {
		if m[p[:i].Key()] {
			return true
		}
	}

	return false
}

// AddNoise records differences between two renders of the same inputs.
func AddNoise(n Noise, a, b *View) {
	for key, oa := range a.Objects {
		ob, ok := b.Objects[key]
		if !ok {
			continue
		}

		for _, c := range manifest.Diff(oa.Value, ob.Value) {
			if n[key] == nil {
				n[key] = map[string]bool{}
			}

			n[key][c.Path.Key()] = true
		}
	}
}

// IsChecksumAnnotation reports whether p is a checksum/* annotation (pod template or object).
func IsChecksumAnnotation(p manifest.Path) bool {
	if len(p) < 3 {
		return false
	}

	k, ok := p[len(p)-1].(string)
	if !ok || !strings.HasPrefix(k, "checksum/") {
		return false
	}

	return p[len(p)-2] == "annotations"
}
