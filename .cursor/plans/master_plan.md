---
name: Master Plan - Product First, Then Strict Safety
overview: Roadmap from the current MongoDB prototype to an audited, trust-minimized Solana prediction app. Build and validate the product on devnet first, then add safety in layers, then optimize cost (see the ultra-low cost plan) and launch on mainnet after an external audit.
todos:
  - id: phase0_consolidate
    content: "Phase 0: Fresh start from C:\\solwalletapp (C:\\Sol dropped): review, cleanup, Vue 3 + Vuetify 3"
    status: completed
  - id: phase1_product_loop
    content: "Phase 1: Finish the full user loop on devnet (events, coupons, payment, results, payouts, My Coupons)"
    status: in_progress
    dependencies:
      - phase0_consolidate
  - id: phase1_product_decisions
    content: "Phase 1: Decide payout economics, coupon price, deadlines, cancellation rules, fees"
    status: in_progress
    dependencies:
      - phase0_consolidate
  - id: phase1_session_auth
    content: "Phase 1: Replace trust in the X-Wallet-Address header with a session token issued after signed login"
    status: completed
    dependencies:
      - phase0_consolidate
  - id: phase2_spec
    content: "Phase 2: Write the rules spec (coupon format, lifecycle, deadline, payout formula, refunds, roles)"
    status: pending
    dependencies:
      - phase1_product_decisions
  - id: phase3_payment_verification
    content: "Phase 3: Real RPC payment verification (finalized, sender, recipient, amount, memo, no reuse)"
    status: in_progress
    dependencies:
      - phase2_spec
  - id: phase4_program_v1
    content: "Phase 4: Anchor program v1 with program-owned vault, deadline enforcement, claim, refund, no admin withdraw"
    status: pending
    dependencies:
      - phase3_payment_verification
  - id: phase5_trust_minimization
    content: "Phase 5: Oracle/multisig outcomes, upgrade multisig + timelock, limited pause, solvency, verifiable builds"
    status: pending
    dependencies:
      - phase4_program_v1
  - id: phase6_scale_cost
    content: "Phase 6: Apply the ultra-low cost plan (logs + indexer, Merkle claims with challenge window, batching)"
    status: pending
    dependencies:
      - phase5_trust_minimization
  - id: phase7_audit_mainnet
    content: "Phase 7: Tests, fuzzing, external audit, bug bounty, capped mainnet launch"
    status: pending
    dependencies:
      - phase6_scale_cost
---

# Master Plan: Product First, Then Strict Safety

This plan sits next to the
[ultra-low cost plan](ultra-low_cost_solana_architecture_with_state_compression_2d66f835.plan.md).
That plan covers how to make predictions cheap on-chain. This one covers the order of work:
get the product right on test money first, then make it safe, then make it cheap, then launch.

## Ground Rules (apply from day one)

1. **Devnet only until Phase 7.** No real SOL is accepted before the audit. This is what
   makes it acceptable to build the product before the safety work.
2. **MongoDB never decides money.** It may store and display data. Balances, payments and
   payouts move to the chain over time; the database becomes a cache, not the source of truth.
3. **Write every rule down when you decide it.** These notes become the program spec
   (Phase 2) and the document an auditor reviews (Phase 7).

## Current State (after the Phase 0 cleanup, Oct 2026)

- `C:\solwalletapp` is the only codebase. `C:\Sol` and its uncommitted work were dropped.
- Backend (Go 1.25, MongoDB driver v2) split into `auth.go`, `events.go`, `entries.go`,
  `solana.go`, `users.go`, `config.go`, with unit tests in `app_test.go`.
- Sign-in verifies the wallet's ed25519 signature over a one-time server nonce and issues a
  12-hour HMAC session token. The `X-Wallet-Address` header is no longer trusted.
- Admins come from the `ADMIN_WALLETS` env var. Admin endpoints are enforced on the server.
- Events: `draft -> open -> closed -> completed`, derived from games, start time and results.
  Games and results can each be set once. Games and picks use `1`/`X`/`2`.
- Entries: `POST /api/events/{id}/entries` with coupons plus a payment signature. The backend
  checks the transaction on devnet (confirmed, succeeded, under 1 hour old, exact amount from
  the user to `TREASURY_WALLET`) and allows each signature once.
- Frontend: Vue 3, Vuetify 3, Vite. Home (sign-in, event list), Event (prize pool, coupon
  builder, Phantom payment, results), My Coupons, Admin (events, games, results, payout list), Users.
