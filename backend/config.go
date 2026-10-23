package main

import (
	"crypto/rand"
	"log"
	"os"
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
}

const defaultWallet = "4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy"

func loadConfig() Config {
	cfg := Config{
		Port:           env("PORT", "8080"),
		MongoURI:       env("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDB:        env("MONGODB_DB", "sol"),
		AllowedOrigin:  env("ALLOWED_ORIGIN", "http://localhost:3000"),
		TreasuryWallet: env("TREASURY_WALLET", defaultWallet),
		SolanaRPCURL:   env("SOLANA_RPC_URL", "https://api.devnet.solana.com"),
		ProgramID:      env("PROGRAM_ID", "H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr"),
		ResultCosigner: env("RESULT_COSIGNER", "4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH"),
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
