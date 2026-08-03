package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// Pagination defaults applied across collection endpoints. Keeping the bounds
// here means callers default to a sane page size and can never request a page
// large enough to flood the response buffer.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// PageParams is the parsed `page` / `pageSize` query string for a list
// endpoint. Defaults are applied by ParsePageParams, so handlers can use the
// zero-value safely after calling that helper.
type PageParams struct {
	Page     int
	PageSize int
}

// ParsePageParams reads `page` and `pageSize` from a query string and applies
// defaults and bounds:
//
//   - page defaults to 1; values < 1 are coerced up
//   - pageSize defaults to defaultPageSize; values < 1 are coerced up
//     and values > maxPageSize are clamped down
//
// Non-numeric values return an error so callers can respond with 400 and the
// frontend doesn't silently get a different page than it asked for.
func ParsePageParams(q url.Values) (PageParams, error) {
	p := PageParams{Page: 1, PageSize: defaultPageSize}

	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return PageParams{}, fmt.Errorf("invalid page %q: must be a positive integer", v)
		}
		if n < 1 {
			n = 1
		}
		p.Page = n
	}

	if v := q.Get("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return PageParams{}, fmt.Errorf("invalid pageSize %q: must be a positive integer", v)
		}
		if n < 1 {
			n = 1
		}
		if n > maxPageSize {
			n = maxPageSize
		}
		p.PageSize = n
	}

	return p, nil
}

// Slice returns the [lo, hi) bounds for the requested page over a slice of
// length total. The returned bounds are always within [0, total], so callers
// can slice directly without any out-of-range checks. When the requested page
// is past the end, lo == hi == total and the caller gets an empty page.
func (p PageParams) Slice(total int) (lo, hi int) {
	if p.PageSize <= 0 {
		return 0, 0
	}
	lo = (p.Page - 1) * p.PageSize
	if lo > total {
		lo = total
	}
	if lo < 0 {
		lo = 0
	}
	hi = lo + p.PageSize
	if hi > total {
		hi = total
	}
	return lo, hi
}

// TotalPages returns the number of pages needed to cover `total` items at the
// configured PageSize. Empty results report 0 pages so the frontend can render
// a "no results" state without dividing by zero.
func (p PageParams) TotalPages(total int) int {
	if p.PageSize <= 0 || total <= 0 {
		return 0
	}
	pages := total / p.PageSize
	if total%p.PageSize != 0 {
		pages++
	}
	return pages
}

// PageMeta is the pagination envelope embedded in every list response. The
// fields mirror the request parameters so the frontend can build "next"/"prev"
// links without tracking its own state.
type PageMeta struct {
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalPages int `json:"totalPages"`
}

// Meta builds a PageMeta for a result set of size `total`.
func (p PageParams) Meta(total int) PageMeta {
	return PageMeta{
		Total:      total,
		Page:       p.Page,
		PageSize:   p.PageSize,
		TotalPages: p.TotalPages(total),
	}
}
