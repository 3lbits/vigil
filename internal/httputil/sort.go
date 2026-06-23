package httputil

// NormalizeSortDir clamps sort direction to asc/desc.
func NormalizeSortDir(raw string) string {
	if raw == "desc" {
		return "desc"
	}
	return "asc"
}
