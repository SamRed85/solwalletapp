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
    status: completed
    dependencies:
      - phase0_consolidate
  - id: phase1_session_auth
    content: "Phase 1: Replace trust in the X-Wallet-Address header with a session token issued after signed login"
    status: completed
    dependencies:
      - phase0_consolidate
  - id: phase2_spec
    content: "Phase 2: Write the rules spec (coupon format, lifecycle, deadline, payout formula, refunds, roles)"
    status: completed
    dependencies:
      - phase1_product_decisions
  - id: phase3_payment_verification
    content: "Phase 3: Real RPC payment verification (finalized, sender, recipient, amount, memo, no reuse)"
    status: completed
    dependencies:
      - phase2_spec
  - id: phase4_program_v1
    content: "Phase 4: Anchor program v1 with program-owned vault, deadline enforcement, claim, refund, no admin withdraw"
    status: completed
    dependencies:
      - phase3_payment_verification
  - id: phase5_trust_minimization
    content: "Phase 5: Oracle/multisig outcomes, upgrade multisig + timelock, limited pause, solvency, verifiable builds"
    status: completed
    dependencies:
      - phase4_program_v1
  - id: phase6_single_money_path
    content: "Phase 6: One money path per event (block treasury entries on vault events, verify the vault on-chain, admin lifecycle UI, coupon backup)"
    status: completed
    dependencies:
      - phase5_trust_minimization
  - id: phase7_rules_parity
    content: "Phase 7: Program and spec parity (void games, end time, unrevealed coupons, house share, production upgrade delay, capacity limits)"
    status: completed
    dependencies:
      - phase6_single_money_path
  - id: phase8_operational_hardening
    content: "Phase 8: Operational hardening (refund memo, shared limiter and nonces, atomic settlement, sessions, RPC, tests, full devnet runs)"
    status: in_progress
    dependencies:
      - phase7_rules_parity
  - id: phase9_scale_cost
    content: "Phase 9: Logs and Merkle settlement (one coupon per transaction, archive, on-chain challenge, no monthly draw)"
    status: completed
    dependencies:
      - phase8_operational_hardening
  - id: phase11_live_payout
    content: "Phase 11: Live score feed and a projected payout on each coupon if the current scores stand"
    status: in_progress
    dependencies:
      - phase9_scale_cost
  - id: phase12_auto_count
    content: "Phase 12: The reveal keeper counts every coupon after results are approved. The manual count stays as a backup."
    status: completed
    dependencies:
      - phase11_live_payout
  - id: phase13_auto_resolve
    content: "Phase 13: Auto resolve checkbox, on by default, settling only finished games from the score feed"
    status: in_progress
    dependencies:
      - phase12_auto_count
  - id: phase14_interface
    content: "Phase 14: League vault and RAD. Every new event keeps 90% for players, 5% for the house, and 5% in a second vault. RAD is earned per row and is the only entry ticket for an admin-created league event whose unwon jackpot feeds the next one."
    status: completed
    dependencies:
      - phase13_auto_resolve
  - id: phase15_reduction
    content: "Phase 15: Color reduction. Up to 4 colors, each with a min and max, stay on one coupon. The player signs once and pays for the surviving rows. The earlier split into one slip per pattern stays for entries already placed."
    status: completed
    dependencies:
      - phase14_interface
  - id: phase16_game_details
    content: "Phase 16: Game details on the game card. One checkbox per detail: stored API odds for 1, X and 2, the probability calculated from those odds, the start time, and, from the event start onward, the share of rows that picked each outcome. An admin button fetches the odds into MongoDB."
    status: completed
    dependencies:
      - phase15_reduction
  - id: phase17_canopy
    content: "Phase 17: Canopy so one score, place, or claim still fits at 1,000,000 coupons. The program refuses coupon 1,000,001. The tree stays depth 20."
    status: pending
    dependencies:
      - phase16_game_details
  - id: phase18_signed_totals
    content: "Phase 18: Signed totals. The keeper counts off chain, the result signers sign the winner totals, and claims open after a 6-hour challenge window. The admin sets the disputer's minimum and maximum SOL when creating the event (defaults 10 and 50). A challenge locks 10% of the player pot inside that range and starts the on-chain recount. Claims are capped per tier at the signed totals."
    status: pending
    dependencies:
      - phase16_game_details
  - id: fixes_before_final
    content: "Fixes before the final phase: move the upgrade authority to a multisig with a time lock, split the two result keys, show who can approve an event, add a dispute window, and decide what happens to unclaimed prizes. Do not start until the user asks for it."
    status: pending
    dependencies:
      - phase16_game_details
      - phase18_signed_totals
  - id: phase10_audit_mainnet
    content: "Final phase: Tests, fuzzing, external audit, bug bounty, capped mainnet launch. When it starts, choose the independent second signer: it signs only a result it derived from the full-time scores, a mismatch stays unsigned, and the 7-day refund stays. A timeout must not adopt the first signature. Do not start until the user says start final phase."
    status: pending
    dependencies:
      - phase17_canopy
      - phase18_signed_totals
      - fixes_before_final
---

# Master Plan: Product First, Then Strict Safety

This plan sits next to the
[ultra-low cost plan](ultra-low_cost_solana_architecture_with_state_compression_2d66f835.plan.md).
That plan covers how to make predictions cheap on-chain. This one covers the order of work:
get the product right on test money first, then make it safe, then make it cheap, then launch.

## Ground Rules (apply from day one)

1. **Devnet only until the final phase.** No real SOL is accepted before the audit. This is what
   makes it acceptable to build the product before the safety work.
2. **MongoDB never decides money.** It may store and display data. Balances, payments and
   payouts move to the chain over time; the database becomes a cache, not the source of truth.
3. **Write every rule down when you decide it.** These notes become the program spec
   (Phase 2) and the document an auditor reviews in the final phase.

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
  treasury in the last hour can be used, once, for any coupons. **Resolved:** payments now carry
  a deterministic `sol-entry-v1` coupon hash memo.
- **`confirmed`, not `finalized`.** **Resolved:** both entry and refund verification require finality.
- **Live memo payment verified.** On 22 Oct 2026 Phantom wallet
  `BvrkaYfUofi7bbuKqzs79xqp7V6v4h9CosiMKKjvUrQ4` paid 1,000,000 lamports for Test1. Finalized
  devnet transaction `538dMDW3DuWQdccszpKMfSAy3q3cCHtky5AJzW5aK7Vgy28f1qd9mM8oPwg5sVGDTjtGgQmgKEYJWXmAUeVGJKjj`
  (slot 513908456) contains the expected `sol-entry-v1` memo, and the backend accepted the entry.
- **Public devnet RPC rate limits** (HTTP 429 seen in testing) can make verification and
  the balance display fail. Use a dedicated RPC endpoint.

### Custody and trust (Phases 4-5)

- **Custodial treasury.** Whoever holds the treasury key controls all entry money.
- **Admin is fully trusted.** One admin key creates events and sets results, and
  admin rights come from an env var.

### Sessions and infrastructure (Phase 8)

- **Tokens cannot be revoked.** Logout only clears the browser. A stolen token works until
  it expires (12 hours). Tokens are kept in `localStorage`, which XSS can read.
- **Single instance.** Sign-in nonces are held in memory. Without `SESSION_SECRET`, every
  restart signs everyone out.
