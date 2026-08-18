package localauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if err := VerifyPassword("s3cret-password", hash); err != nil {
		t.Fatalf("VerifyPassword(correct): %v", err)
	}
	if err := VerifyPassword("wrong", hash); err == nil {
		t.Fatal("VerifyPassword(wrong) must fail")
	}
	if err := VerifyPassword("", hash); err == nil {
		t.Fatal("VerifyPassword(empty) must fail")
	}
}

func TestVerifyPasswordNoCredentials(t *testing.T) {
	if err := VerifyPassword("anything", ""); err == nil {
		t.Fatal("empty stored hash must fail")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, bad := range []string{
		"not-a-hash",
		"$bcrypt$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$garbage$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$!!!",
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$",
	} {
		if err := VerifyPassword("x", bad); err == nil {
			t.Fatalf("VerifyPassword(%q) must fail", bad)
		}
	}
}

func TestVerifyPasswordDifferentParams(t *testing.T) {
	// Hashes are self-describing: verification must honor the stored params,
	// so a hash produced with different cost settings still verifies.
	hash, err := HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	// Tamper the stored cost up; verification should recompute with the
	// claimed params and fail the comparison only if params were ignored —
	// here we just assert parsing different-but-valid params works by
	// rebuilding a hash with non-default params manually.
	alt := "$argon2id$v=19$m=1024,t=1,p=1$" +
		strings.Split(hash, "$")[4] + "$" + strings.Split(hash, "$")[5]
	// Recomputing with smaller memory yields a different key, so this must
	// NOT verify — proving params are actually honored.
	if err := VerifyPassword("pw", alt); err == nil {
		t.Fatal("verification must honor stored params (altered params should fail)")
	}
}

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestIssueAndVerifyToken(t *testing.T) {
	tok, exp, err := IssueToken(testSecret, "local:admin", "admin", true)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if time.Until(exp) < 11*time.Hour || time.Until(exp) > 13*time.Hour {
		t.Fatalf("unexpected expiry: %v", exp)
	}
	c, err := VerifyToken(testSecret, tok)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if c.Sub != "local:admin" || c.Username != "admin" || !c.Admin {
		t.Fatalf("unexpected claims: %+v", c)
	}
}

func TestVerifyTokenWrongSecret(t *testing.T) {
	tok, _, _ := IssueToken(testSecret, "local:admin", "admin", true)
	if _, err := VerifyToken([]byte("another-secret-another-secret"), tok); err == nil {
		t.Fatal("VerifyToken with wrong secret must fail")
	}
}

func TestLooksLocal(t *testing.T) {
	tok, _, _ := IssueToken(testSecret, "local:admin", "admin", true)
	if !LooksLocal(tok) {
		t.Fatal("own token must look local")
	}
	if LooksLocal("not-a-jwt") {
		t.Fatal("garbage must not look local")
	}
	if LooksLocal("ainsel_user-token") {
		t.Fatal("user token must not look local")
	}
}

func TestIssueTokenEmptySecret(t *testing.T) {
	if _, _, err := IssueToken(nil, "local:a", "a", false); err == nil {
		t.Fatal("IssueToken with empty secret must fail")
	}
}

func TestMiddleware(t *testing.T) {
	var seen *oidc.User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := oidc.FromContext(r.Context())
		seen = u
		w.WriteHeader(http.StatusOK)
	})
	mw := NewMiddleware(testSecret)

	t.Run("valid local token sets user", func(t *testing.T) {
		seen = nil
		tok, _, _ := IssueToken(testSecret, "local:alice", "alice", false)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(inner).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if seen == nil || seen.Sub != "local:alice" || seen.Username != "alice" {
			t.Fatalf("user not set correctly: %+v", seen)
		}
	})

	t.Run("tampered local token gets 401 and does not fall through", func(t *testing.T) {
		seen = nil
		tok, _, _ := IssueToken(testSecret, "local:alice", "alice", false)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok+"x")
		rec := httptest.NewRecorder()
		mw(inner).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if seen != nil {
			t.Fatal("inner handler must not run for invalid local token")
		}
	})

	t.Run("foreign token falls through", func(t *testing.T) {
		seen = nil
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer some.rs256.token")
		rec := httptest.NewRecorder()
		mw(inner).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || seen != nil {
			t.Fatalf("expected fall-through without user, got status=%d user=%v", rec.Code, seen)
		}
	})

	t.Run("no token falls through", func(t *testing.T) {
		seen = nil
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		mw(inner).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || seen != nil {
			t.Fatalf("expected fall-through, got status=%d user=%v", rec.Code, seen)
		}
	})
}
