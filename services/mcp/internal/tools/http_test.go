package tools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

func TestHubPostAcceptsCreated(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new"}`))
	}))
	t.Cleanup(srv.Close)

	body, err := hubPost(context.Background(), srv.Client(), srv.URL, "/api/v1/triggers", strings.NewReader(`{"name":"x"}`))
	if err != nil {
		t.Fatalf("hubPost: %v", err)
	}
	if !strings.Contains(string(body), `"id":"new"`) {
		t.Errorf("body = %q", string(body))
	}
	if !strings.Contains(string(gotBody), `"name":"x"`) {
		t.Errorf("sent body = %q", string(gotBody))
	}
}

func TestHubPutAcceptsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`{"updated":true}`))
	}))
	t.Cleanup(srv.Close)

	body, err := hubPut(context.Background(), srv.Client(), srv.URL, "/api/v1/triggers/t-1", strings.NewReader(`{"name":"x"}`))
	if err != nil {
		t.Fatalf("hubPut: %v", err)
	}
	if !strings.Contains(string(body), `"updated":true`) {
		t.Errorf("body = %q", string(body))
	}
}

func TestHubPostRejectsBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := hubPost(context.Background(), srv.Client(), srv.URL, "/x", strings.NewReader(`{}`)); err == nil {
		t.Fatal("expected error for 400")
	}
}

func TestHubGetForwardsAuthorizationFromContext(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	ctx := oidc.ContextWithToken(context.Background(), "test-token-xyz")
	body, err := hubGet(ctx, srv.Client(), srv.URL, "/api/v1/agents")
	if err != nil {
		t.Fatalf("hubGet: %v", err)
	}
	if !strings.Contains(string(body), `"ok":true`) {
		t.Errorf("body = %q; want contains \"ok\":true", string(body))
	}
	if gotAuth != "Bearer test-token-xyz" {
		t.Errorf("forwarded Authorization = %q; want Bearer test-token-xyz", gotAuth)
	}
}

func TestHubGetOmitsAuthorizationWhenNoTokenInContext(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := hubGet(context.Background(), srv.Client(), srv.URL, "/x"); err != nil {
		t.Fatalf("hubGet: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q; want empty", gotAuth)
	}
}
