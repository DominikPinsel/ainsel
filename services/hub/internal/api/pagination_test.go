package api

import (
	"net/url"
	"testing"
)

func TestParsePageParams_Defaults(t *testing.T) {
	p, err := ParsePageParams(url.Values{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Page != 1 {
		t.Errorf("expected default page=1, got %d", p.Page)
	}
	if p.PageSize != defaultPageSize {
		t.Errorf("expected default pageSize=%d, got %d", defaultPageSize, p.PageSize)
	}
}

func TestParsePageParams_Explicit(t *testing.T) {
	q := url.Values{}
	q.Set("page", "3")
	q.Set("pageSize", "25")
	p, err := ParsePageParams(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Page != 3 {
		t.Errorf("expected page=3, got %d", p.Page)
	}
	if p.PageSize != 25 {
		t.Errorf("expected pageSize=25, got %d", p.PageSize)
	}
}

func TestParsePageParams_ClampsAndCoerces(t *testing.T) {
	cases := []struct {
		name         string
		page         string
		pageSize     string
		wantPage     int
		wantPageSize int
	}{
		{"page below 1 coerced to 1", "0", "10", 1, 10},
		{"negative page coerced to 1", "-5", "10", 1, 10},
		{"pageSize below 1 coerced to 1", "1", "0", 1, 1},
		{"pageSize above max clamped", "1", "9999", 1, maxPageSize},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("page", c.page)
			q.Set("pageSize", c.pageSize)
			p, err := ParsePageParams(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Page != c.wantPage {
				t.Errorf("expected page=%d, got %d", c.wantPage, p.Page)
			}
			if p.PageSize != c.wantPageSize {
				t.Errorf("expected pageSize=%d, got %d", c.wantPageSize, p.PageSize)
			}
		})
	}
}

func TestParsePageParams_InvalidValuesReturnError(t *testing.T) {
	cases := []url.Values{
		{"page": []string{"abc"}},
		{"pageSize": []string{"xyz"}},
	}
	for _, q := range cases {
		if _, err := ParsePageParams(q); err == nil {
			t.Errorf("expected error for %v, got nil", q)
		}
	}
}

func TestPageParams_Slice(t *testing.T) {
	cases := []struct {
		name           string
		page, pageSize int
		total          int
		wantLo, wantHi int
	}{
		{"first page within range", 1, 10, 25, 0, 10},
		{"second page within range", 2, 10, 25, 10, 20},
		{"last page partial", 3, 10, 25, 20, 25},
		{"page past end is empty", 5, 10, 25, 25, 25},
		{"page exactly at end", 3, 10, 30, 20, 30},
		{"single item, single page", 1, 50, 1, 0, 1},
		{"empty total", 1, 10, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := PageParams{Page: c.page, PageSize: c.pageSize}
			lo, hi := p.Slice(c.total)
			if lo != c.wantLo || hi != c.wantHi {
				t.Errorf("expected (%d, %d), got (%d, %d)", c.wantLo, c.wantHi, lo, hi)
			}
		})
	}
}

func TestPageParams_TotalPages(t *testing.T) {
	cases := []struct {
		name           string
		page, pageSize int
		total, want    int
	}{
		{"exact multiple", 1, 10, 30, 3},
		{"with remainder", 1, 10, 31, 4},
		{"under one page", 1, 50, 25, 1},
		{"zero total", 1, 50, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := PageParams{Page: c.page, PageSize: c.pageSize}
			if got := p.TotalPages(c.total); got != c.want {
				t.Errorf("TotalPages(%d) = %d, want %d", c.total, got, c.want)
			}
		})
	}
}

func TestPageParams_Meta(t *testing.T) {
	p := PageParams{Page: 2, PageSize: 10}
	m := p.Meta(25)
	if m.Total != 25 || m.Page != 2 || m.PageSize != 10 || m.TotalPages != 3 {
		t.Errorf("unexpected meta: %+v", m)
	}
}
