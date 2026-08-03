package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantReg  string
		wantRepo string
		wantTag  string
	}{
		{
			name:     "docker hub namespaced with tag",
			ref:      "dpinsel/ainsel-pi:dev",
			wantReg:  "registry-1.docker.io",
			wantRepo: "dpinsel/ainsel-pi",
			wantTag:  "dev",
		},
		{
			name:     "internal registry with port and tag",
			ref:      "zot.platform.svc:5000/ainsel/pi:main",
			wantReg:  "zot.platform.svc:5000",
			wantRepo: "ainsel/pi",
			wantTag:  "main",
		},
		{
			name:     "internal registry with port, no tag defaults to latest",
			ref:      "zot.platform.svc:5000/ainsel/pi",
			wantReg:  "zot.platform.svc:5000",
			wantRepo: "ainsel/pi",
			wantTag:  "latest",
		},
		{
			name:     "localhost registry with port and tag",
			ref:      "localhost:5000/foo:bar",
			wantReg:  "localhost:5000",
			wantRepo: "foo",
			wantTag:  "bar",
		},
		{
			name:     "single-segment docker hub defaults to library and latest",
			ref:      "nginx",
			wantReg:  "registry-1.docker.io",
			wantRepo: "library/nginx",
			wantTag:  "latest",
		},
		{
			name:     "single-segment docker hub with tag",
			ref:      "nginx:1.27",
			wantReg:  "registry-1.docker.io",
			wantRepo: "library/nginx",
			wantTag:  "1.27",
		},
		{
			name:     "namespaced docker hub without tag defaults to latest",
			ref:      "dpinsel/ainsel-pi",
			wantReg:  "registry-1.docker.io",
			wantRepo: "dpinsel/ainsel-pi",
			wantTag:  "latest",
		},
		{
			name:     "registry with hostname and nested repo",
			ref:      "ghcr.io/ainsel/pi:dev",
			wantReg:  "ghcr.io",
			wantRepo: "ainsel/pi",
			wantTag:  "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, repo, tag := parseImageRef(tt.ref)
			if reg != tt.wantReg {
				t.Errorf("registry = %q, want %q", reg, tt.wantReg)
			}
			if repo != tt.wantRepo {
				t.Errorf("repository = %q, want %q", repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

func TestSplitDigest(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		wantBase   string
		wantDigest string
		wantOK     bool
	}{
		{
			name:       "digest-pinned ref",
			ref:        "dpinsel/ainsel-pi@sha256:cec864ebec545abcdef",
			wantBase:   "dpinsel/ainsel-pi",
			wantDigest: "sha256:cec864ebec545abcdef",
			wantOK:     true,
		},
		{
			name:       "tag ref has no digest",
			ref:        "dpinsel/ainsel-pi:dev",
			wantBase:   "dpinsel/ainsel-pi:dev",
			wantDigest: "",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, digest, ok := splitDigest(tt.ref)
			if base != tt.wantBase || digest != tt.wantDigest || ok != tt.wantOK {
				t.Errorf("splitDigest(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.ref, base, digest, ok, tt.wantBase, tt.wantDigest, tt.wantOK)
			}
		})
	}
}

// withTestClient swaps the shared httpClient for a short-timeout client so
// the HTTPS-first attempt against a plain-HTTP test server fails fast and the
// test stays deterministic.
func withTestClient(t *testing.T) {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{Timeout: 2 * time.Second}
	t.Cleanup(func() { httpClient = orig })
}

func TestResolveImageDigest_DigestPinnedSkipsNetwork(t *testing.T) {
	// No server: a digest-pinned ref must resolve without any network call.
	const digest = "sha256:cec864ebec545abcdef"
	got, err := resolveImageDigest(context.Background(), "dpinsel/ainsel-pi@"+digest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != digest {
		t.Errorf("digest = %q, want %q", got, digest)
	}
}

func TestResolveImageDigest_HTTPFallbackAndHeaders(t *testing.T) {
	withTestClient(t)

	const wantDigest = "sha256:deadbeefcafe"
	var gotAccept, gotPath, gotMethod string

	// Plain-HTTP server: the HTTPS-first attempt fails and the client must
	// fall back to HTTP, which reaches this handler.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Docker-Content-Digest", wantDigest)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	registry := strings.TrimPrefix(srv.URL, "http://")
	imageRef := registry + "/ainsel/pi:dev"

	got, err := resolveImageDigest(context.Background(), imageRef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantDigest {
		t.Errorf("digest = %q, want %q", got, wantDigest)
	}
	if gotMethod != http.MethodHead {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodHead)
	}
	if gotPath != "/v2/ainsel/pi/manifests/dev" {
		t.Errorf("path = %q, want %q", gotPath, "/v2/ainsel/pi/manifests/dev")
	}
	if !strings.Contains(gotAccept, "application/vnd.oci.image.manifest.v1+json") {
		t.Errorf("Accept header missing OCI manifest type: %q", gotAccept)
	}
	if !strings.Contains(gotAccept, "application/vnd.docker.distribution.manifest.v2+json") {
		t.Errorf("Accept header missing Docker manifest type: %q", gotAccept)
	}
}

func TestResolveImageDigest_NonOKStatus(t *testing.T) {
	withTestClient(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	registry := strings.TrimPrefix(srv.URL, "http://")
	_, err := resolveImageDigest(context.Background(), registry+"/ainsel/pi:dev")
	if err == nil {
		t.Fatal("expected error for non-OK status, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestResolveImageDigest_MissingDigestHeader(t *testing.T) {
	withTestClient(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 OK but no Docker-Content-Digest header.
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	registry := strings.TrimPrefix(srv.URL, "http://")
	_, err := resolveImageDigest(context.Background(), registry+"/ainsel/pi:dev")
	if err == nil {
		t.Fatal("expected error for missing digest header, got nil")
	}
	if !strings.Contains(err.Error(), "Docker-Content-Digest") {
		t.Errorf("error should mention missing header: %v", err)
	}
}
