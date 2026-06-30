package provider

// firstNonEmpty returns the first argument that is not the empty string, or
// "" if every argument is empty.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
