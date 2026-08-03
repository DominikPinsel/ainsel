package api

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiter is a per-IP token bucket rate limiter. Each unique client IP
// gets its own limiter with the configured rate and burst. Stale entries are
// evicted after idleTimeout to prevent unbounded memory growth.
type rateLimiter struct {
	mu          sync.Mutex
	clients     map[string]*clientLimiter
	rate        rate.Limit
	burst       int
	idleTimeout time.Duration
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newRateLimiter creates a per-IP rate limiter. r is the sustained request
// rate (requests per second) and burst is the maximum burst size.
func newRateLimiter(r float64, burst int) *rateLimiter {
	rl := &rateLimiter{
		clients:     make(map[string]*clientLimiter),
		rate:        rate.Limit(r),
		burst:       burst,
		idleTimeout: 5 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

// getLimiter returns the rate limiter for the given IP, creating one if needed.
func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	c, ok := rl.clients[ip]
	if !ok {
		c = &clientLimiter{limiter: rate.NewLimiter(rl.rate, rl.burst)}
		rl.clients[ip] = c
	}
	c.lastSeen = time.Now()
	return c.limiter
}

// cleanupLoop periodically removes stale entries.
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.idleTimeout)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, c := range rl.clients {
			if now.Sub(c.lastSeen) > rl.idleTimeout {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware returns an HTTP middleware that applies per-IP rate
// limiting. Requests exceeding the limit receive 429 Too Many Requests.
//
// Default: 30 requests/second sustained, burst of 60.
func RateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	rl := newRateLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !rl.getLimiter(ip).Allow() {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the client IP from the request, preferring
// X-Forwarded-For (set by reverse proxies) over RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (client) IP in the chain.
		if i := len(xff); i > 0 {
			for j := 0; j < len(xff); j++ {
				if xff[j] == ',' {
					return xff[:j]
				}
			}
			return xff
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
