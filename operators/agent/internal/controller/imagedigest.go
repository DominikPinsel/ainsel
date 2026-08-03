package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// imageDigestAnnotation is stamped on the Deployment pod template.
	// When the digest behind a mutable tag changes (e.g. :dev is rebuilt),
	// the annotation value changes and Kubernetes performs a rolling restart.
	imageDigestAnnotation = "ainsel.dev/image-digest"

	// dockerHubRegistry is the default registry when no host is specified.
	dockerHubRegistry = "registry-1.docker.io"
	// dockerHubAuthURL is the token endpoint for Docker Hub.
	dockerHubAuthURL = "https://auth.docker.io/token"

	// digestResolveTimeout is the HTTP timeout for registry requests.
	digestResolveTimeout = 10 * time.Second

	// tokenSafetyMargin is subtracted from a cached token's lifetime so we
	// never present a token that is about to expire.
	tokenSafetyMargin = 10 * time.Second
)

// manifestAcceptTypes are the manifest media types we ask the registry for.
// Covering both Docker and OCI manifest/index types keeps resolution working
// across registries and multi-arch images.
var manifestAcceptTypes = strings.Join([]string{
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.image.index.v1+json",
}, ", ")

var (
	// httpClient is shared across digest resolutions so connections are
	// reused instead of rebuilding a client (and re-dialing) on every call.
	httpClient = &http.Client{Timeout: digestResolveTimeout}

	// tokenCache caches Docker Hub anonymous pull tokens per repository for
	// their validity window. With a 5-minute requeue across many agents this
	// avoids hammering the Docker Hub auth endpoint (and its rate limits).
	tokenCacheMu sync.Mutex
	tokenCache   = map[string]cachedToken{}
)

type cachedToken struct {
	token  string
	expiry time.Time
}

// resolveImageDigest queries the container registry for the manifest digest
// of the given image reference (e.g. "dpinsel/ainsel-pi:dev"). It returns
// the digest string (e.g. "sha256:abc123...") or an error.
//
// Digest-pinned references (repo@sha256:…) already carry their digest and are
// returned as-is without contacting the registry.
func resolveImageDigest(ctx context.Context, imageRef string) (string, error) {
	// A digest-pinned ref is immutable; its digest is already known.
	if _, digest, ok := splitDigest(imageRef); ok {
		return digest, nil
	}

	registry, repository, tag := parseImageRef(imageRef)

	// For Docker Hub, obtain a (cached) bearer token first.
	var token string
	if registry == dockerHubRegistry {
		var err error
		token, err = getDockerHubToken(ctx, repository)
		if err != nil {
			return "", fmt.Errorf("docker hub auth for %s: %w", repository, err)
		}
	}

	// Docker Hub is HTTPS-only. Every other registry is tried over HTTPS
	// first and falls back to HTTP on a connection failure, so plain-HTTP
	// internal registries (e.g. zot) keep working while HTTPS registries
	// such as GHCR/ECR are used securely by default.
	schemes := []string{"https", "http"}
	if isDockerHub(registry) {
		schemes = []string{"https"}
	}

	var lastErr error
	for _, scheme := range schemes {
		url := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, registry, repository, tag)

		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			return "", fmt.Errorf("creating manifest request: %w", err)
		}
		req.Header.Set("Accept", manifestAcceptTypes)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			// Transport failure (e.g. TLS handshake against an HTTP-only
			// registry): try the next scheme.
			lastErr = fmt.Errorf("fetching manifest for %s: %w", imageRef, err)
			continue
		}

		status := resp.StatusCode
		digest := resp.Header.Get("Docker-Content-Digest")
		_ = resp.Body.Close()

		if status != http.StatusOK {
			// A definitive registry answer (auth, not-found, etc.) — no
			// point falling back to HTTP for this.
			return "", fmt.Errorf("registry returned %d for %s", status, imageRef)
		}
		if digest == "" {
			return "", fmt.Errorf("no Docker-Content-Digest header for %s", imageRef)
		}
		return digest, nil
	}
	return "", lastErr
}

// splitDigest separates a digest-pinned reference (repo@sha256:…) into the
// base reference and the digest. ok is false when ref is not digest-pinned.
func splitDigest(ref string) (base, digest string, ok bool) {
	if i := strings.Index(ref, "@"); i >= 0 {
		return ref[:i], ref[i+1:], true
	}
	return ref, "", false
}

// isDockerHub reports whether the registry host is Docker Hub.
func isDockerHub(registry string) bool {
	return registry == dockerHubRegistry || strings.Contains(registry, "docker.io")
}

// parseImageRef splits an image reference into registry, repository, and tag.
// Digest-pinned references are not expected here; callers should strip the
// digest first via splitDigest. Examples:
//
//	"dpinsel/ainsel-pi:dev"                -> ("registry-1.docker.io", "dpinsel/ainsel-pi", "dev")
//	"zot.platform.svc:5000/ainsel/pi:main" -> ("zot.platform.svc:5000", "ainsel/pi", "main")
//	"zot.platform.svc:5000/ainsel/pi"      -> ("zot.platform.svc:5000", "ainsel/pi", "latest")
//	"localhost:5000/foo:bar"               -> ("localhost:5000", "foo", "bar")
//	"nginx"                                -> ("registry-1.docker.io", "library/nginx", "latest")
func parseImageRef(ref string) (registry, repository, tag string) {
	registry = dockerHubRegistry
	tag = "latest"

	// Split off the tag: the last colon whose remainder has no slash. A
	// remainder containing a slash means the colon is a registry port
	// (e.g. "zot:5000/foo"), not a tag separator.
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		candidate := ref[idx+1:]
		if !strings.Contains(candidate, "/") {
			tag = candidate
			ref = ref[:idx]
		}
	}

	// The first path component is a registry hostname when it contains a
	// dot or a colon (port), or is the "localhost" alias.
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		registry = parts[0]
		repository = parts[1]
	} else {
		repository = ref
	}

	// Docker Hub single-segment names (e.g. "nginx") live in the "library"
	// namespace; the registry API needs the fully-qualified repository path.
	if registry == dockerHubRegistry && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return registry, repository, tag
}

// getDockerHubToken obtains an anonymous pull token for the given repository,
// reusing a cached token while it is still valid.
func getDockerHubToken(ctx context.Context, repository string) (string, error) {
	tokenCacheMu.Lock()
	if ct, ok := tokenCache[repository]; ok && time.Now().Before(ct.expiry) {
		tokenCacheMu.Unlock()
		return ct.token, nil
	}
	tokenCacheMu.Unlock()

	url := fmt.Sprintf("%s?service=registry.docker.io&scope=repository:%s:pull", dockerHubAuthURL, repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth returned %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding auth response: %w", err)
	}
	if tokenResp.Token == "" {
		return "", fmt.Errorf("empty token in auth response")
	}

	// Cache for the token's validity window (minus a safety margin). When
	// the registry omits expires_in, assume a conservative 60s lifetime.
	ttl := time.Duration(tokenResp.ExpiresIn) * time.Second
	if ttl <= tokenSafetyMargin {
		ttl = 60 * time.Second
	}
	tokenCacheMu.Lock()
	tokenCache[repository] = cachedToken{token: tokenResp.Token, expiry: time.Now().Add(ttl - tokenSafetyMargin)}
	tokenCacheMu.Unlock()

	return tokenResp.Token, nil
}
