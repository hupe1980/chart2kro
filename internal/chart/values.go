package chart

import (
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
)

// UserValues merges -f/--set/--set-string/--set-file exactly like the helm CLI.
func UserValues(files, set, setString, setFile []string) (map[string]any, error) {
	o := values.Options{ValueFiles: files, Values: set, StringValues: setString, FileValues: setFile}

	v, err := o.MergeValues(getter.All(cli.New()))
	if err != nil {
		return nil, err
	}

	if v == nil {
		v = map[string]any{}
	}

	return v, nil
}
