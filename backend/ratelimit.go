package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type rateWindow struct {
	start time.Time
	count int
}

type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
}

func (l *rateLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = make(map[string]rateWindow)
	}
	current := l.windows[key]
	if current.start.IsZero() || now.Sub(current.start) >= window {
		current = rateWindow{start: now}
	}
	if current.count >= limit {
		return false
	}
	current.count++
	l.windows[key] = current

	if len(l.windows) > 10000 {
		for k, value := range l.windows {
			if now.Sub(value.start) >= window {
				delete(l.windows, k)
			}
		}
	}
	return true
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (a *App) limitRequests(scope string, limit int, window time.Duration, key func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limiter.allow(scope+":"+key(r), limit, window, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "too many requests, try again shortly")
			return
		}
		next(w, r)
	}
}

func ipRateKey(r *http.Request) string {
	return remoteIP(r)
}

func walletRateKey(r *http.Request) string {
	return walletFrom(r)
}