- **Rate limiting is now enabled** in memory: sign-in attempts are bounded per source IP, while
  entries and payment-recovery requests are bounded per authenticated wallet. A shared limiter is
  still required before running multiple backend instances.
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
- [ ] Run a full event on devnet with several Phantom wallets. `Test2` completed the loop with one
      wallet. `Test1` now has finalized entries from three wallets, including one post-memo payment,
      but it starts on 31 Oct 2026 and cannot be resolved before then.

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
- [x] **Unclaimed prizes do not expire.** Awarded funds remain reserved and claimable indefinitely;
      they are never redirected or rolled over.

### Cheap safety fix worth doing now

- [x] Replace header-based auth: after the wallet signs the login message, issue a
      short-lived session token and require it on protected routes. (Done in Phase 0.)

**Done when:** you have run a full event on devnet with several wallets and the rules feel right.

---

## Phase 2: Write the Rules Spec

**Goal:** an unambiguous description of the game that anyone can use to recompute results.

The spec defines:

- [x] Coupon format (the `1`/`X`/`2` string, one character per game) and cost.
- [x] Event lifecycle: `created -> open -> closed -> resolved | cancelled`, and who may
      trigger each transition.
- [x] Deadline rule, based on chain time.
- [x] Payout formula, including rounding and how the pool is split.
- [x] Refund rules for cancelled events and voided games.
- [x] Roles and permissions (admin, result signer, user) and what each can never do.

Normative document: [`docs/rules-spec.md`](../../docs/rules-spec.md).

**Done when:** someone else can compute every payout for an event by hand using only the spec.

---

## Phase 3: Real Payment Verification (still devnet)

**Goal:** fake or reused payments are rejected. The design is still custodial at this point.

The stub was replaced in Phase 0 (`backend/solana.go`). Remaining items:

- [x] Require `finalized` commitment.
- [x] Check sender = authenticated wallet, recipient = treasury, exact amount.
- [x] Require a memo containing the coupon hash so a payment is bound to its coupons.
- [x] Store used signatures with a unique index; reject reuse.
- [x] Remove the `mock_tx_` / `real_tx_` shortcuts.
- [x] Refund or recovery path for payments whose entry was rejected: users submit the finalized
      payment to a recovery queue; the admin UI sends a treasury refund and the backend verifies
      the finalized refund before recording it.
- [x] Verify one real end-to-end payment from Phantom on devnet. Signature
      `538dMDW3DuWQdccszpKMfSAy3q3cCHtky5AJzW5aK7Vgy28f1qd9mM8oPwg5sVGDTjtGgQmgKEYJWXmAUeVGJKjj`
      was finalized, coupon-bound, and accepted.

**Done when:** made-up, mismatched and replayed transaction hashes are all rejected.

---

## Phase 4: On-Chain Program v1

**Goal:** the program, not the backend, holds the money and enforces the rules.
Keep it deliberately simple.

Source is in `programs/predictions`. Payout math matches the rules spec and passes 7 host
tests. The instructions below ran on an in-process Solana validator
(`solana-program-test` 2.3, `cargo test --test lifecycle`: 8 passed, 0 failed). The app targets program `35eSgMRzq1xNM4qDDLezingfXmSDM2ZAjRMTWcJ4QNca` (devnet-testing build, deploy signature `4f25PffjvxYfxdbmbjWGWg4mUkscG6CbLKkmkTGUb7gS28d6frfZsnAq6pQ22gbdutfokMLFL68szTTj1FK198PA`, finalized). The deployer pubkey is
`955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun`; the keypairs are gitignored under
`programs/predictions/keys/`. Devnet is `4.4.0-beta.1`. The program account is executable. An event can
hold at most 30 coupons in this version so resolution fits in one transaction. Rollover can
only be forwarded to the earliest event that has not started. If none exists, `park_rollover`
holds it for the next event created. A cancelled event keeps incoming rollover reserved for
that same reassignment. The deployable binary `programs/predictions/target/deploy/predictions.so`
was rebuilt from this source with Agave 4.3.2. Its sha256 is in `programs/predictions/build-hash.txt`.
That same binary was loaded into a local Agave 4.3.2 validator and `examples/local_loop.rs`
placed a coupon, rejected a late entry, resolved it with two signers, rejected a stranger's
claim, and paid the owner from the vault. The settled prize was 950,000 lamports. The vault
was left with its rent reserve plus the 50,000 lamport house share, and a second claim was
rejected. `frontend/scripts/program-loop.mjs` repeated that result with the wallet transaction
format in `frontend/src/services/program.js`. The event page, My Coupons, and the admin event
list can sign those same transactions in Phantom once an event has a `chain` record. Until an
admin opens that vault, entries still pay the treasury.

### Instructions

- [x] `create_event` - creates the event account and its vault (program-derived addresses).
- [x] `place_coupon` - transfers SOL into the vault and records the coupon. Rejected after
      the deadline (uses the Solana clock).
- [x] `resolve_event` - sets the outcomes. The admin is still trusted for results here.
      Resolution without every coupon account is rejected.
- [x] `claim` - computes the payout from the stored coupon and pays it exactly once.
      A second claim fails, and the vault keeps the rent reserve plus the house share.
- [x] `cancel_event` / `refund` - refund path for cancelled events. The vault returns to its
      rent reserve.
- [x] **No instruction lets the admin withdraw from a vault.**

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

- [x] **Outcomes:** resolution and cancellation require at least two distinct result signers.
      One signer is rejected. Tested in `one_signer_cannot_resolve_and_pause_spares_claims`.
- [x] **Upgrade authority:** two council signers must schedule an upgrade. Production builds
      wait one day; the devnet-testing build waits five minutes and exposes an explicit
      test-only override that requires every council member. The local test reaches the
      upgrade loader only after the delay or that unanimous override. Only the
      deployer key can create the council. A local validator run rejected a stranger, created the
      council once, and rejected a second create. The signers are the deployer and
      `4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH` (gitignored `keys/council-signer.json`).
      **Superseded by "Fixes before the final phase":** the council cannot upgrade the program,
      because a program cannot upgrade itself. On the live devnet program `AaXABVJE...` the loader
      authority is the deployer key and no council account exists.
- [x] **Pause:** may only stop new coupons. A paused event still pays a claim.
- [x] **Permissionless refund:** after `resolve_timeout`, any caller can expire the event and the
      player can refund. Before the timeout that call is rejected.
- [x] **Front-running:** a coupon stores a commitment at entry. Picks are recorded only in a
      later reveal, and only the committed picks score.
- [x] **Solvency:** resolution rejects a vault that cannot cover prizes, house, and rollover.
- [x] **Replay protection:** a claim is bound to its coupon owner, pays once, and a second wallet
      cannot redirect it.
- [x] **Verifiable builds:** `scripts/verify-program-binary.sh` compared `target/deploy/predictions.so`
      and `solana program dump` of the devnet program. Both hashes are
      `9a2de6c257a270b8f834ef0efc07dfcc55be9debb7d1026bb83654c0b2e5b456`, the same value as
      `build-hash.txt`.

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

**Done when:** all nine invariants hold and are covered by tests. Invariants 3 and 8 do not hold
on the live devnet program. The work that closes them is in "Fixes before the final phase".

---

## Phase 6: One Money Path per Event

**Goal:** every event has exactly one place where its money lives, and a vault event can be run
from creation to payout in the app.

- [x] **Block treasury entries on vault events.** The entry handler rejects any event with a vault
      (also one from another program ID) and re-checks after reserving the payment, so a vault
      attached at the same moment turns the payment into a refund-only recovery. Recovery stays
      open for vault events because a recovery is never turned into an entry; it is labelled refund-only.
- [x] **Verify the vault on-chain before saving it.** `PUT /api/events/{id}/chain` reads the event
      and signer accounts (owner, discriminator, admin, nonce, status, games, price, tiers, deadline,
      end and reveal time, 7-day timeout, 2-of-2 signers) and rejects the vault if any entry or payment
      exists. The cosigner must be the configured `RESULT_COSIGNER`, which may not be an admin wallet.
