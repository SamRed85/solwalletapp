# Sol

A match-prediction app on Solana **devnet**. Users sign in with their Phantom wallet, admins create
events of 7–13 games and enter results, and users place coupons (one pick per game: `1` home win,
`X` draw, `2` away win) paid in devnet SOL.

Stack: Go backend, MongoDB, Vue 3 + Vuetify 3 frontend (Vite).

> Devnet only. The backend refuses to start against a mainnet RPC URL. See
> [`.cursor/plans/master_plan.md`](.cursor/plans/master_plan.md) for the road to mainnet.

## Quick start

Prerequisites: Go 1.25+, Node.js 20.19+ or 22.12+, Docker Desktop, and the Phantom browser extension set to Devnet.

```bash
docker compose up -d              # MongoDB on localhost:27017

cd backend && go run .            # API on http://localhost:8080

cd frontend && npm install && npm run dev   # UI on http://localhost:3000
```

On Windows, `start-all.bat` runs all three. The frontend dev server proxies `/api` and `/health` to the backend.

Optional database UI: `docker compose --profile tools up -d`, then http://localhost:8081 (admin / admin123).

Test SOL: `solana airdrop 2 <wallet> --url devnet`, or https://faucet.solana.com.

## Configuration (backend environment variables)

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `MONGODB_URI` | `mongodb://localhost:27017` | MongoDB connection |
| `MONGODB_DB` | `sol` | Database name |
| `ADMIN_WALLETS` | `4tyf8a...cHdy` | Comma-separated admin wallet addresses |
| `TREASURY_WALLET` | `4tyf8a...cHdy` | Wallet that receives coupon payments |
| `COUPON_PRICE_LAMPORTS` | `1000000` (0.001 SOL) | Price per coupon |
| `SOLANA_RPC_URL` | `https://api.devnet.solana.com` | RPC used to verify payments |
| `SESSION_SECRET` | random per start | HMAC key for session tokens; set it to keep sessions across restarts |
| `ALLOWED_ORIGIN` | `http://localhost:3000` | CORS origin, only needed when not using the dev proxy |

Frontend: `VITE_API_URL` (default empty, uses the dev proxy) and `BACKEND_URL` for the proxy target.

## How it works

**Sign-in.** The frontend asks the backend for a one-time nonce, the wallet signs a message containing
the wallet address and nonce, and the backend verifies the ed25519 signature. It then issues a session
token (12 hours) that the frontend sends as `Authorization: Bearer <token>`.

**Events.** An event moves through `draft` (no games) → `open` (games set, entries allowed until the
start time) → `closed` (started) → `completed` (results set). Games and results can each be set once.

**Entries.** A user pays `price × number of coupons` with a normal SOL transfer to the treasury and
submits the transaction signature with the coupons. The backend fetches the transaction from the RPC
and accepts it only if it is confirmed, succeeded, is less than an hour old, and moved exactly that
amount from the user's wallet to the treasury. Each signature can be used once.

## API

| Method | Path | Access |
|---|---|---|
| GET | `/health` | public |
| GET | `/api/config` | public |
| POST | `/api/auth/nonce` | public |
| POST | `/api/auth/login` | public |
| GET | `/api/me` | signed in |
| GET | `/api/events`, `/api/events/{id}` | signed in |
| POST | `/api/events/{id}/entries` | signed in |
| GET | `/api/entries` | signed in (own entries) |
| POST | `/api/events` | admin |
| PUT | `/api/events/{id}/games` | admin |
| POST | `/api/events/{id}/results` | admin |
| GET | `/api/users` | admin |

## Development

```bash
cd backend && go test ./...
cd frontend && npm run lint && npm run build
```
