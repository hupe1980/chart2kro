// Package output provides deterministic YAML/JSON serialization, output
// writers, resource splitting, and validation for KRO ResourceGraphDefinitions.
//
// The package is organized around five concerns:
//
//   - Serialization (serializer.go): Canonical YAML/JSON with deterministic key
//     ordering, null stripping, and optional CEL expression comments.
//
//   - Writers (writer.go): Pluggable output destinations via the [Writer]
//     interface, with [StdoutWriter] and [FileWriter] implementations.
//
//   - Splitting (splitter.go): Break a multi-resource RGD into per-resource
//     files with a generated kustomization.yaml.
//
//   - Validation (validator.go): Structural and semantic validation of RGD
//     maps including required fields, schema types, CEL references, and
//     dependency cycle detection.
//
//   - Formatting (FormatKustomizeDir): Produce a Kustomize-ready directory
//     with the full RGD and a kustomization.yaml.
package output
