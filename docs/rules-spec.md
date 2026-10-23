# Sol Prediction Rules Specification

Version 1, 22 October 2026. This specification describes the devnet product and is the
normative input for the first on-chain program. All amounts are integer lamports.

## 1. Roles and authority

- A **user** controls a Solana wallet, submits picks, pays for them, and may claim only
  winnings or refunds belonging to that wallet.
- An **event administrator** creates events and games. In the current devnet product the
  administrator also submits results. The on-chain design must never let this role withdraw
  from an event vault (other than the settled house share, see section 10), redirect a claim,
  alter an accepted coupon, or block claims/refunds.
- A **result signer** resolves outcomes. Phase 4 may use the administrator; Phase 5 replaces
  that single signer with an oracle or M-of-N independent signers.

MongoDB is a display cache. It is not authoritative for payments, accepted coupons, results,
claims, or refunds once those records move on-chain.

## 2. Event and game data

An event has an immutable identifier, title, game count, coupon price, prize tiers, entry
deadline, end time, ordered games, and vault. It has 7 through 13 games. Each game has an
ordered index, home team, away team, and kickoff time. Kickoff must be between the event
deadline and end time, inclusive.

A standard event has 13 games, costs 1,000,000 lamports per coupon row, and uses these tiers:

- 13 correct: 50% of ticket sales
- 12 correct: 25%
- 11 correct: 15%
- 10 correct: 5%

A special event explicitly stores its price and tiers. Each tier identifies a distinct number
of correct picks, has a positive integer percentage, and all percentages total 95. A tier for
all games correct is mandatory.

## 3. Coupon format and canonicalization

A coupon row is an ASCII string with exactly one character per game, in game order. Each
character is `1` (home win), `X` (draw), or `2` (away win).

A system entry stores one non-empty set of outcomes per game. Each set is canonicalized in
`1`, `X`, `2` order with duplicates removed. The number of paid rows is the product of the set
sizes and must be between 1 and 44,000. Rows are the Cartesian product in game order; each row
has the format above. A system entry is economically identical to submitting all those rows
separately.

For a custodial Phase 3 payment, define:

```text
canonical = event_id + "\n" + join(canonical_game_sets, ",")
coupon_hash = lowercase_hex(SHA-256(UTF-8(canonical)))
memo = "sol-entry-v1:" + coupon_hash
```

`event_id` is the 24-character lowercase MongoDB ObjectID used by the devnet API. The backend
recomputes this value; a client-supplied hash is never trusted.

## 4. Lifecycle and time

The logical lifecycle is:

1. **created** (`draft` in the current API): event exists, but games are not fixed.
2. **open**: games are fixed and chain time is strictly before the entry deadline.
3. **closed**: chain time is at or after the deadline; no coupon may be accepted.
4. **resolved** (`completed` in the current API): every game has an outcome and settlement is
   fixed permanently.
5. **cancelled**: the whole event cannot be resolved and every accepted coupon is refundable.

Games and economic parameters become immutable before the event opens. The deadline is the
event start time (normally the first kickoff). The program uses Solana Clock time, not a
browser or backend clock. A coupon transaction is valid only if program execution occurs
strictly before the deadline.

Resolution is allowed after the deadline and exactly once. Event cancellation is allowed by
the authorized result role before resolution. In Phase 5, if neither resolution nor
cancellation occurs by the specified resolution timeout, anyone can move the event to the
refundable state.

## 5. Outcomes and voided games

Normal outcomes are `1`, `X`, and `2`. A postponed game remains unresolved until played unless
the result authority voids it.

The current devnet void rule uses a reproducible weighted draw one hour after kickoff. Let the
weights be the number of accepted rows selecting `1`, `X`, and `2`. If all are zero, use
`[1,1,1]`. Use the first finalized Solana block at or after kickoff plus one hour:

```text
seed = SHA-256("sol-draw:" + event_id + ":" + zero_based_game_index + ":" + blockhash)
r = big_endian_uint64(seed[0:8]) mod sum(weights)
```

The selected outcome is the first of `1`, `X`, `2` whose cumulative weight is greater than
`r`. Store the slot, blockhash, block time, and weights. A voided game therefore scores as one
ordinary outcome; it does not reduce the coupon price or number of games.

## 6. Payment and acceptance

The cost of an entry is `coupon_price * row_count`, with checked integer multiplication. In
Phase 3, one finalized, successful System Program transaction must transfer exactly that
amount from the authenticated wallet to the configured treasury and contain exactly the
expected memo from section 3. The transaction must be no more than one hour old and its
signature may be accepted once.

In Phase 4, `place_coupon` atomically transfers that amount into the event vault and records
the immutable coupon. There is no interval in which payment succeeds but coupon acceptance can
fail.

