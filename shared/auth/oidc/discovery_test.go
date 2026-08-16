package oidc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issuer": "https://idp.example.com",
			"jwks_uri": "https://idp.example.com/oauth/v2/keys",
			"userinfo_endpoint": "https://idp.example.com/oidc/v1/userinfo"
		}`))
	}))
	t.Cleanup(srv.Close)

	d, err := FetchDiscovery(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("FetchDiscovery: %v", err)
	}
	if d.JWKSURI != "https://idp.example.com/oauth/v2/keys" {
		t.Errorf("JWKSURI = %q", d.JWKSURI)
	}
	if d.UserInfoEndpoint != "https://idp.example.com/oidc/v1/userinfo" {
		t.Errorf("UserInfoEndpoint = %q", d.UserInfoEndpoint)
	}
}

func TestFetchDiscoveryStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	if _, err := FetchDiscovery(context.Background(), nil, srv.URL); err == nil {
		t.Fatal("expected error for 404 discovery document")
	}
}

func TestFetchDiscoveryInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srv.Close)

	if _, err := FetchDiscovery(context.Background(), nil, srv.URL); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchDiscoveryMissingJWKS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://idp.example.com"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := FetchDiscovery(context.Background(), nil, srv.URL); err == nil {
		t.Fatal("expected error when jwks_uri is missing")
	}
}

func TestResolveEndpointsUsesDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"issuer": "https://idp.example.com",
			"jwks_uri": "https://idp.example.com/custom/keys",
			"userinfo_endpoint": "https://idp.example.com/custom/userinfo"
		}`))
	}))
	t.Cleanup(srv.Close)

	jwks, userinfo, err := ResolveEndpoints(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("ResolveEndpoints: %v", err)
	}
	if jwks != "https://idp.example.com/custom/keys" || userinfo != "https://idp.example.com/custom/userinfo" {
		t.Errorf("got jwks=%q userinfo=%q", jwks, userinfo)
	}
}

func TestResolveEndpointsFallsBackOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	jwks, userinfo, err := ResolveEndpoints(context.Background(), nil, srv.URL)
	if err == nil {
		t.Fatal("expected discovery error to be surfaced")
	}
	wantJWKS := srv.URL + fallbackJWKSPath
	wantUserInfo := srv.URL + fallbackUserInfoPath
	if jwks != wantJWKS || userinfo != wantUserInfo {
		t.Errorf("fallback got jwks=%q userinfo=%q; want %q / %q", jwks, userinfo, wantJWKS, wantUserInfo)
	}
}
