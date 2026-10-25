package main

import (
	"crypto/rand"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                string
	MongoURI            string
	MongoDB             string
	AllowedOrigin       string
	SessionSecret       []byte
	AdminWallets        map[string]bool
	TreasuryWallet      string
	CouponPriceLamports uint64
	SolanaRPCURL        string
	ProgramID           string
	ResultCosigner      string
	APIFootballKey      string
}

const defaultWallet = "4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy"

func loadConfig() Config {
	loadDotEnv()
	cfg := Config{
		Port:           env("PORT", "8080"),
		MongoURI:       env("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDB:        env("MONGODB_DB", "sol"),
		AllowedOrigin:  env("ALLOWED_ORIGIN", "http://localhost:3000"),
		TreasuryWallet: env("TREASURY_WALLET", defaultWallet),
		SolanaRPCURL:   env("SOLANA_RPC_URL", "https://api.devnet.solana.com"),
		ProgramID:      env("PROGRAM_ID", "AaXABVJE79W6za8ntvto8wvtP6Kqgs5kQgHCS2uoHoaX"),
		ResultCosigner: env("RESULT_COSIGNER", "4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH"),
		APIFootballKey: env("API_FOOTBALL_KEY", ""),
		AdminWallets:   map[string]bool{},
	}
	if strings.Contains(env("ADMIN_WALLETS", defaultWallet), cfg.ResultCosigner) {
		log.Fatal("RESULT_COSIGNER must not be an admin wallet: an admin would then hold both result keys")
	}

	for _, w := range strings.Split(env("ADMIN_WALLETS", defaultWallet), ",") {
		if w = strings.TrimSpace(w); w != "" {
			cfg.AdminWallets[w] = true
		}
	}

	price, err := strconv.ParseUint(env("COUPON_PRICE_LAMPORTS", "1000000"), 10, 64)
	if err != nil || price == 0 {
		log.Fatal("COUPON_PRICE_LAMPORTS must be a positive integer")
	}
	cfg.CouponPriceLamports = price

	if strings.Contains(strings.ToLower(cfg.SolanaRPCURL), "mainnet") && os.Getenv("ALLOW_MAINNET") != "true" {
		log.Fatal("SOLANA_RPC_URL points at mainnet; this app is devnet-only until it has been audited")
	}

	if secret := os.Getenv("SESSION_SECRET"); secret != "" {
		cfg.SessionSecret = []byte(secret)
	} else {
		cfg.SessionSecret = make([]byte, 32)
		if _, err := rand.Read(cfg.SessionSecret); err != nil {
			log.Fatal(err)
		}
		log.Println("SESSION_SECRET not set: using a random secret, sessions end when the server restarts")
	}

	return cfg
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv reads SOLANA_RPC_URL and the other settings from a gitignored .env file.
// A variable already set in the environment is left alone. The file is looked up in the
// working directory and its parent, so `go run .` from backend still finds the repo root.
func loadDotEnv() {
	for _, path := range []string{".env", filepath.Join("..", ".env")} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		applyDotEnv(data)
		return
	}
}

func applyDotEnv(data []byte) {
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func redactRPC(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery == "" {
		return raw
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