- Payouts (Phase 1): tiered shared pool with a 5% house cut, rollover and a weighted lottery for
  cancelled games, settled once when results are saved (`payouts.go`).
- Still custodial: money goes to a single treasury wallet. No Solana program, vault or indexer.

---

## Phase 0: Fresh Start and Cleanup (done)

- [x] Use `C:\solwalletapp` as the only codebase; ignore `C:\Sol`.
- [x] Fix `.gitignore` (it ignored every `public` folder, so `frontend/public/index.html`
      was never committed and a fresh clone could not build).
- [x] Remove router entries for views that did not exist, demo mode with fake signatures,
      the unused `WalletAuth` component, outdated docs and scripts, and the bogus
      `backend/package.json`.
- [x] Real signature verification, session tokens, admin-only user list.
- [x] Event and coupon validation on the server; set-once games and results.
- [x] Replace the accept-anything payment stub with devnet RPC verification.
- [x] Vue 2 + Vuetify 2 + Vue CLI -> Vue 3 + Vuetify 3 + Vite; drop `@solana/web3.js`
      (only the balance was used, and it pulled in vulnerable dependencies).
- [x] One database (`sol`), indexes created by the backend, MongoDB bound to localhost.

---

## Known Caveats After Phase 0

Each caveat names the phase that resolves it.

### Product gaps (Phase 1)

- **Coupon UI, payouts and My Coupons exist** (see Phase 1). Payouts are shown but paid
  manually, and nothing records whether a payout has been sent.
- **No way to correct admin mistakes.** Games and results are set once. A wrong result can only be
  fixed in the database. Cancelled games are handled by the lottery, but there is no cancel-event flow.
- **Stranded payments.** If a user pays but the entry is rejected (event closed during
  verification, wrong coupon count, database error), the money stays in the treasury. The
  browser keeps the payment and My Coupons offers a retry for up to 1 hour. After that,
  the only path is a manual refund by the admin.
- **Settlement is not atomic.** The results and settlement are saved in one update, and the rollover
  is then added to the target event in a second update. If the server fails between the two, the
  rollover is lost (an error is logged). MongoDB runs without a replica set, so it has no transactions.
- **The admin decides which games are cancelled.** The draw cannot be steered, because the block is
  fixed by the kickoff time. But marking a game `C` is still a trusted admin action (Phase 5).
- **Draws use the public devnet RPC.** One draw takes about 30 RPC calls and can take around 20 s
  because of rate limits. The results request allows 150 s.

### Payments (Phase 3)

- **Not bound to coupon content.** No memo is required, so any matching transfer to the
  treasury in the last hour can be used, once, for any coupons.
- **`confirmed`, not `finalized`.** There is a small risk that a payment is rolled back after being accepted.
- **Not tested with a live successful payment.** The devnet faucet was dry again in Phase 1, so
  only rejections were tested live. The transfer encoding was checked with a devnet simulation,
  and acceptance is covered by unit tests. Phantom's `signAndSendTransaction` request has not been
  run yet. Run one real payment from Phantom through the coupon UI.
- **Public devnet RPC rate limits** (HTTP 429 seen in testing) can make verification and
  the balance display fail. Use a dedicated RPC endpoint.

### Custody and trust (Phases 4-5)

- **Custodial treasury.** Whoever holds the treasury key controls all entry money.
- **Admin is fully trusted.** One admin key creates events and sets results, and
  admin rights come from an env var.

### Sessions and infrastructure (before any public deployment)

- **Tokens cannot be revoked.** Logout only clears the browser. A stolen token works until
  it expires (12 hours). Tokens are kept in `localStorage`, which XSS can read.
- **Single instance.** Sign-in nonces are held in memory. Without `SESSION_SECRET`, every
  restart signs everyone out.
- **No rate limiting** on sign-in or entry endpoints.
- **No database auth.** MongoDB has no authentication (it is now bound to localhost only).
  The optional Mongo Express UI uses default credentials.
- **No production setup.** No HTTPS, no deployment config, no frontend tests. Backend tests
  cover core logic only.
- **Old local data is orphaned.** Data in the old `sol_db`, `admin_db` and
  `predictions_db` databases is not migrated.

---

## Phase 1: Make the Product Work (devnet, test money)

**Goal:** a full event can be played end to end and you are happy with the product.

### User loop

- [x] Admin creates an event and its games (kickoff per game; special events with their own price and tiers).
- [x] User connects a wallet, fills in one or more coupons, pays in devnet SOL with Phantom
      (`frontend/src/views/Event.vue`, `services/entry.js`). The transaction is a hand-encoded System
      Program transfer, checked with a devnet simulation, so `@solana/web3.js` was not added back.
