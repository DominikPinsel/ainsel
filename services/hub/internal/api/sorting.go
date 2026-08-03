package api

import (
	"fmt"
	"net/url"
	"strings"
)

// SortParams holds the parsed orderBy/orderDir query parameters for a list
// endpoint. Zero values mean "no sort requested" (handler keeps its default
// order).
type SortParams struct {
	OrderBy string // one of the allowed column names, or "" if absent
	OrderDir string // "asc" or "desc", or "" if absent
}

// ParseSortParams reads `orderBy` and `orderDir` from a query string and
// validates them against the provided whitelist of allowed column names.
//
//   - If neither param is present, returns zero SortParams (no error).
//   - If orderBy is present but not in allowed, returns an error listing the
//     allowed values so the handler can respond with HTTP 400.
//   - If orderDir is present but not "asc" or "desc", returns an error.
//
// The whitelist comparison is case-insensitive to be forgiving of client
// casing, but the returned OrderBy is normalised to lowercase.
func ParseSortParams(q url.Values, allowed []string) (SortParams, error) {
	raw := q.Get("orderBy")
	if raw == "" {
		// No orderBy → no sort requested. Ignore orderDir even if present.
		return SortParams{}, nil
	}

	lower := strings.ToLower(raw)
	ok := false
	for _, a := range allowed {
		if strings.ToLower(a) == lower {
			ok = true
			break
		}
	}
	if !ok {
		return SortParams{}, fmt.Errorf("invalid orderBy %q: allowed values are %s", raw, strings.Join(allowed, ", "))
	}

	dir := strings.ToLower(q.Get("orderDir"))
	if dir != "" && dir != "asc" && dir != "desc" {
		return SortParams{}, fmt.Errorf("invalid orderDir %q: must be asc or desc", q.Get("orderDir"))
	}

	return SortParams{OrderBy: lower, OrderDir: dir}, nil
}
