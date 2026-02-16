package k8s

import "strings"

// IsImageDigest returns true if the image reference contains a digest reference.
// This consistently checks for @sha256: which is the standard digest format.
// Both the audit and harden packages should use this for consistent behavior.
func IsImageDigest(image string) bool {
	return strings.Contains(image, "@sha256:")
}

// HasLatestTag returns true if the image uses :latest or has no explicit tag.
// Images with digests are never considered "latest".
func HasLatestTag(image string) bool {
	if len(image) == 0 {
		return false
	}

	// Images with digest are never "latest".
	if IsImageDigest(image) {
		return false
	}

	// Extract the reference part after the last slash (to avoid matching
	// colons in the registry hostname, e.g., "registry.io:5000/app").
	ref := image
	if slashIdx := strings.LastIndex(ref, "/"); slashIdx >= 0 {
		ref = ref[slashIdx+1:]
	}

	// Check for explicit tag.
	if colonIdx := strings.LastIndex(ref, ":"); colonIdx >= 0 {
		tag := ref[colonIdx+1:]
		return tag == "latest"
	}

	// No tag specified — defaults to latest.
	return true
}