- [x] Admin enters results; cancelled games are drawn; the event is settled once.
- [x] Payouts are calculated and shown (event page, admin payout list per wallet).
- [x] User sees their coupons, results and winnings in My Coupons (`views/MyCoupons.vue`).
- [ ] Run a full event on devnet with several Phantom wallets (blocked so far: the devnet faucet was
      dry, so no real payment has gone through the coupon UI yet).

### Product decisions (the on-chain program will hard-code these later)

Decided Oct 2026. Code: `backend/payouts.go`.

- [x] **Payout economics: shared pool with tiers.** The house keeps **5%** of ticket sales. The rest is
      split by number of correct picks: **50% for 13, 25% for 12, 15% for 11, 5% for 10**. Each tier is
      shared equally by its coupons. Integer leftovers (at most winners−1 lamports per tier) go to the house.
- [x] **Rollover.** Any tier without winners rolls over into the top tier (all correct) of the next event
      that has not started when the results are saved, earliest start time first. If none exists, it is
      held and given to the next event the admin creates. Received rollover is added only to the top tier,
      and the 5% house cut is not taken from it a second time.
- [x] **Price and games.** 0.001 SOL per coupon (`COUPON_PRICE_LAMPORTS`), 13 games by default. Special
      events (7–13 games) need their own coupon price and tiers from the admin. Tiers must add up to
      95% (the house cut stays 5%) and must include "all correct". Price and tiers are stored on the event.
- [x] **Entry deadline:** the event start time (first kickoff). Every game kickoff must be between the
      event start and end.
- [x] **Cancelled or postponed game: weighted lottery.** The admin enters `C` as the result. From
      1 hour after that game's kickoff, the outcome is drawn with weights equal to the number of coupons
      that picked `1`, `X` and `2`. Seed: `sha256("sol-draw:<eventId>:<gameIndex>:<blockhash>")` using
      the first finalized devnet block at or after kickoff + 1 hour. `r` = first 8 bytes (big endian) mod
      total weight. The outcome is the first of `1`, `X`, `2` whose cumulative weight exceeds `r`. If there
      are no coupons, the weights are 1/1/1. The block and weights are stored so anyone can recompute it.
- [x] **Fees.** The user pays the Solana network fee for their payment transaction. The house pays the
      costs of calculation and other on-chain work out of its 5%.
- [x] **Payout execution in Phase 1:** calculated and shown only. The admin pays winners manually from
      the treasury using the payout list. The server holds no keys.
- [x] **Profiles.** A wallet has no name until the first sign-in, which asks for one before the app
      is used. The owner can change it later on the profile page. The wallet address cannot be changed.
      An admin can rename or remove another user only when that user has no coupons on an event whose
      results are not set yet. Removing a user does not delete finished coupons.
- [ ] Unclaimed prize policy (expiry, rollover). Not decided; becomes relevant with `claim` in Phase 4.

### Cheap safety fix worth doing now

- [x] Replace header-based auth: after the wallet signs the login message, issue a
      short-lived session token and require it on protected routes. (Done in Phase 0.)

**Done when:** you have run a full event on devnet with several wallets and the rules feel right.

---

## Phase 2: Write the Rules Spec

**Goal:** an unambiguous description of the game that anyone can use to recompute results.

The spec defines:

- [ ] Coupon format (e.g. the `1`/`X`/`2` string, one character per game) and cost.
- [ ] Event lifecycle: `created -> open -> closed -> resolved | cancelled`, and who may
      trigger each transition.
- [ ] Deadline rule, based on chain time.
- [ ] Payout formula, including rounding and how the pool is split.
- [ ] Refund rules for cancelled events and voided games.
- [ ] Roles and permissions (admin, result signer, user) and what each can never do.

**Done when:** someone else can compute every payout for an event by hand using only the spec.

---

## Phase 3: Real Payment Verification (still devnet)

**Goal:** fake or reused payments are rejected. The design is still custodial at this point.

The stub was replaced in Phase 0 (`backend/solana.go`). Remaining items:

- [ ] Require `finalized` commitment (currently `confirmed`).
- [x] Check sender = authenticated wallet, recipient = treasury, exact amount.
- [ ] Require a memo containing the coupon hash so a payment is bound to its coupons.
- [x] Store used signatures with a unique index; reject reuse.
- [x] Remove the `mock_tx_` / `real_tx_` shortcuts.
- [ ] Refund or recovery path for payments whose entry was rejected.
- [ ] Verify one real end-to-end payment from Phantom on devnet.

