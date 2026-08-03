package api

import (
	"errors"
	"net/http"
	"testing"
)

// TestValidateAuthConfig covers the startup check introduced for issue #296.
// The matrix encodes the intended behaviour: auth middleware is required
// unless the operator explicitly sets HUB_ALLOW_INSECURE_NO_AUTH=true.
func TestValidateAuthConfig(t *testing.T) {
	passthrough := func(next http.Handler) http.Handler { return next }

	cases := []struct {
		name          string
		allowInsecure bool
		withAuthMW    bool
		wantErr       bool
	}{
		{name: "auth wired: ok", withAuthMW: true, wantErr: false},
		{name: "no auth, no override: error", withAuthMW: false, wantErr: true},
		{name: "no auth, override set: ok", allowInsecure: true, withAuthMW: false, wantErr: false},
		{name: "auth wired, override irrelevant: ok", allowInsecure: false, withAuthMW: true, wantErr: false},
		{name: "auth wired, override set: ok", allowInsecure: true, withAuthMW: true, wantErr: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{}
			if tc.withAuthMW {
				s.SetAuthMiddleware(passthrough)
			}
			err := s.ValidateAuthConfig(tc.allowInsecure)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, ErrInsecureAuthConfig) {
					t.Fatalf("expected ErrInsecureAuthConfig, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestAuthMiddlewareConfigured(t *testing.T) {
	s := &Server{}
	if s.AuthMiddlewareConfigured() {
		t.Fatalf("expected false on fresh server")
	}
	s.SetAuthMiddleware(func(next http.Handler) http.Handler { return next })
	if !s.AuthMiddlewareConfigured() {
		t.Fatalf("expected true after SetAuthMiddleware")
	}
}
