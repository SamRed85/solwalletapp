package main

import (
	"context"
	"crypto/ed25519"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestUserProfileRules(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skip(err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Skip(err)
	}
	db := client.Database("sol_profile_test")
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	playerPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	adminPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	player := base58Encode(playerPub)
	admin := base58Encode(adminPub)
	app := &App{
		cfg:     Config{SessionSecret: []byte("profile-test"), AdminWallets: map[string]bool{admin: true}},
		users:   db.Collection("users"),
		events:  db.Collection("events"),
		entries: db.Collection("entries"),
	}
	now := time.Now()
	if _, err := app.users.InsertOne(ctx, User{WalletAddress: player, CreatedAt: now, LastLoginAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.users.InsertOne(ctx, User{WalletAddress: admin, DisplayName: "Admin", CreatedAt: now, LastLoginAt: now}); err != nil {
		t.Fatal(err)
	}
	eventID := bson.NewObjectID()
	if _, err := app.events.InsertOne(ctx, bson.M{"_id": eventID, "title": "Open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.entries.InsertOne(ctx, Entry{
		WalletAddress: player, EventID: eventID, Combinations: 4, Signature: "sig", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	mux := app.routes()
	playerToken := app.signToken(player, now.Add(time.Hour))
	adminToken := app.signToken(admin, now.Add(time.Hour))

	if rec := call(mux, http.MethodPut, "/api/me", playerToken, `{"displayName":"A"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short name: %d %s", rec.Code, rec.Body)
	}
	if rec := call(mux, http.MethodPut, "/api/me", playerToken, `{"displayName":"Ada"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"displayName":"Ada"`) {
		t.Fatalf("set name: %d %s", rec.Code, rec.Body)
	}
	if rec := call(mux, http.MethodGet, "/api/users", adminToken, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"activeCoupons":4`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := call(mux, http.MethodPut, "/api/users/"+player, adminToken, `{"displayName":"Nope"}`); rec.Code != http.StatusConflict {
		t.Fatalf("rename while active: %d %s", rec.Code, rec.Body)
	}
	if rec := call(mux, http.MethodDelete, "/api/users/"+player, adminToken, ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete while active: %d %s", rec.Code, rec.Body)
	}
	if rec := call(mux, http.MethodDelete, "/api/users/"+admin, adminToken, ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete self: %d %s", rec.Code, rec.Body)
	}

	if _, err := app.events.UpdateOne(ctx, bson.M{"_id": eventID}, bson.M{"$set": bson.M{"resultsSetAt": now}}); err != nil {
		t.Fatal(err)
	}
	if rec := call(mux, http.MethodPut, "/api/users/"+player, adminToken, `{"displayName":"Renamed"}`); rec.Code != http.StatusOK {
		t.Fatalf("rename after results: %d %s", rec.Code, rec.Body)
	}
	if rec := call(mux, http.MethodDelete, "/api/users/"+player, adminToken, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete after results: %d %s", rec.Code, rec.Body)
	}
	if n, err := app.entries.CountDocuments(ctx, bson.M{"walletAddress": player}); err != nil || n != 1 {
		t.Fatalf("finished coupon should remain, count %d err %v", n, err)
	}
}

func call(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