- [x] **Do not settle vault events in MongoDB.** `POST /results` rejects vault events and they are
      never a rollover target. `POST /api/events/{id}/chain/sync` mirrors the chain status and results.
- [x] **Admin lifecycle UI.** `VaultPanel` (admin dialog and event page): each signer approves results
      (1, X, 2 or Void) or cancellation in its own transaction, anyone runs the batched count, expire,
      forward or park rollover, house withdrawal and pause. Publishing checks the 16-event limit and
      keeps the nonce until the backend accepts the vault, so a failed save can be retried.
- [x] **My Coupons follows chain state.** Rows come from coupon accounts and the event account:
      Reveal only in the reveal window, Claim after `Resolved` (an unrevealed coupon only returns its
      rent), Refund after `Cancelled`.
- [x] **Coupon backup and recovery.** Each coupon stores its picks and nonce encrypted with a key
      derived from a wallet signature; My Coupons can rediscover and decrypt a wallet's coupons on a
      new device, and export or import a JSON backup.
- [x] **Multi-wallet devnet run from the UI.** Confirmed on the live devnet program.

**Done when:** a vault event is created, entered from several wallets, revealed, resolved, claimed or
refunded, and its rollover forwarded, all from the UI, and the API rejects treasury payments for it.
Code and local-validator runs are done; the devnet run waits for the deployment.

---

## Phase 7: Program and Spec Parity

**Goal:** the program, the backend and `docs/rules-spec.md` describe the same game.

- [x] **Cancelled single games.** For vault events a void result `C` counts every pick as correct
      (spec section 10). Treasury events keep the weighted draw.
- [x] **End time.** Results can be approved only from the end time; the reveal deadline equals it.
- [x] **Unrevealed coupons.** Forfeited: the stake stays in the pool, the coupon cannot win, and
      claiming it only returns the account rent. A refund would let players reveal only winners.
- [x] **House share.** `withdraw_house` lets the event admin take exactly the settled house amount,
      once, after resolution.
- [x] **Production upgrade delay, source only.** `build-sbf.sh` builds the 24-hour variant without
      the override unless `DEVNET_TESTING=1`. That binary stays unused until the final phase. Devnet keeps
      the five-minute delay and the unanimous override for the whole test period.
- [x] **Capacity limits.** The 30-coupon cap is gone: approved results lock the event and a
      permissionless, batched `tally` (12 coupons per transaction in the UI) counts every coupon. The
      16 unresolved vaults per admin stays; it is in `/api/config` and checked before publishing.
