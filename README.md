# Sol

A match-prediction app on Solana **devnet**. Users sign in with their Phantom wallet, admins create
events (13 games by default, 7–13 for special events) and enter results, and users place coupons
(one pick per game: `1` home win, `X` draw, `2` away win) paid in devnet SOL with Phantom.

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
| `COUPON_PRICE_LAMPORTS` | `1000000` (0.001 SOL) | Default price per coupon (stored on each event when it is created) |
| `SOLANA_RPC_URL` | `https://api.devnet.solana.com` | RPC used to verify payments and read vault accounts |
| `PROGRAM_ID` | `H4x1HK...G97tr` | Predictions program that owns vault events |
| `RESULT_COSIGNER` | `4gGayW...FEHH` | Required second result signer of every vault; must not be an admin wallet |
| `SESSION_SECRET` | random per start | HMAC key for session tokens; set it to keep sessions across restarts |
| `ALLOWED_ORIGIN` | `http://localhost:3000` | CORS origin, only needed when not using the dev proxy |

Frontend: `VITE_API_URL` (default empty, uses the dev proxy) and `BACKEND_URL` for the proxy target.

## How it works

**Sign-in.** The frontend asks the backend for a one-time nonce, the wallet signs a message containing
the wallet address and nonce, and the backend verifies the ed25519 signature. It then issues a session
token (12 hours) that the frontend sends as `Authorization: Bearer <token>`. Nonce and login attempts
are rate-limited per source IP; entry and payment-recovery attempts are rate-limited per wallet.

**Events.** An event moves through `draft` (no games) → `open` (games set, entries allowed until the
start time) → `closed` (started) → `completed` (results set). Games (each with its own kickoff) and
results can each be set once. Standard events have 13 games and the default price and prize tiers;
special events set their own game count, price and tiers.

**Entries.** On the event page the user fills one coupon and can select more than one outcome
per game, up to 44,000 combinations. Every combination is a paid row: selecting 1 and X on one
game, and one outcome on the rest, makes two rows. The browser sends the selections, and the server
counts the rows without storing each one. The frontend builds a System Program transfer of
`price × combinations` to the treasury plus a memo containing the event-and-coupon hash. A devnet
blockhash makes a wallet left on mainnet reject the transaction instead of spending real SOL. The
backend accepts only a finalized, successful transaction less than an hour old that has the expected
memo and moved exactly that amount from the user's wallet to the treasury. Each signature can be used
once. Until the entry is accepted the payment is kept in the browser. My Coupons offers a retry or a
verified refund request; a refund is recorded only after its treasury transaction is finalized and
verified.

**Payouts.** When the admin saves the results the event is settled once and the settlement is
stored. The house keeps 5% of ticket sales. The rest is split into tiers (standard: 50% for 13
correct, 25% for 12, 15% for 11, 5% for 10), and each tier is shared equally by its coupons. Rollover
from earlier events is added to the top tier. A tier without winners rolls over to the top tier of the
next event that has not started (or the next event created, if none exists). Integer leftovers go to
the house. Winnings are shown in My Coupons; the admin sees a payout list per wallet and pays
manually from the treasury (the server holds no keys).

**Cancelled games.** The admin enters `C` as the result. From 1 hour after the game's kickoff, its
outcome is drawn: the seed is `sha256("sol-draw:<eventId>:<gameIndex>:<blockhash>")`, using the
first finalized devnet block at or after that time. The first 8 bytes (big endian) modulo the total
number of picks select `1`, `X` or `2` in proportion to how many coupons picked each. The block and
weights are stored with the game so anyone can recompute the draw.

**Vault events.** An admin can move an open event without entries to an on-chain vault (at most 16
unresolved vaults per admin wallet). From then on coupons are paid to the program, never the treasury,
and the rules differ, see [`docs/rules-spec.md`](docs/rules-spec.md): picks stay hidden until entries
close and must be revealed before the end time, or the coupon cannot win. The player signs only the
entry. The picks are time-lock encrypted until entries close, and the backend publishes them from the
gitignored `keys/reveal-keeper.json`, so the player does not need to be online or sign the reveal.
My Coupons can arm that publish later, or reveal immediately once the window is open. A void game
(`C`) counts every pick as correct; the admin and the configured cosigner each approve the same results
(or a cancellation) in their own transaction, after which anyone runs the batched count and winners
claim from the vault. A cancellation takes effect only after both have approved; players then refund
from My Coupons or the event page. When the cosigner is the council key on disk, it approves with
`node scripts/cosign-event.mjs <event> cancel` or `... resolve <results>`.
Without approved results by 7 days after the end time, anyone can cancel the event and every coupon is
refunded. Each coupon carries an encrypted backup of its picks, so My Coupons can rebuild it on another
device after the wallet signs a fixed message.

## API

| Method | Path | Access |
|---|---|---|
| GET | `/health` | public |
| GET | `/api/config` | public |
| POST | `/api/auth/nonce` | public |
| POST | `/api/auth/login` | public |
| GET | `/api/me` | signed in |
| PUT | `/api/me` | signed in (set profile name) |
| GET | `/api/events`, `/api/events/{id}` | signed in |
| POST | `/api/events/{id}/entries` | signed in |
| POST | `/api/events/{id}/payment-recoveries` | signed in (own stranded payment) |
| POST | `/api/events/{id}/chain/sync` | signed in (copies the vault outcome from chain) |
| POST | `/api/reveals` | signed in (stores the owner's time-locked reveal; the server cannot read it early) |
| GET | `/api/entries` | signed in (own entries) |
| POST | `/api/events` | admin |
| DELETE | `/api/events/{id}` | admin (only before a vault, coupons, or results) |
| PUT | `/api/events/{id}/games` | admin |
| PUT | `/api/events/{id}/chain` | admin (attaches a vault after checking it on chain) |
| POST | `/api/events/{id}/results` | admin (settles a treasury event) |
| GET | `/api/events/{id}/payouts` | admin |
| GET | `/api/payment-recoveries` | admin |
| PUT | `/api/payment-recoveries/{signature}` | admin (records verified refund) |
| GET | `/api/users` | admin |
| PUT | `/api/users/{wallet}` | admin (rename, only with no coupons still in play) |
| DELETE | `/api/users/{wallet}` | admin (remove, only with no coupons still in play) |

## Development

```bash
cd backend && go test ./...
cd frontend && npm run lint && npm run build

# Optional: check the cancelled-game block search against real devnet
SOLANA_LIVE_RPC=https://api.devnet.solana.com go test -run TestFindDrawBlockLive -v
```

Before Phase 4, complete [`docs/devnet-acceptance.md`](docs/devnet-acceptance.md). A recorded
payment can be checked independently with `node scripts/verify-devnet-payment.mjs`; the command
recomputes its coupon memo and requires a successful finalized devnet transfer.
