package phpenv

import (
	"os"
	"strings"
)

// MergePathSegments prepends additions to an existing PATH string, removing duplicates.
func MergePathSegments(additions []string, existing string) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(additions))
	for _, raw := range additions {
		clean := normalizeLocalPath(raw)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	if existing != "" {
		for _, raw := range strings.Split(existing, string(os.PathListSeparator)) {
			clean := normalizeLocalPath(raw)
			if clean == "" {
				continue
			}
			key := strings.ToLower(clean)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, clean)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}
