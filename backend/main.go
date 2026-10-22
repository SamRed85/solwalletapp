package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type App struct {
	cfg    Config
	db     *mongo.Database
	nonces *nonceStore

	users    *mongo.Collection
	events   *mongo.Collection
	entries  *mongo.Collection
	rollover *mongo.Collection

	verifyPayment func(ctx context.Context, signature, payer, treasury string, lamports uint64) error
	findDrawBlock func(ctx context.Context, t time.Time) (*blockRef, error)
}

func main() {
	cfg := loadConfig()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = client.Ping(ctx, nil)
		cancel()
	}
	if err != nil {
		log.Fatalf("failed to connect to MongoDB at %s: %v", cfg.MongoURI, err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database(cfg.MongoDB)
	rpc := newSolanaRPC(cfg.SolanaRPCURL)
	app := &App{
		cfg:           cfg,
		db:            db,
		nonces:        newNonceStore(),
		users:         db.Collection("users"),
		events:        db.Collection("events"),
		entries:       db.Collection("entries"),
		rollover:      db.Collection("rollover"),
		verifyPayment: rpc.verifyPayment,
		findDrawBlock: rpc.findDrawBlock,
	}
	if err := app.ensureIndexes(); err != nil {
		log.Fatalf("failed to create indexes: %v", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.cors(app.routes()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      150 * time.Second,
	}
	log.Printf("Sol backend listening on :%s (db %q, RPC %s, treasury %s)",
		cfg.Port, cfg.MongoDB, cfg.SolanaRPCURL, cfg.TreasuryWallet)
	log.Fatal(srv.ListenAndServe())
}

func (a *App) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /api/config", a.handleConfig)
	mux.HandleFunc("POST /api/auth/nonce", a.handleNonce)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)

	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("PUT /api/me", a.requireAuth(a.handleUpdateMe))
	mux.HandleFunc("GET /api/events", a.requireAuth(a.handleListEvents))
	mux.HandleFunc("GET /api/events/{id}", a.requireAuth(a.handleGetEvent))
	mux.HandleFunc("POST /api/events/{id}/entries", a.requireAuth(a.handleCreateEntry))
	mux.HandleFunc("GET /api/entries", a.requireAuth(a.handleMyEntries))

	mux.HandleFunc("GET /api/users", a.requireAdmin(a.handleListUsers))
	mux.HandleFunc("PUT /api/users/{wallet}", a.requireAdmin(a.handleAdminUpdateUser))
	mux.HandleFunc("DELETE /api/users/{wallet}", a.requireAdmin(a.handleAdminDeleteUser))
	mux.HandleFunc("POST /api/events", a.requireAdmin(a.handleCreateEvent))
	mux.HandleFunc("PUT /api/events/{id}/games", a.requireAdmin(a.handleSetGames))
	mux.HandleFunc("POST /api/events/{id}/results", a.requireAdmin(a.handleSetResults))
	mux.HandleFunc("GET /api/events/{id}/payouts", a.requireAdmin(a.handlePayouts))

	return mux
}

func (a *App) ensureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	unique := options.Index().SetUnique(true)

	if _, err := a.users.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.M{"walletAddress": 1}, Options: unique}); err != nil {
		return err
	}
	if _, err := a.events.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.M{"startTime": -1}}); err != nil {
		return err
	}
	_, err := a.entries.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.M{"signature": 1}, Options: unique},
		{Keys: bson.D{{Key: "walletAddress", Value: 1}, {Key: "createdAt", Value: -1}}},
		{Keys: bson.M{"eventId": 1}},
	})
	return err
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Client().Ping(ctx, nil); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "message": "database unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"treasuryWallet":      a.cfg.TreasuryWallet,
		"couponPriceLamports": a.cfg.CouponPriceLamports,
		"solanaRpcUrl":        a.cfg.SolanaRPCURL,
		"minGames":            minGames,
		"maxGames":            maxGames,
		"maxCouponsPerEntry":  maxCombinations,
		"maxCombinations":     maxCombinations,
		"defaultGameCount":    defaultGameCount,
		"defaultTiers":        defaultTiers(defaultGameCount),
		"houseCutPercent":     houseCutPercent,
		"tierPercentTotal":    tierPercentTotal,
	})
}