## 7. Settlement and payout arithmetic

Let `sales` be the sum of accepted entry payments and `rollover_in` the amount assigned from
older events. For each tier with percentage `p`:

```text
tier_pot = floor(sales * p / 100)
if tier is "all correct": tier_pot += rollover_in
winner_count = number of coupon rows with exactly the tier's correct-pick count
```

If `winner_count > 0`:

```text
per_winner = floor(tier_pot / winner_count)
paid_for_tier = per_winner * winner_count
rounding_remainder = tier_pot - paid_for_tier
```

Every winning row receives `per_winner`. The rounding remainder belongs to the house. If
`winner_count == 0`, the entire tier pot is rollover out.

The house amount is:

```text
house = sales - sum(floor(sales * tier_percent / 100)) + sum(rounding_remainders)
```

Rollover out goes to the all-correct tier of the earliest event that has not started when
resolution is submitted. If no such event exists, it is assigned to the next event created.
The 5% house allocation is not charged again on rollover.

Settlement is deterministic and may occur only once. For each resolved event:

```text
sales + rollover_in = house + paid_to_winners + rollover_out
```

## 8. Claims, cancellation, and refunds

Each winning row may be paid exactly once to the wallet that owns it. Claims do not expire;
unclaimed winnings remain reserved and claimable indefinitely. No administrator, indexer, or
upgrade may redirect or roll over an awarded but unclaimed prize.

If the whole event is cancelled before resolution, each accepted row is refundable at its
original coupon price, exactly once, to its owning wallet. A cancelled event has no winner
payout, house allocation, or rollover from its ticket sales. Incoming rollover remains
reserved for reassignment under the program's cancellation procedure and cannot be withdrawn
by an administrator.

A voided individual game follows section 5 and does not create a refund. A failed or rejected
coupon transaction must transfer no funds. The temporary Phase 3 custodial flow must expose
stranded successful transfers for manual refund; it must never silently treat them as house
funds.

## 9. Required invariants

1. An accepted coupon cannot be altered or omitted.
2. No coupon is accepted at or after the deadline.
3. Total claims plus refunds cannot exceed vault liabilities.
4. A claim or refund cannot be repeated or redirected.
5. The administrator cannot withdraw event funds other than the settled house share, once.
6. Settlement and every draw can be recomputed from public inputs.
7. Claims remain available without administrator or indexer cooperation.
8. Cancellation or timeout provides a permissionless refund path.
9. Every state transition is one-way and resolution occurs at most once.

## 10. Vault events (on-chain program)

An event attached to the on-chain program follows sections 1 to 9 with these differences. The
program is authoritative; MongoDB only mirrors its status.

- **Hidden picks.** `place_coupon` stores `SHA-256(owner || picks || nonce)` and the paid row
  count. Picks are revealed with the nonce from the entry deadline until the reveal deadline,
  which equals the event end time. A coupon that is not revealed in time cannot win; its
  payment stays in the pool and claiming it only closes the account (its rent goes back to the
  owner). A refund for unrevealed coupons would let a player reveal only winning coupons.
- **Picks backup.** Each coupon stores 21 encrypted bytes: the 13 pick masks and the nonce,
  XORed with `SHA-256(key || commitment)[0..21]`, where `key = SHA-256(wallet signature of a
  fixed message)`. The program never reads it. It lets the owner rebuild a coupon on another
  device; a wallet whose message signatures are not deterministic cannot use it.
- **Void games.** A result of `C` scores every pick on that game as correct. There is no
  weighted draw for vault events.
- **Resolution.** The vault names two result signers: the event administrator and the
  configured cosigner, which must not be an administrator. Each approves the same results, or
  approves cancellation, in its own transaction, no earlier than the end time and before the
  resolution timeout (end time + 7 days). A different proposal restarts the count. When both
  agree on results they are locked and the event enters **tallying**, which cannot be cancelled
  or expired. Anyone then counts the coupons in index order, in batches, and the last batch
  settles the event under section 7. When both agree on cancellation the event is cancelled.
- **Timeout.** If neither happens before the resolution timeout, anyone can cancel the event.
- **House share.** After resolution the event administrator may withdraw exactly the settled
  house amount, once. Winnings, rollover and refunds cannot be reached by this instruction.
- **Rollover.** Anyone may forward an event's rollover out to the earliest scheduled vault
  event of the same administrator that has not started, or park it in the program escrow when
  none exists; the next event created by that administrator takes it.
- **Capacity.** One administrator wallet can have at most 16 vault events that are not yet
  resolved or cancelled. A coupon holds at most 44,000 rows, as in section 3.
- **Treasury payments.** An event with a vault never turns a treasury transfer into an entry;
  such a transfer is recorded for manual refund.