**Done when:** made-up, mismatched and replayed transaction hashes are all rejected.

---

## Phase 4: On-Chain Program v1

**Goal:** the program, not the backend, holds the money and enforces the rules.
Keep it deliberately simple.

### Instructions

- [ ] `create_event` - creates the event account and its vault (program-derived addresses).
- [ ] `place_coupon` - transfers SOL into the vault and records the coupon. Rejected after
      the deadline (uses the Solana clock).
- [ ] `resolve_event` - sets the outcomes. The admin is still trusted for results here.
- [ ] `claim` - computes the payout from the stored coupon and pays it exactly once.
- [ ] `cancel_event` / `refund` - refund path for cancelled events.
- [ ] **No instruction lets the admin withdraw from a vault.**

### Notes

- One account per coupon is fine at this stage. Rent is a refundable deposit: closing the
  coupon account after claim returns it, so cost is low at small and medium volume.
  (The ultra-low cost plan counts rent as a pure expense, which overstates the cost.)
- MongoDB becomes a read cache filled from on-chain data.
- Write program tests for every rule in the spec.

**Done when:** a user can place, win and claim on devnet without the backend ever touching funds.

---

## Phase 5: Remove Trust in the Admin

**Goal:** no single key can change results, take funds, or silently replace the code.

- [ ] **Outcomes:** oracle, or M-of-N multisig of independent result signers, instead of one key.
- [ ] **Upgrade authority:** multisig plus timelock; eventually consider making the program immutable.
- [ ] **Pause:** may only stop new coupons. It must never block claims or refunds.
- [ ] **Permissionless refund:** if an event is not resolved within a set time, anyone can
      trigger refunds.
- [ ] **Front-running:** add commit-reveal if picks are visible and copyable before the deadline.
- [ ] **Solvency:** the vault always covers the maximum possible payout under the payout model.
- [ ] **Replay protection:** each claim is bound to event, wallet and coupon, and can succeed once.
- [ ] **Verifiable builds:** prove the deployed program matches the published source.

### Invariants an auditor should be able to confirm

1. An accepted coupon cannot be altered or omitted.
2. The admin cannot withdraw or redirect vault funds.
3. Outcomes cannot be fabricated by one privileged key.
4. Every payout can be independently recomputed from public data.
5. Every eligible winner can claim without admin or indexer cooperation.
6. No claim can be repeated or redirected to another wallet.
7. Cancelled or unresolved events have a permissionless refund path.
8. Upgrades and privileged actions are public and delayed.
9. The deployed binary matches the audited source.

**Done when:** all nine invariants hold and are covered by tests.

---

## Phase 6: Scale and Reduce Cost

**Goal:** apply the ultra-low cost plan once volume justifies it.

- [ ] Store predictions as instruction data plus logs instead of accounts.
- [ ] Make all data reconstructible from transaction history, not only logs (logs can be
      truncated and RPC history is not guaranteed).
- [ ] Run several independent indexers; publish data so results are reproducible.
- [ ] Merkle-root settlement with a challenge window during which anyone can dispute the root.
- [ ] Batch coupons into fewer transactions (check compute and transaction size limits).
- [ ] cNFTs only if needed, with locked-down tree, mint and delegate authorities.

Every change here adds trust assumptions. Re-check each one against the Phase 5 invariants.

**Done when:** measured costs at target volume are acceptable and all invariants still hold.

---

## Phase 7: Audit and Mainnet

- [ ] Internal review against the spec and invariants.
- [ ] Fuzz tests on `place_coupon`, `claim` and `refund`.
- [ ] External audit, using the Phase 2 spec and Phase 5 invariants as the brief.
- [ ] Bug bounty.
- [ ] Mainnet launch with caps on stake per coupon and per event, raised gradually.
- [ ] Monitoring and an incident plan (what pause does and does not do).

---

## Why This Order

| Phase | Risk if skipped |
|-------|-----------------|
| 0 | Building on the wrong copy and losing work |
| 1-2 | Building a secure program around rules you will want to change |
| 3 | Fake payments on devnet hide real bugs |
| 4 | The admin can take the funds |
| 5 | The admin can rig results or swap the code |
| 6 | Only costly at high volume, so it can wait |
| 7 | Real users lose real money |

**Next step:** finish Phase 1 by running a full event on devnet with several Phantom wallets
(fund them at https://faucet.solana.com). Then decide the unclaimed prize policy and start the
Phase 2 spec from the decisions above.
