# Devnet Acceptance Gate

Complete this gate before starting the Phase 4 program. Use test SOL only.

## Preconditions

1. Start the current backend and frontend. Confirm `GET http://localhost:8080/health`
   returns `{"status":"healthy"}`.
2. Set Phantom to **Devnet**. Use at least three funded accounts controlled by the tester.
3. Create an event with enough time to submit every entry and later resolve it. Games and
   economic rules must be fixed before the first payment.

## Payment checks

For each wallet:

1. Sign in by approving the login message.
2. Select a coupon system and record its canonical game sets in `1`, `X`, `2` order.
3. Approve the payment and wait for My Coupons to show the accepted entry.
4. Record the wallet, event ID, signature, paid lamports, and canonical game sets.
5. Independently verify the finalized transaction:

```powershell
node scripts/verify-devnet-payment.mjs `
  <signature> <event-id> <payer-wallet> <treasury-wallet> <lamports> `
  "1,X,2,1X,12,X2,1X2"
```

The last argument has one comma-separated set per game; the seven values above are only an
example. The command must print `"verified": true`. It checks:

- the signature exists at finalized commitment on devnet;
- the transaction succeeded;
- the authenticated payer transferred exactly the expected lamports to the treasury; and
- exactly one memo equals the SHA-256 coupon binding in the rules specification.

Also confirm these negative cases through the API or unit tests:

- an invented signature is rejected;
- the same signature cannot create a second entry;
- changing any game set causes a memo mismatch;
- changing payer, recipient, or amount is rejected.

## Event completion

1. After the deadline, confirm new entries are rejected.
2. Submit all game outcomes. If testing a cancelled game, wait until one hour after its kickoff
   and independently recompute its stored draw.
3. Confirm settlement is stored once and a second result submission is rejected.
4. For every submitted coupon, recompute its score and payout using `docs/rules-spec.md`.
5. Confirm My Coupons shows each wallet's rows, scores, and winnings.
6. Confirm the admin payout totals equal the settlement's paid amount.
7. Confirm:

```text
sales + rollover_in = house + paid_to_winners + rollover_out
```

## Recovery check

Use a separate small payment if testing recovery:

1. Make a valid payment, but request recovery instead of retrying the entry.
2. Confirm it appears once in the admin recovery list.
3. Connect the configured treasury wallet and issue the refund.
4. Confirm the backend records it only after the refund transaction is finalized and proves an
   exact treasury-to-user transfer.
5. Confirm neither the payment nor refund signature can be reused.

## Evidence to retain

- Event ID and final event JSON.
- Each entry and payment signature.
- Output from the independent verifier for every post-memo payment.
- Final settlement and admin payout response.
- Any cancelled-game slot, blockhash, block time, weights, and recomputed outcome.

Phase 1 and Phase 3 are complete only when all checks above pass for one event containing entries
from several wallets and at least one payment made after the memo requirement was introduced.
