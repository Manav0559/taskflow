package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// bucket tracks one client's tokens plus when it was last topped up, so refill can be
// computed lazily from elapsed wall-clock time on each request rather than requiring a
// background goroutine/ticker per client (which wouldn't scale with client cardinality).
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// rateLimiter is a hand-rolled per-client-IP token bucket. It is deliberately a
// per-process, in-memory limiter rather than a Redis-backed shared one: at this API's
// current scale, rate limiting exists to blunt abusive submission bursts against a
// single instance, not to enforce one hard quota across a fleet, so each replica
// policing its own traffic independently is an acceptable trade-off. A shared/
// distributed limiter would be the natural next step if taskflow's API scaled out to
// multiple replicas that needed one true combined limit per client.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   int
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	return &rateLimiter{
		buckets: make(map[string]*bucket),
		rps:     rps,
		burst:   burst,
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.burst), lastRefill: now}
		rl.buckets[key] = b
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.lastRefill = now

	b.tokens += elapsed * rl.rps
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
