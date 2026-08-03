package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

func hubGet(ctx context.Context, client *http.Client, baseURL, path string) ([]byte, error) {
	return hubRequest(ctx, client, http.MethodGet, baseURL, path, nil)
}

func hubDelete(ctx context.Context, client *http.Client, baseURL, path string) ([]byte, error) {
	return hubRequest(ctx, client, http.MethodDelete, baseURL, path, nil)
}

func hubPost(ctx context.Context, client *http.Client, baseURL, path string, body io.Reader) ([]byte, error) {
	return hubRequestWithCodes(ctx, client, http.MethodPost, baseURL, path, body, http.StatusOK, http.StatusCreated)
}

func hubPut(ctx context.Context, client *http.Client, baseURL, path string, body io.Reader) ([]byte, error) {
	return hubRequestWithCodes(ctx, client, http.MethodPut, baseURL, path, body, http.StatusOK)
}

func hubRequest(ctx context.Context, client *http.Client, method, baseURL, path string, body io.Reader) ([]byte, error) {
	return hubRequestWithCodes(ctx, client, method, baseURL, path, body, http.StatusOK)
}

func hubRequestWithCodes(ctx context.Context, client *http.Client, method, baseURL, path string, body io.Reader, validCodes ...int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if tok, ok := oidc.TokenFromContext(ctx); ok {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// DELETE is idempotent; a 204 No Content is success.
	if method == http.MethodDelete && resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	ok := false
	for _, code := range validCodes {
		if resp.StatusCode == code {
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("hub returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
