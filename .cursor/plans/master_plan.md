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
    status: in_progress
    dependencies:
      - phase5_trust_minimization
  - id: phase7_rules_parity
    content: "Phase 7: Program and spec parity (void games, end time, unrevealed coupons, house share, production upgrade delay, capacity limits)"
    status: in_progress
    dependencies:
      - phase6_single_money_path
  - id: phase8_operational_hardening
    content: "Phase 8: Operational hardening (refund memo, shared limiter and nonces, atomic settlement, sessions, RPC, tests, full devnet runs)"
    status: pending
    dependencies:
      - phase7_rules_parity
  - id: phase9_scale_cost
    content: "Phase 9: Apply the ultra-low cost plan (logs + indexer, Merkle claims with challenge window, batching)"
    status: pending
    dependencies:
      - phase8_operational_hardening
  - id: phase10_audit_mainnet
    content: "Phase 10: Tests, fuzzing, external audit, bug bounty, capped mainnet launch"
    status: pending
    dependencies:
      - phase9_scale_cost
---

# Master Plan: Product First, Then Strict Safety

This plan sits next to the
[ultra-low cost plan](ultra-low_cost_solana_architecture_with_state_compression_2d66f835.plan.md).
That plan covers how to make predictions cheap on-chain. This one covers the order of work:
get the product right on test money first, then make it safe, then make it cheap, then launch.

## Ground Rules (apply from day one)

1. **Devnet only until Phase 10.** No real SOL is accepted before the audit. This is what
   makes it acceptable to build the product before the safety work.
2. **MongoDB never decides money.** It may store and display data. Balances, payments and
   payouts move to the chain over time; the database becomes a cache, not the source of truth.
3. **Write every rule down when you decide it.** These notes become the program spec
   (Phase 2) and the document an auditor reviews (Phase 10).

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
      On devnet the loader authority is council `9ad7bFZNUVuvGkM2ZbepzK6wADV4LaWpW2weQPxXSWD2`.
      Its threshold is 2 and its signers are the deployer and
      `4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH`.
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

**Done when:** all nine invariants hold and are covered by tests.

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
- [ ] **Multi-wallet devnet run from the UI.** Blocked on deploying the new binary (see the end of
      this plan).

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
      the override unless `DEVNET_TESTING=1`. That binary stays unused until Phase 10. Devnet keeps
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
- [ ] **Devnet run from the UI.** A multi-wallet run through the app is still open. Phase 8 waits
      for confirmation.

**Done when:** every rule in the spec is enforced by the program or explicitly marked off-chain,
and the program tests cover each decision above.

---

## Phase 8: Operational Hardening

**Goal:** the app survives restarts, multiple instances and ordinary failures before cost work starts.

- [ ] **Refund memo.** The backend must require the `sol-refund-v1:<payment signature>` memo when
      recording a treasury refund.
- [ ] **Shared state.** Move rate limits and sign-in nonces out of process memory so several backend
      instances can run.
- [ ] **Atomic settlement.** Run MongoDB as a replica set and save results, settlement and rollover
      in one transaction.
- [ ] **Sessions.** Revocable sessions and a token store that XSS cannot read.
- [ ] **RPC.** A dedicated devnet RPC endpoint instead of the public, rate-limited one. Coupon
      recovery uses `getProgramAccounts`, which public endpoints throttle.
- [ ] **Chain mirror.** The backend copies a vault's status only when a browser calls the sync
      endpoint; add a periodic sync so listings do not lag the chain.
- [ ] **Tests.** Frontend tests, HTTP tests for the payment and recovery handlers, and an automated
      multi-wallet run for both the treasury and the vault flow.
- [ ] **Full devnet runs.** Settle `Test1` (starts 31 Oct 2026) with several wallets, and repeat the
      Phase 6 vault run.
- [ ] **Source control.** Commit `programs/predictions` and `frontend/src/services/program.js`.

**Done when:** both flows pass repeated multi-wallet runs, and a backend restart or a second
instance does not lose sessions, limits, or settlement data.

---

## Phase 9: Scale and Reduce Cost

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

## Phase 10: Audit and Mainnet

- [ ] Switch the program to the 24-hour build with no override. This is the last program change
      before mainnet, and the last upgrade that can skip the delay.
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
| 6 | One event's money is split between the treasury and a vault, and vault events cannot finish |
| 7 | The program and the spec disagree, so payouts cannot be checked against the rules |
| 8 | Restarts, failures or a second instance lose data or let abuse through |
| 9 | Only costly at high volume, so it can wait |
| 10 | Real users lose real money |

**Next step:** finish the Phase 6 devnet run, then stop for confirmation before Phase 8. The test
program is `H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr`. Its upgrade authority is council
`D6e74RVzp1cTXKQb1UyCygJg17szm9uvphiKZTKfarpt` (2 of 2, five-minute delay, unanimous override). The
old program `35eSgMRzq1xNM4qDDLezingfXmSDM2ZAjRMTWcJ4QNca` is stuck and unused. The 24-hour build
with no override waits until Phase 10. The cost work (Phase 9) waits until Phases 6 to 8 are done.
