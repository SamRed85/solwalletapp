package main

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type rateWindow struct {
	start time.Time
	count int
}

type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
	coll    *mongo.Collection
}

func (l *rateLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	if l.coll != nil {
		return l.allowShared(key, limit, window, now)
	}
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

// allowShared counts in MongoDB so every backend process shares one window.
func (l *rateLimiter) allowShared(key string, limit int, window time.Duration, now time.Time) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	open := now.Add(-window)
	res, err := l.coll.UpdateOne(ctx, bson.M{"_id": key, "start": bson.M{"$gt": open}, "count": bson.M{"$lt": limit}}, bson.M{"$inc": bson.M{"count": 1}})
	if err != nil {
		return false
	}
	if res.MatchedCount == 1 {
		return true
	}
	res, err = l.coll.UpdateOne(ctx, bson.M{"_id": key, "start": bson.M{"$lte": open}}, bson.M{"$set": bson.M{"start": now, "count": 1}})
	if err != nil {
		return false
	}
	if res.MatchedCount == 1 {
		return true
	}
	_, err = l.coll.InsertOne(ctx, bson.M{"_id": key, "start": now, "count": 1})
	if err == nil {
		return true
	}
	if !mongo.IsDuplicateKeyError(err) {
		return false
	}
	res, err = l.coll.UpdateOne(ctx, bson.M{"_id": key, "start": bson.M{"$gt": open}, "count": bson.M{"$lt": limit}}, bson.M{"$inc": bson.M{"count": 1}})
	return err == nil && res.MatchedCount == 1
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
