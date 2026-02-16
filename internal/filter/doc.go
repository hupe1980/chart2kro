// Package filter implements resource filtering for chart2kro's transformation
// pipeline. It supports exclusion by kind, resource ID, subchart origin, and
// labels, as well as ExternalRef promotion and automatic dependency rewiring.
//
// The package is built around the [Filter] interface and [Chain] type, which
// allow composable, ordered filter application.
package filter
