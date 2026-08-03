package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/usertokens"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

type tokenResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

type createTokenResponse struct {
	tokenResponse
	Token string `json:"token"`
}

func toTokenResponse(t usertokens.Token) tokenResponse {
	return tokenResponse{
		ID:         t.ID,
		Name:       t.Name,
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
		CreatedAt:  t.CreatedAt,
		RevokedAt:  t.RevokedAt,
	}
}

// handleUserTokens serves GET (list) and POST (create) on /api/v1/user-tokens.
func (s *Server) handleUserTokens(w http.ResponseWriter, r *http.Request) {
	if s.userTokens == nil {
		writeError(w, http.StatusServiceUnavailable, "user tokens not configured")
		return
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		toks, err := s.userTokens.List(r.Context(), u.Sub)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if toks == nil {
			toks = []usertokens.Token{}
		}
		out := make([]tokenResponse, len(toks))
		for i, t := range toks {
			out[i] = toTokenResponse(t)
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var body struct {
			Name          string `json:"name"`
			ExpiresInDays int    `json:"expiresInDays"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if body.ExpiresInDays != 30 && body.ExpiresInDays != 60 && body.ExpiresInDays != 90 {
			writeError(w, http.StatusBadRequest, "expiresInDays must be 30, 60, or 90")
			return
		}
		expiresAt := time.Now().Add(time.Duration(body.ExpiresInDays) * 24 * time.Hour)
		tok, plaintext, err := s.userTokens.Create(r.Context(), u.Sub, body.Name, expiresAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, createTokenResponse{
			tokenResponse: toTokenResponse(tok),
			Token:         plaintext,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleUserTokenDelete serves DELETE on /api/v1/user-tokens/{id}.
func (s *Server) handleUserTokenDelete(w http.ResponseWriter, r *http.Request) {
	if s.userTokens == nil {
		writeError(w, http.StatusServiceUnavailable, "user tokens not configured")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := extractName(r.URL.Path, "/api/v1/user-tokens/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "token id required")
		return
	}
	if err := s.userTokens.Revoke(r.Context(), id, u.Sub); err != nil {
		if errors.Is(err, usertokens.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUserTokenValidate is called by the MCP service to resolve a plaintext
// user token to a user identity. Protected by X-Internal-Token shared secret;
// registered outside /api/v1/ so it bypasses the OIDC middleware.
func (s *Server) handleUserTokenValidate(w http.ResponseWriter, r *http.Request) {
	if s.userTokens == nil {
		writeError(w, http.StatusServiceUnavailable, "user tokens not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.internalValidateSecret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Token")), []byte(s.internalValidateSecret)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	tok, err := s.userTokens.Validate(r.Context(), body.Token)
	if err != nil {
		if errors.Is(err, usertokens.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	user, err := s.authzStore.GetUser(r.Context(), tok.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"userId":   tok.UserID,
		"username": user.Username,
	})
}