- [x] **Review fixes.** Rollover escrow is per admin (a global escrow let anyone create an event with
      their own signers and take another admin's parked rollover). `per_winner` is indexed by
      `correct - 1` (13-game events panicked). Event and schedule accounts are boxed (stack overflow).
- [x] **Deploy the test build.** The previous program `35eSgM...QNca` cannot be upgraded: its
      `execute_upgrade` never passes the program-data account to the loader. The fix is in the new
      program `H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr`, built with `DEVNET_TESTING=1` (five-minute
      delay and override). Council `D6e74RVzp1cTXKQb1UyCygJg17szm9uvphiKZTKfarpt`. Hash in `build-hash.txt`.
- [x] **Scripted devnet run.** Place, reveal, both approvals, tally and claim passed on
      `H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr`. The player gained 2,392,800 lamports.
      Claim `4ae2Nz...SPZM`.
- [x] **Devnet run from the UI.** Confirmed on the live devnet program.

**Done when:** every rule in the spec is enforced by the program or explicitly marked off-chain,
and the program tests cover each decision above.

---

## Phase 8: Operational Hardening

**Goal:** the app survives restarts, multiple instances and ordinary failures before cost work starts.

- [x] **Refund memo.** A treasury refund is recorded only when its memo is `sol-refund-v1:` plus the original payment signature.
- [x] **Shared state.** Sign-in nonces and rate limits are stored in MongoDB, so several backend processes share them.
- [x] **Atomic settlement.** Rollover from a settled treasury event is applied once per source event. A restart finishes an apply that did not complete.
- [x] **Sessions.** Login sets an httpOnly cookie. The session id lives in MongoDB and logout deletes it. The token is not stored in localStorage.
- [x] **RPC.** The devnet RPC URL is read from the gitignored `.env` at the repo root. `start-all.bat`
      and `go run .` both load it. The startup log omits the key.
- [x] **Chain mirror.** Every 30 seconds the backend copies open vault status into MongoDB.
- [x] **Tests, handlers and scoring.** Go tests cover payment recovery, the refund memo rejection,
      shared nonces and rate limits, cookie sessions across two processes, and once-only rollover repair.
      `npm test` covers coupon scoring and the session cache. Run with `go test` in `backend` and `npm test` in `frontend`.
- [x] **Tests, multi-wallet.** `frontend/scripts/multi-wallet-devnet.mjs` placed, settled, and claimed a vault
      event for two wallets, then paid and settled a treasury event for two other wallets. Run with
      `node scripts/multi-wallet-devnet.mjs` from `frontend` while the backend is on port 8080.
- [ ] **Full devnet runs.** Settle `Test1` (starts 31 Oct 2026) with several wallets, and repeat the
      Phase 6 vault run.
- [ ] **Source control.** Commit `programs/predictions` and `frontend/src/services/program.js`.

**Done when:** both flows pass repeated multi-wallet runs, and a backend restart or a second
instance does not lose sessions, limits, or settlement data.

---

## Phase 9: Scale and Reduce Cost

**Goal:** remove the per-coupon account so a player pays the coupon price, while hidden picks,
payouts, refunds, and claims stay checkable without trusting one server.

The [ultra-low cost plan](ultra-low_cost_solana_architecture_with_state_compression_2d66f835.plan.md)
is background only. Do not follow its batch of 20, its compressed NFTs, or its treatment of
rent as a spent fee. The entry screen submits one coupon. Extra picks on a game are extra rows
of that coupon, not another coupon. One coupon account now costs about 0.0019836 SOL, which the
player does not get back. At the 0.001 SOL price that deposit is larger than the stake. Those
deposits are the whole monthly pot, about 0.51 SOL once the 256 ticket slots are full. The draw
pays out almost nothing, so this phase removes it.

- [x] **No coupon accounts.** A new coupon is one `place_slip`. The vault receives the stake and
      no coupon account is created. `score_slip` counts one coupon at a time, in index order, and
      the last one settles. `claim_slip` pays the owner. `one_slip_pays_no_coupon_account`,
      `a_slip_claim_pays_the_owner_once` and `an_unrevealed_slip_cannot_win` cover it. Older
      coupon accounts can still be claimed; closing one returns its rent to the owner.
- [x] **One coupon per transaction.** `place_slip` and `reveal_slip` each take one coupon. A
      second coupon is a second transaction. Compressed NFTs are not used.
- [x] **Picks stay hidden until the deadline.** `place_slip` logs the commitment. `reveal_slip`
      logs the picks only after the deadline and writes them into a reveal root. A reveal that
      does not match the commitment is rejected, and the same index cannot be revealed twice.
- [x] **Replay protection.** The slip instruction assigns the next index. A proof for an earlier
      index does not match the root, so that coupon cannot be entered again.
- [x] **History, not only logs.** Each slip, reveal, score, claim and refund is a log line that
      includes the event. The backend stores those lines in MongoDB and can walk older transactions.
      The wallet rebuilds the Merkle proof from that archive with `slipTree.js`.
- [x] **Several indexers.** Saving a line is idempotent, so more than one backend can ingest the
      same transactions into the shared archive. A claim or refund checks the proof on chain, so it
      still works when those indexers are offline and the wallet has the archive.
- [x] **Merkle settlement checked on chain.** `score_slip` accepts one coupon only when its
      entry leaf and reveal leaf fold into the roots. A wrong leaf is rejected. Settlement runs
      when every index has been scored, so an accepted coupon cannot be skipped. The vault then
      pays `claim_slip` against those same roots. There is no separate challenge window: a bad
      score cannot be written.
- [x] **Unrevealed coupons** still cannot win, and the stake stays in the pool. There is no rent
      to return. `an_unrevealed_slip_cannot_win` scores the empty reveal as zero and rejects the claim.
- [x] **Cancellation.** `refund_slip` pays the stake once to the owner from the entry proof.
      A second refund fails, and another wallet cannot redirect it. `a_cancelled_slip_refunds_its_owner_once`.
- [x] **No monthly draw.** New claims do not record a ticket. Closing an older coupon returns its
      rent to the owner. The event page and My Coupons no longer describe a draw. The 5% stays the
      house share. The devnet lottery vault still holds the rent from coupons closed before this
      change, about 0.0027 SOL for one ticket. This program no longer pays that vault.
- [x] Re-check every item against the Phase 5 invariants. A slip claim is signed by the owner and
      does not need the admin. Settlement waits until every index is scored, so an accepted coupon
      cannot be omitted. A repeated or redirected claim fails. A cancelled slip refunds from a proof.

**Done when:** one coupon pays no account deposit, a winner can claim from the archive
alone, a bad root can be rejected on-chain, and the measured cost at the target volume is
acceptable.

---

## Phase 11: Live scores and projected payouts

**Goal:** show each coupon what it would pay if the current scores do not change. This display never settles an event.

The score feed is API-Football. The free plan includes live scores and full-time status, at 100 requests a day. `API_FOOTBALL_KEY` stays in the gitignored `.env` and is never sent to the browser or written in a log.

Each saved game stores that feed's fixture id. Team names are not matched again at score time. A missing id leaves that game without a live score.

- [x] Read the feed once at 45 minutes after the event starts and once at 90 minutes. The admin can refresh that event from its page. Do not call the feed once per open browser.
- [x] Map the score to a projected result: more home goals is `1`, level is `X`, more away goals is `2`. A match that has not started has no projected result. Penalties never change 1, X or 2; a draw after regular time stays `X`.
- [x] On My Coupons and on the coupon being entered, show "If these scores stand" with the rows that would win and the projected SOL, using the same tier math as the spec and the current pool.
- [x] Before other coupons are revealed, other players' picks stay hidden. Show that coupon's correct count and the maximum it would pay if it were the only winner of its tier, and label that figure as a maximum.
- [x] After the reveal window, count revealed rows. An unrevealed coupon scores zero, as it does at settlement. Update the figure when a goal changes 1, X or 2.
- [x] The projection is not a promise and is not the settled payout.

**Done when:** a goal that changes a projected result updates the shown payout, and no score poll can submit a result.

---

## Phase 12: Automatic count

**Goal:** after the result signers approve, the reveal keeper counts every coupon and the last one settles the event. Nobody signs once per coupon.

`score_slip` already accepts any signer, and approving the results already moves the event to tallying with `tallied` at zero. This phase adds the loop. It does not change the payout math, and it does not pay winners. Each winner still claims their own coupon.

- [x] When a vault event is tallying, the keeper sends `score_slip` for `tallied`, then the next index, until the event is resolved. One coupon per transaction, in order.
- [x] The proof comes from the slip archive, including a coupon that was never revealed. An unrevealed coupon scores nothing.
- [x] A restart continues from the on-chain `tallied` counter. A coupon that is already scored is not sent again.
- [x] The keeper pays the network fee. The count does not wait for the admin's wallet.
- [x] **Count coupons and settle** stays and still counts from the same counter.
- [x] If one score transaction does not fit, the keeper stops and says so. This phase does not add a canopy. A few hundred coupons fit. Ten thousand do not. Phase 17 is the canopy.

**Done when:** an approved event with several coupons settles with no Phantom approval for the count, and a stopped keeper finishes the remaining coupons on its own.

---

## Phase 13: Auto resolve

**Goal:** an event can settle itself from finished matches, while a manual event stays manual.

- [x] The create-event form has an **Auto resolve** checkbox. It is on by default and is stored with the event.
- [x] When it is off, the admin sets results as today.
- [x] When it is on, the keeper submits a result only after every linked game is finished, cancelled, or postponed. A live or suspended game waits. Cancelled and postponed become `C`. Finished games use the same 1, X, 2 mapping as Phase 11, read twice and accepted only when both reads agree.
- [x] A game with no fixture id blocks auto resolve and is shown to the admin. It does not guess a match from the team names.
- [x] Submission uses the existing resolution path, including the cosigner. The result can still be saved only once.
- [x] The player-facing event says whether it will auto resolve.
- [x] After the results are on chain, Phase 12 counts the coupons. Auto resolve is not finished while the event is still tallying.

**Done when:** a finished auto-resolve event settles without the admin picking 1, X, 2 or C, and a live score cannot settle it.

---

## Phase 14: League vault and RAD

**Goal:** a second program-owned vault collects 5% of every new event's ticket sales. Playing a row earns one RAD. An admin can open a league event that is entered only with RAD, pays out the whole collected pool, and feeds an unwon jackpot into the next league event.

The player-site restyle that used to sit in this phase was reverted and is not part of this work.

- [x] **90 / 5 / 5.** New events' prize tiers add up to 90. Settlement still computes the old house remainder, then moves exactly 5% of ticket sales from that remainder into the league vault. The event admin can still withdraw the rest once. Rollover is not taxed again. Events already created with tiers that add up to 95 settle exactly as before and do not fund the league vault.
- [x] **League vault.** One global program account, seeds `[b"league"]`. No instruction lets an admin withdraw from it. It is funded only by the 5% transfer inside settlement, and it is emptied only when a league event is created.
- [x] **RAD.** One program account per wallet, seeds `[b"rad", owner]`, holding a balance and a lifetime total. It is not a token and cannot be transferred. A SOL entry credits one RAD per row. A cancelled SOL event does not take that RAD back.
- [x] **League event.** The admin creates it like any other vault event, with `kind = league`, tiers that add up to 100, and a price in RAD (one RAD per row unless the admin sets another price). Creation sweeps the league vault: the parked jackpot becomes `rollover_in`, and the rest becomes `sales`, so it is split across every tier. Entry spends RAD and sends no SOL. Rows on a league event do not earn RAD.
- [x] **Feed-forward jackpot.** A league event's unwon or too-small tiers stay `rollover_out`. Parking them adds that amount to the league vault's jackpot, which the next league event takes as `rollover_in` only. SOL rollover never enters a league event, and a league jackpot never enters a SOL event.
- [x] **Cancel.** Cancelling a league event returns the swept SOL to the league vault (jackpot back to jackpot, sales back to the pool) and each slip refunds its RAD. It does not pay the pool out as SOL refunds.

**Done when:** a SOL event leaves 5% of its sales in the league vault and one RAD per row with the player, and a later league event pays that pool to its winners, or parks an unwon jackpot for the next league event.

---

## Phase 15: Color reduction

**Goal:** a player can start from a large frame of picks and cut it down with up to 4 colors, as on [reducering.se](https://www.reducering.se/hur-funkar-det/fargreducering), and sign once. The coupon carries the frame and the colors. The stake is the coupon price times the surviving rows.

The first build of this phase split each color pattern into its own ordinary coupon, so a tight reduction could ask for dozens or hundreds of signatures. That split stays on devnet for entries already placed. New colored entries do not use it.

How a color works, unchanged from the first build:

- The player marks a frame: any mix of 1, X and 2 on each game. A system coupon is this frame with no colors.
- A color is a group of signs. A sign is one outcome of one game, such as "game 4, X". One sign can be in several colors.
- Each color has a **min** and a **max**. A row is kept only when every color is hit between its min and max.
- At most 4 colors (purple, yellow, red, green). 1X2 reduction, point reduction, and payout forecasts are not in this phase.
- Min cannot be above max. A reduction that leaves no rows is refused. Treasury events stay unreduced.
- The page shows the row count before and after, the share removed, and the cost. A colored entry is one signature, so the page no longer shows a coupon count for the split.

What one coupon stores:

- The frame is the same 13 masks as today. Each color is the signs it covers (13 games by 3 outcomes, bit `3 × game + outcome`), plus a min and a max: 10 bytes per color, so four colors are 40 bytes.
- `place_slip` still takes a commitment and a row count. The row count is the kept rows, and the stake is the price times that count. The colors are inside the commitment, with the masks and the reveal nonce, so they stay hidden until the reveal window.
- The browser record and the time-locked reveal hold the masks, the colors, and the nonce together. A slip has no on-chain sealed backup, so the exported backup file is the copy to keep.
- A coupon with no colors commits exactly as it does today. Events already on chain keep verifying.

How the program counts, without listing rows:

- Reveal publishes the masks and the colors. The program counts the kept rows and rejects the reveal unless the count equals the paid row count. A rejected reveal stays unrevealed: its stake stays in the pool and it cannot win. It does not stop settlement.
- Scoring uses the same count and also tracks how many kept rows have each number of correct picks. For each game it keeps a table of rows so far by correct picks and by hits in each color. An outcome that would push a color past its max is dropped. At the end, a state counts only when every color has reached its min.
- The table size is the product, over the colors, of one more than each color's effective max. A color whose min is 0 and whose max covers every one of its signs adds no states. The program refuses a coupon whose table is over the cap of 64 states, and the coupon page shows the refusal before the signature. On a local Agave 4.3.2 validator the worst case at the cap (13 games, every outcome, three colors at 4 × 4 × 4 states, 32,640 kept rows) used 156,195 compute units to reveal and 325,996 to score and settle. `examples/local_loop.rs` repeats that measurement.
- A void game still scores every pick correct. Colors describe picks, not results.
- The same count is implemented in the program, in the browser, and in the Go pick-share code. One shared set of test vectors checks all three. The browser refuses anything the program would reject, including a sign that is not in the frame, a color with no signs, and a table over the cap.

Security rules:

- **One count everywhere.** Reveal, scoring, and claim all call the same colored count. `claim_slip` pays `per_winner` times this coupon's kept rows by score, never the raw frame. A frame of every outcome is 1,594,323 rows; paying that when 1,000 were scored would drain the tier pot.
- **The kept rows equal the paid rows.** Across every score, a coupon's kept rows add up to the row count in its entry leaf. Reveal rejects anything else.
- **The commitment binds the colors.** Every sign, min, and max is inside the commitment and the reveal leaf, as the exact list of up to four 10-byte colors. Any other list, order, or byte hashes to a different commitment, so a coupon can only be revealed the way it was committed. Reveal also rejects a color with no signs, a sign outside the frame, and a min above its max or a max above the game count.
- **Old bytes for no colors.** A coupon with an empty color list hashes its commitment and its reveal leaf exactly as before, so coupons already on chain still reveal, score, and claim.
- **Settlement cannot stall.** The state cap is checked at reveal, and it is measured on `score_slip`, which also checks three proofs. A coupon that would run out of compute while scoring is refused at reveal and stays unrevealed. Never lower the cap after colored coupons exist: a revealed coupon above a lower cap could no longer be scored.
- **Treasury and legacy coupon accounts** never carry colors.

Edges the count has to get right:

- Every outcome selected on all 13 games is 1,594,323 rows. That entry is allowed only when the colors bring the kept rows down to 44,000 or fewer. The count does not walk the rows.
- Zero kept rows is refused. One kept row is a normal coupon. Min 0 and a max that covers every sign keeps the whole frame.
- Two signs of the same color on one game can add at most one hit, because a row picks one outcome. A min above that returns zero rows.

Already built, and left in place for entries that used it:

- [x] **Spec for the split.** `docs/rules-spec.md` describes colors, min and max, and the split into disjoint systems, including the 41-coupon example.
- [x] **Browser splitter.** `reduceSystem` built the pieces. It is replaced by `colorReduction`; groups already saved keep their pieces.
- [x] **MongoDB group and signing queue.** `reductions` stores the encrypted plan. The page signs one `place_slip` per piece and can resume. Reveal, claim, and refund follow the same queue. The page now only resumes groups saved before the upgrade.
- [x] **Coupon page.** The four colors, min, max, row count, share removed, and cost are on the event page.
- [x] **Uncolored coupons.** A coupon with no colors is still one slip, and its commitment is unchanged.

One coupon per colored entry:

- [x] **Spec.** `docs/rules-spec.md` has the one-coupon rule: colors travel inside the commitment, reveal checks the kept-row count, and scoring and claims count only kept rows. Entries placed before this upgrade remain a group of ordinary slips.
- [x] **Counting function.** `kept_counts` in `programs/predictions/src/math.rs` counts with a table of states and never lists rows. Tests compare it to brute force on 600 random frames and colors, and cover the 13-game edges, zero rows, and the cap. The table uses precomputed flags per state, a bitmask of live states, and one score slot when there are no results; the first version ran out of compute at the cap.
- [x] **Program.** `RevealArg` ends with the color list. The commitment and the reveal leaf add `colors-v1` and the color bytes only when there are colors. `reveal_slip`, `score_slip`, and `claim_slip` all use the kept rows, and their sum must equal the paid rows. `reveal-v1` logs the colors as a 7th field only for a colored coupon.
- [x] **Keeper, archive, and counts.** `backend/colors.go` has the leaf, commitment, and count. The time-locked reveal carries the colors, `archive.go` reads the 7th field, the keeper's reveal and score send the colors, and the live winner counts and pick share count only kept rows.
- [x] **Browser.** `colorReduction` prices the frame with the same count and cap, and the event page enters one slip with one signature. Reveal, score, claim, the projected payout, and the settled score on My Coupons all use the colors.
- [x] **Shared vector.** The same colored commitment and reveal leaf are pinned in the program, Go, and browser tests.
- [x] **Lifecycle test.** A colored coupon is placed, revealed, scored, and claimed and is paid for its 14 kept rows only, not the jackpot row in its frame. Changed colors, dropped colors, and a paid count that differs from the kept rows are rejected.
- [x] **Devnet upgrade.** Built with `DEVNET_TESTING=1` and upgraded. New colored entries sign once. Groups already saved in `reductions` still finish through the old queue.

**Done when:** a player can mark a large frame, add colors, see the kept rows and the cost, and enter with one signature. Reveal and scoring count only the rows inside every color's range, and an uncolored coupon is unchanged. A reduction that leaves no rows, or whose count is too expensive, is refused before the signature.

---

## Phase 16: Game details on the game card

**Goal:** a player can choose which extra details each game card shows. There is one checkbox per detail: odds, probability, start time, and pick share. The settings apply to every game card on the page.

- [x] **Checkboxes.** Add a small "Show on game cards" control above the games in `frontend/src/views/Event.vue` with four checkboxes: **Odds**, **Probability**, **Start time**, and **Pick share**. Each one shows or hides its detail on every game card. The choice is saved in `localStorage` per browser, not per event. By default, Start time is on and the other three are off. A detail with no data for a game, such as a game with no fixture id, shows a dash on that card.
- [x] **Odds from the API.** An event is about 13 high-profile games from one weekend day, so the chosen bookmaker has a Match Winner line for each of them. The backend reads 1X2 odds from API-Football (`/odds?fixture=<id>`, bet "Match Winner") in `backend/football.go`, using the fixture id saved on each game. Odds come from one fixed bookmaker, named in the config, so all games use the same source. The free plan allows 100 requests a day and that budget is already shared with scores and auto resolve, so this phase does not fetch odds on a timer. An admin-only button on the event page, next to Refresh scores, fetches the odds for that event's games and stores them in MongoDB. The game card shows the stored odds. A game that has not been fetched yet shows a dash. Show the odds as decimals with 2 decimals for `1`, `X` and `2`. The API key never reaches the browser.
- [x] **Probability from the odds.** For each game, the raw value for each outcome is `1 / odds`. These raw values add up to more than 100% because of the bookmaker margin, so divide each one by their sum. Show the result as a whole percent for `1`, `X` and `2`, so the three add up to about 100%. The calculation runs in one shared function and is tested against worked examples. These numbers are an estimate only and never affect payouts.
- [x] **Start time.** The checkbox shows or hides the kickoff stamp that game cards already display (`kickoffStamp`).
- [x] **Pick share, only once the event has started.** From the event start time onward, and for good after that, each game card shows the share of all rows on the event that picked `1`, `X` and `2` for that game. Before the start this detail stays hidden, and the checkbox says "after start". Shares count rows, not coupons, so a system coupon with 2 picks on a game adds weight to both outcomes based on how many of its rows use each one. A Phase 15 reduced coupon counts only its surviving rows.
- [x] **Hidden picks stay hidden.** Vault events keep picks secret until they are revealed (Phase 9). Pick share counts only revealed coupons, says "from N of M coupons revealed", and updates as more coupons are revealed. Unrevealed coupons are not guessed at. Treasury events count every entry from MongoDB. The backend computes the shares once per update and serves them on the event, so no browser loads every coupon.
- [x] **Tests.** Test probability normalization, row-weighted pick share for a system coupon and a reduced coupon, and that no share is returned before the event start.

**Done when:** a player can turn each of the four details on or off for every game card. After an admin fetches the odds, they and the probabilities appear on the linked games. Pick share shows nothing before the event starts and, from then on, the share of revealed rows for each outcome.

---

## Phase 17 Event types and type-aware rollover

In progress (Nov 2026). The backend and frontend parts ship now. The program part goes into the next devnet build.

**Goal:** the admin picks one of three event types when creating an event. The type decides the icon in the event list, the default game count and start day, and where an unclaimed jackpot goes: always to the next event of the same type, never to another type.

The three types, with their working names and the alternatives that were considered:

| Type | Code | Runs | Games | Icon | Name options |
|------|------|------|-------|------|--------------|
| Saturday 13 | `saturday` | Every Saturday, fixed slot. Entries close 15:59. | 13, locked | Star | — |
| Wildcard 13 | `wildcard` | Any day of the week. Entries close at the first kickoff. | 13, locked | Bolt | — |
| Sprint 8 | `sprint` | Any day of the week. Entries close at the first kickoff. | 8 by default, 7 to 12 allowed | Dice | — |

A jackpot overrides the type colour with gold in the list, as it does today. League stays a separate checkbox, because it changes the currency, not the schedule. The design mockups are in the canvas `event-types-icons-rollover`.

How rollover already works, and what changes:

- A settled event keeps its unclaimed tiers as `rollover_out` in its own vault until the admin presses rollover. Nothing moves on its own. Two or more settled events can wait with a rollover each, of the same type or of different types, and none of the amounts mix, because each stays in its own vault.
- Pressing rollover does one of two things. If an event of the same type exists that has not started, `forward_rollover` moves the amount straight into that event's vault; the escrow is not involved. If none exists, `park_rollover` moves it into the admin's escrow, and the next event of that type the admin creates takes it as `rollover_in`.
- The program keeps one `kind` byte per scheduled event in `ScheduleKinds`, and `earliest_future` and `forward_rollover` already route by that byte. Today it is `0` for SOL and `1` for League. The type becomes part of that byte, so the same code routes by type with no new instruction.
- The escrow stays one account per admin with one balance, and the account does not grow. The kind rides in the top byte of `pending` (the low 56 bits are the lamports). An escrow written before types has those bits at zero, so it reads as Saturday. A park of another kind while the balance is above zero is refused, and `create_event` only sweeps the escrow when the new event's kind matches. Two parks of the same type add up and go to the next event of that type together. The admin's flow is "press rollover, then create the next event of that type", and the list offers that as one transaction.

Rules:

- [x] **Type on the event.** `backend/events.go` gets `Type string`, one of `saturday`, `wildcard`, `sprint`, checked on create. An event saved before the field existed reads as `saturday`. The on-chain `kind` byte encodes it: `0` Saturday, `2` Wildcard, `3` Sprint; `1` stays League. Vault events already on chain read as Saturday.
- [x] **Rollover target by type.** `rolloverTarget` in `backend/payouts.go` adds the type to its filter, so a treasury event's rollover only lands on an event of the same type. The parked bucket in the `rollover` collection is one document per type (`pending:saturday`, `pending:wildcard`, `pending:sprint`). A new treasury event drains only its own type's bucket, and a Saturday event also drains the old `pending` document. The repair loop replays the stored target, so it follows the type chosen when the event was settled.
- [x] **Escrow guard.** The kind is the top byte of `Escrow.pending`. The account stays the same size, so no realloc and no migration. `park_rollover` refuses a second kind while the balance is above zero. `create_event` sweeps only when the new event's kind matches. An escrow from before the upgrade reads as Saturday, so a parked amount at upgrade time goes to the next Saturday event, which is what it does today.
- [x] **Rollover in one transaction.** The vault panel has "Park and create the next {type} event" next to the plain park button. It opens the create form for that type, and publishing sends `park_rollover` and `create_event` in one transaction. The plain park button stays for the case where the next event is not ready yet.
- [x] **Admin create form.** The type selector is the first field in `Admin.vue` and replaces the "standard or special" split inferred from the game count. Saturday 13 and Wildcard 13 lock the game count at 13. Sprint 8 defaults to 8 and allows 7 to 12, and opens the custom price and tier editor. Saturday preselects the next Saturday 15:59 as the start time. The form says where this event's rollover will go.
- [x] **Event list icon.** `Home.vue` replaces the game-count circle with a type mark: star, bolt or dice in a rounded tile, with the game count in a small corner badge. Gold when a jackpot is attached. The filter chips under the tabs stay All and Jackpot. The type is shown by the icon, not by a filter.
- [x] **Copy.** `Event.vue` and `VaultPanel.vue` say "the next Saturday event" (or Wildcard, or Sprint, or league) instead of "the next event", and link to it once it exists.
- [x] **Tests.** Go tests: a Wildcard rollover never lands on a Saturday event, each type has its own pending bucket, and the old `pending` bucket is drained only by Saturday. Program test: a park of a second kind is refused while the escrow holds another kind, two parks of the same kind add up, `create_event` of another kind leaves the escrow alone, and a forward to an event of another kind is refused.

**Done when:** the admin creates one event of each type from the form, the list shows a different icon for each, a settled Wildcard event's jackpot reaches the next Wildcard event and only that one, and a parked Sprint jackpot waits in the escrow until the next Sprint event is created while a Saturday event created in between takes none of it. The program change is in the source and ships with the next devnet upgrade. Until that upgrade, Saturday vaults work as before and a Wildcard or Sprint vault cannot be published.
---
## Phase 18: Canopy for large events

**Goal:** placing, counting, and claiming one coupon still fit in a Solana transaction at 1,000,000 coupons, and the program refuses another one. The tree stays depth 20, which could hold 1,048,576. The event stops at 1,000,000.

Empty proof hashes are already left out. Above a few hundred coupons the upper hashes are real, and three of those proofs do not fit. A canopy is the upper part of the tree stored by the program. The wallet sends only the bottom of the proof. The program attaches the stored top and checks the same root.

- [ ] New events grow a canopy as coupons are placed. The program writes the shared upper nodes. The client does not send them again on place, reveal, score, claim, or refund.
- [ ] A score, place, or claim for one coupon stays under the Solana size limit at coupon 1,000,000.
- [ ] `place_slip` rejects the coupon that would make the count 1,000,001. The create-event page and the coupon page say the event is full.
- [ ] The tree depth stays 20. Events already on chain keep the proofs they use now.
- [ ] The stored nodes have to fit in accounts the event can rent.
- [ ] The payout math does not change. The on-chain count is still one coupon per transaction, in order. A canopy makes each transaction fit. It does not settle every coupon in one approval. After Phase 18, that count runs only as the recount after a challenge, or when no totals are signed in time.

**Done when:** coupon 1,000,000 can be placed, counted, and claimed in a transaction that fits, and coupon 1,000,001 is rejected.

---

## Phase 19: Signed totals and a challenge window

Do not start this phase until the user asks for it. 

**Goal:** an event with 1,000,000 coupons settles in hours, not weeks. Counting moves off chain. Each claim is still checked on chain. A wrong total can be thrown out by anyone during a short window, and the vault can never pay out more than the pool.

At about 2 seconds per `score_slip`, the Phase 12 count takes about 23 days for 1,000,000 coupons. One slot per coupon is still about 4.6 days and about 5 SOL in fees. A claim already checks the coupon's entry and reveal proofs and recomputes its score, so the only thing the chain still needs from the count is the number of winning rows per tier, which sets the payout per row.

How it works:

- After the results are approved, the keeper counts every coupon from the archive and posts the winner rows per score. The event's result signers sign that exact list, with the same threshold as a result: 2 today, more if the final phase allows a larger set. A list that differs in any value is a different proposal.
- The poster locks a 3 SOL bond with the totals.
- The 6-hour window starts when the totals land, not at result approval. It is measured in slots, about 54,000, so a cluster halt does not shorten it.
- During the window, claims are closed, and anyone can challenge by locking a bond.
- When the window ends with no challenge, anyone can call `finalize_totals`. It sets the payout per row, runs the existing settlement (house share, league 5%, rollover), opens claims, and returns the poster's bond.
- A challenge starts the recount: `tallied` goes back to 0 and the existing `score_slip` count runs, one coupon per transaction, in order. Anyone can send scores. When the recount reaches `coupon_count`, the program compares it with the signed totals. This is the same count as Phase 12, so 1,000,000 coupons still takes about 23 days at the current pace, or about 4.6 days if each transaction lands in one slot. Claims stay closed until it finishes.
- If the totals were wrong, the recount totals are used. The challenger gets their bond back plus the poster's bond, and the event shows that its signed totals were rejected.
- If the totals were right, the signed totals stand. The challenger's bond first pays back the recount fees of whoever sent the scores, and the remainder goes to the event pool.

Rules:

- [ ] **Totals instruction.** `sign_totals` records one signer's approval of a totals list and its poster, the same way `approve_resolution` records a result. When the threshold agrees on the same list, the totals are locked, the poster's bond moves into the event, and the window opens. A second list cannot replace a locked one.
- [ ] **Bounded totals.** Every revealed row has exactly one score, so the totals for 0 through 13 correct must add up to the kept rows of every revealed coupon. `reveal_slip` keeps that running sum on chain. Totals that add up to anything else are rejected, so rows cannot be invented or dropped as a whole. Rows can still be moved between tiers, and the challenge is what catches that.
- [ ] **Per-tier claim cap.** `claim_slip` adds the rows it pays to a per-tier counter and refuses a claim that would go past the signed total for that tier. The tier pot is the ceiling, whatever was signed.
- [ ] **Disputer bond set on the event.** The create-event form has a minimum and a maximum SOL for the disputer. The defaults are 10 and 50. Both are whole SOL amounts from 1 to 100, and the minimum cannot be above the maximum. The program stores them on the event. The bond is 10% of the player pot, clamped to that event's minimum and maximum. The player pot is the amount the tiers divide: sales plus rollover, after the house and league shares. With the defaults, a pot of 100 SOL or less locks 10 SOL, a pot of 250 SOL locks 25 SOL, and a pot of 500 SOL or more locks 50 SOL. The page shows the amount before the challenger signs. Events created before this field existed use 10 and 50. A minimum under 10 SOL does not cover the fees of a recount at 1,000,000 coupons.
- [ ] **One challenge.** Only the first challenge counts. A later challenge is refused and moves no money. Only the challenger's signing key can receive the bonds, so copying the challenge transaction earns nothing.
- [ ] **Recount fees.** Each `score_slip` in a recount records its fee payer and the fee. The losing bond pays those fees back first, up to the bond, before anything else is paid. At 1,000,000 coupons the recount costs about 5 SOL. With the default minimum of 10 SOL, a false challenge covers those fees and the remainder goes to the event pool. A lower minimum set on the event can leave some of the fees unpaid. A challenge that wins is repaid from the poster's 3 SOL bond, so fees above that stay with whoever sent the scores.
- [ ] **No totals in time.** If no totals are locked within 24 hours of result approval, anyone can start the on-chain count without bonds, as today. An event cannot be stuck because nobody signed.
- [ ] **Signers can still cancel.** During the window the result signers can cancel the event with the usual threshold. Every stake is then refunded. This is the dispute window from "Fixes before the final phase": a wrong result, such as a mistyped `C`, can still be undone before money moves.
- [ ] **Claims wait.** No claim is paid while the event is in the window or a recount. A signer who undercounts a tier cannot claim first.
- [ ] **Too-high totals.** A total that is too high makes every payout smaller and leaves the rest in the vault. That remainder follows the unclaimed-prize rule chosen in the fixes phase.
- [ ] **Events already on chain.** Events already tallying, and events created before the upgrade, keep the Phase 12 count. The new path applies to events created after the upgrade.

Off-chain work:

- [ ] **Keeper.** It counts with the same colored row count as the program (`kept_counts`, the shared test vectors), builds the scored list, asks the signers to sign, and posts. A count error in Go must lose a challenge, not settle.
- [ ] **Two watchers.** At least two watchers, each with its own archive built from chain history as the event runs, on different RPC providers. Neither may read the other's MongoDB. Each counts independently and challenges automatically when its totals differ from the signed totals, and it alerts the operator either way. Many RPC providers drop old transactions, so the archive is kept from the first coupon, not rebuilt after the event.
- [ ] **The second signer counts on its own.** It signs totals only when its own count, from its own archive, matches. When the counts differ, it does not sign, and the 24-hour fallback applies.
- [ ] **Event page.** It shows the signed totals, the time left in the window, the bond amounts, the challenge button, and the recount progress, and it says whether the totals were confirmed or rejected.

Tests:

- [ ] Correct totals settle after the window with no challenge, and claims pay as before.
- [ ] Undercounted totals are challenged and recounted, the recount totals are used, and the challenger receives both bonds.
- [ ] A false challenge loses its bond, the recount fees are paid back first, and the signed totals stand.
- [ ] A claim past the tier cap is refused. A claim during the window or a recount is refused.
- [ ] A second challenge, a totals list that differs between signers, totals above the revealed rows, and a window shortened by wall-clock time are all rejected.
- [ ] With no totals after 24 hours, the old count runs and settles.

**Done when:** a devnet event settles from signed totals 6 hours after posting with no score transactions, a deliberately wrong total is thrown out by a watcher's challenge and recounted, and no claim can pay more than its tier pot.

---

## Fixes before the final phase

Do not start this phase until the user asks for it. It does not wait for Phase 17, and the final phase does not start until this one is done. Nothing here changes the live devnet program until the user asks for a build.

The Phase 5 council cannot upgrade the program. `execute_upgrade` fails with PrivilegeEscalation: the loader needs the program account writable, and the runtime keeps the running program read-only. On devnet the upgrade authority is the deployer key `955GUT...`, so that one key can replace the program instantly and can therefore take any vault. No council account exists for program `AaXABVJE...`.

- [x] **Stop the handoff that would freeze the program.** `handoff-devnet.sh` and `deploy-devnet.sh` moved the authority to the council PDA, which can never sign an upgrade. The handoff is disabled in both. `upgrade-devnet.mjs --probe` is retired. Council addresses are derived from the program id. The copied address `D6e74...` belonged to the retired program. `examples/initialize_upgrade.rs` derives the address too, so a local run no longer panics on the old one.
- [ ] **Add a dispute window.** Once the threshold approves, the event is in Tallying and cannot be cancelled, so a wrong result, including a mistyped void game `C`, cannot be corrected. Phase 18's 6-hour totals window is that window: the signers can cancel during it. Ship both in this build.
- [ ] **Decide unclaimed prizes.** Winnings with no claim stay in the vault forever, so a lost key locks that share. Choose one: leave them, or roll them into the next event after a claim deadline. A rollover needs a program change, so it belongs in the same build as the items below.
- [ ] **Delete the dead council instructions.** Remove `schedule_upgrade`, `execute_upgrade`, `override_upgrade_delay` and `initialize_upgrade`. The last one hard-codes the deployer address. The devnet-testing build's five-minute delay and override go with them.
- [ ] **Build the production binary.** The live program is the devnet-testing build. The replacement drops the delay and the override, because the delay will live in the multisig. This build also carries the dispute window and the unclaimed-prize rule. It is the last program change before the audit, and the last upgrade the deployer key signs.
- [ ] **Move the upgrade authority off the deployer key.** After that build is deployed, move the loader authority to a multisig program, such as Squads, with a 24-hour time lock, or burn the authority after the audit. Until then the deployer key remains a single point of control.
- [ ] **Split the two result keys.** With auto resolve on, the backend holds the admin key and `keys/council-signer.json`, so one machine approves both. That file is also named as a council member. Separate the keys in this phase. How the second signer acts is the option chosen at the start of the final phase.
- [ ] **Show who can settle an event.** Any wallet can create an event with two keys it controls, and players have to trust that creator. The event page shows the signer set. On mainnet, either only a platform signer set is allowed, or a third signer is required that the creator does not choose.

**Done when:** no single key can replace the program or approve a result, the event page shows the signers, a wrong approval can still be cancelled inside the dispute window, and unclaimed prizes follow the chosen rule.

---

## Final phase: Audit and Mainnet

Do not start this phase until the user says "start final phase", and not before "Fixes before the final phase" is done. The app stays on devnet until then.

**Choose this when the phase starts.** The second result signer runs apart from the first and signs only a result it has derived itself from the full-time scores. When both sides produce the same 1, X, 2 string, both approvals land and the event settles with no one at the keyboard. When they differ, the event stays unsigned. The 7-day rule stays: after the resolution window, anyone can cancel and every stake is refunded. A timeout must not adopt the first signer's result, because that would let one key settle the pot by waiting.

- [ ] **Independent second signer.** Take the option above. The first signature never becomes the result on its own. The same signer also signs the Phase 18 totals only from its own count.
- [ ] **More result signers.** Decide whether an event can have more than 2 result signers, and its threshold. Results and Phase 18 totals use the same set.
- [ ] Confirm the deployed program is the production build from the fixes phase, with the council instructions removed and the upgrade authority on the multisig.
- [ ] Internal review against the spec and invariants.
- [ ] Fuzz tests on `place_coupon`, `claim` and `refund`.
- [ ] External audit, using the Phase 2 spec and Phase 5 invariants as the brief.
- [ ] Bug bounty.
- [ ] Mainnet launch with caps on stake per coupon and per event, raised gradually.
- [ ] Monitoring and an incident plan (what pause does and does not do).
- [ ] Replace the free API-Football plan so the app is no longer capped at 100 requests a day. Then refresh scores and odds on a schedule, instead of the Phase 11 score marks and the Phase 16 admin odds button.

---

## Why This Order

| Phase | Risk if skipped |
|-------|-----------------|
| 0 | Building on the wrong copy and losing work |
| 1-2 | Building a secure program around rules you will want to change |
| 3 | Fake payments on devnet hide real bugs |
| 4 | The admin can take the funds |
| 5 | The admin can rig results or swap the code |
| 6 | One event's money is split between the treasury and a vault, and vault events cannot finish |
| 7 | The program and the spec disagree, so payouts cannot be checked against the rules |
| 8 | Restarts, failures or a second instance lose data or let abuse through |
| 9 | Players keep paying about 0.002 SOL per coupon into a monthly draw that pays out almost nothing |
| 11 | Players cannot see what the current scores would pay, or a live score is mistaken for a settlement |
| 12 | Someone has to approve a transaction for every coupon before the event can settle |
| 13 | Someone has to type every result, or a live match is settled before it is finished |
| 14 | Players keep 95% of every pool, and there is no season event funded by play |
| 15 | Players can only buy every row of a system, or a colored entry asks them to sign once per pattern |
| 16 | Players pick without seeing the odds, the probabilities, or how everyone else picked once the event has started |
| 17 | Every 13-game event looks the same in the list, and a Wildcard jackpot can land on Saturday, or the other way round |
| 18 | An event past a few hundred coupons cannot be counted or claimed, or it keeps accepting coupons after 1,000,000 |
| 19 | A 1,000,000-coupon event takes about 23 days to count before anyone can claim |
| Fixes | One key can replace the program and take every vault, one server approves both sides of a result, and a wrong result cannot be undone |
| Final | Real users lose real money |

**Next step:** Phase 15 is done: a colored entry is one coupon and one signature, and the devnet program counts only its kept rows. Groups split before the upgrade still finish through the old queue. Phase 16 game details are on the event page. Phase 17 is the canopy and does not start until the user asks for it. Phase 18 (event types and type-aware rollover) is built: Saturday 13, Wildcard 13 and Sprint 8. The backend and the pages are in this tree. The program change ships with the next devnet upgrade; until then a Wildcard or Sprint vault cannot be published. Phase 19 replaces the coupon-by-coupon count with signed totals, a 6-hour challenge window and an on-chain recount on challenge. It does not start until the user asks for it, and its program change ships with the fixes build. "Fixes before the final phase" covers the upgrade authority, the result signers, the dispute window and unclaimed prizes, and does not start until the user asks for it. Do not start the final phase until the user says "start final phase". The old lottery vault is not paid by this program.
