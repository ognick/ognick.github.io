package nutrition

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newIPLimiter() *ipLimiter {
	return &ipLimiter{limiters: make(map[string]*rate.Limiter)}
}

func (l *ipLimiter) get(key string, r rate.Limit, burst int) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lim, ok := l.limiters[key]; ok {
		return lim
	}
	lim := rate.NewLimiter(r, burst)
	l.limiters[key] = lim
	return lim
}

func (h *Handler) DailyRateLimit(next http.Handler) http.Handler {
	dailyLimiter := newIPLimiter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		lim := dailyLimiter.get(ip, 10.0/60.0, 3)
		if !lim.Allow() {
			http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) WeeklyRateLimit(next http.Handler) http.Handler {
	weeklyLimiter := newIPLimiter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		lim := weeklyLimiter.get(ip, 2.0/60.0, 1)
		if !lim.Allow() {
			http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
