package chart2kro

import "errors"

// Sentinel errors. Wrap them with fmt.Errorf("...: %w", ErrX); ExitCode maps them.
var (
	ErrUsage      = errors.New("usage error")
	ErrLoad       = errors.New("chart could not be loaded")
	ErrRender     = errors.New("chart could not be rendered")
	ErrConstraint = errors.New("conversion constraint not met")
	ErrWrite      = errors.New("write failed")
	ErrInvalid    = errors.New("kro rejects the ResourceGraphDefinition")
	ErrBreaking   = errors.New("breaking change")
	ErrStale      = errors.New("committed output is stale")
)

// ExitCode maps an error to the documented process exit code.
//
//	0 success · 1 internal · 2 usage · 3 load · 4 render · 5 constraint
//	6 write · 7 kro rejects · 8 breaking change · 9 stale
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ErrUsage):
		return 2
	case errors.Is(err, ErrLoad):
		return 3
	case errors.Is(err, ErrRender):
		return 4
	case errors.Is(err, ErrConstraint):
		return 5
	case errors.Is(err, ErrWrite):
		return 6
	case errors.Is(err, ErrInvalid):
		return 7
	case errors.Is(err, ErrBreaking):
		return 8
	case errors.Is(err, ErrStale):
		return 9
	default:
		return 1
	}
}
