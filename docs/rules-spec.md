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

- 13 correct: 45% of ticket sales
- 12 correct: 25%
- 11 correct: 15%
- 10 correct: 5%

A special event explicitly stores its price and tiers. Each tier identifies a distinct number
of correct picks, has a positive integer percentage, and all percentages total 90. A tier for
all games correct is mandatory. The remaining 10% of ticket sales is the house remainder:
settlement moves 5% of ticket sales into the league vault (section 11) and leaves the rest,
including rounding dust, as the house share.

An event created before this rule, whose stored tiers total 95, keeps that settlement. It does
not fund the league vault. A league event (section 11) stores tiers that total 100.

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

The house remainder is:

```text
remainder = sales - sum(floor(sales * tier_percent / 100)) + sum(rounding_remainders)
```

When the tiers total 90, the league cut is `floor(sales * 5 / 100)` and the house amount is
`remainder - league_cut`. When the tiers total 95, the league cut is zero and the house amount
is the whole remainder. Rollover is not taxed again.

Rollover out goes to the all-correct tier of the earliest event of the same kind that has not
started when resolution is submitted. If no such event exists, it is assigned to the next event
of that kind. A SOL event and a league event never accept each other's rollover.

Settlement is deterministic and may occur only once. For each resolved event:

```text
sales + rollover_in = house + league_cut + paid_to_winners + rollover_out
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
5. The administrator cannot withdraw event funds other than the settled house share, once,
   and cannot withdraw from the league vault at all.
6. Settlement and every draw can be recomputed from public inputs.
7. Claims remain available without administrator or indexer cooperation.
8. Cancellation or timeout provides a permissionless refund path.
9. Every state transition is one-way and resolution occurs at most once.

## 10. Vault events (on-chain program)

An event attached to the on-chain program follows sections 1 to 9 with these differences. The
program is authoritative; MongoDB only mirrors its status.

- **Hidden picks.** `place_coupon` stores `SHA-256(owner || picks || nonce)` and the paid row
  count. The player signs only that entry. The picks are time-lock encrypted until the entry
  deadline and stored by the backend. After entries close, anyone who knows the picks can
  publish them: the owner does not sign the reveal, and a wrong set of picks fails the
  commitment check. The keeper opens the time lock and sends that transaction, so the player
  does not have to be online. Reveals are only accepted from the entry deadline until the
  reveal deadline, which equals the event end time. A coupon that is not revealed in time
  cannot win; its payment stays in the pool and claiming it only closes the account (its rent
  goes back to the owner). A refund for unrevealed coupons would let a player reveal only
  winning coupons.
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
- **League funding.** Resolving a vault event whose tiers total 90 transfers the league cut
  into the league vault in the same instruction that settles the event.

## 11. League vault and RAD

RAD is a per-wallet counter held by the program. It is not an SPL token. One RAD is credited
for each row of a SOL entry at the moment the entry is accepted. Spending RAD is only possible
by entering a league event. Cancelling a SOL event does not debit RAD already credited.

The league vault is a single program account. Its liquid balance above rent is the collected
league cuts plus any parked league jackpot. The jackpot portion is stored as an integer on the
account so it is not spread across every prize tier. Nobody can withdraw this account except
by creating a league event, which moves the whole liquid balance into that event's vault. The
jackpot integer becomes that event's `rollover_in`. The rest becomes its `sales`.

A league event:

- costs a stored RAD price per row (one, unless the admin sets another price)
- accepts entry only by debiting that wallet's RAD
- does not credit RAD for its own rows
- uses tiers that total 100, so there is no second house cut; integer dust left after paying
  winners is added to rollover out
- parks an unwon rollover into the league vault's jackpot integer, for the next league event
- on cancellation, returns the swept SOL to the league vault and refunds each row's RAD

A league event's prize is SOL that earlier players funded. Entry does not add SOL to it.

## 12. Color reduction

A vault coupon can be built with up to four colors (purple, yellow, red, and green). A color is a set of signs. A sign is one outcome of one game, and the same sign may belong to more than one color. Each color has a minimum and a maximum. A row is kept only when, for every color, the number of its signs on that row is inside that range.

A colored entry is one slip. Its commitment covers the 13 pick masks, the reveal nonce, and the list of colors. Each color is 10 bytes: the sign bits (game `g` outcome `k` is bit `3g + k`, in the order `1`, `X`, `2`) as a little-endian u64, then the minimum, then the maximum. With colors the commitment is `sha256(owner, picks, nonce, "colors-v1", colors)` and the reveal leaf ends with `"colors-v1"` and the same bytes. A slip with no colors hashes exactly as before colors existed.

The row count paid at entry is the number of kept rows. The frame may be larger than 44,000 rows, but the kept rows may not. The program counts the kept rows without listing them, using a table of states: one more than each color's effective maximum, multiplied over the colors. A color whose minimum is 0 and whose maximum covers all of its signs adds no states. A table over 64 states is refused.

Reveal checks the commitment and the colors. It rejects a color with no signs, a sign outside the frame, a minimum above its maximum, a maximum above the game count, more than four colors, a table over the cap, and a kept-row count that differs from the paid count. A rejected coupon stays unrevealed: its stake stays in the pool and it cannot win. Scoring and claims count only the kept rows by number of correct picks, never the raw frame. A void game still counts every pick correct.

Worked example: six games marked yellow on `1`, minimum 3 and maximum 5, and every other selected sign left as it is. The other two games of an eight-game frame are `1`, `X`, and `2`. The frame is 6,561 rows and the kept rows are 2,088, on one coupon with one signature.

Entries placed before this rule were split in the browser into ordinary system coupons, one `place_slip` per pattern, from a group saved in MongoDB with the picks sealed by the wallet's coupon key. A group saved that way can still be resumed, and each of its coupons is revealed, scored, claimed, and refunded by the rules for one slip.

## 13. Event size and the canopy

An event takes at most 1,000,000 coupons, or the lower maximum the admin set when creating it. The program refuses the next coupon, and the event page says the event is full. The limit counts coupons, not rows.

The coupons of a vault event are the leaves of two hash trees of depth 20: one for entries and one for revealed picks. A coupon proves its place with the 20 hashes on its path. An event created from this rule on has a canopy: an account, owned by the program, that stores every node from level 10 up to the root of both trees, and one paid bit per coupon. A transaction for one coupon sends only the 10 hashes inside the coupon's own group of 1,024. The program reads the other 10 from the canopy and checks the same depth-20 root. Placing a coupon sends no hashes at all. This keeps place, reveal, count, claim, and refund for one coupon inside one Solana transaction up to coupon 1,000,000.

The canopy changes how a coupon is proved. It does not change what a coupon pays or wins. An event created from the signed-totals rule on settles from a signed winner list, as section 14 describes. An earlier event is still counted one coupon per transaction, in coupon order, and every amount in sections 4 to 12 is computed as before.

- **Storage fee.** The canopy grows by 256 bytes for every 1,024 coupons. Each coupon pays 1,740 lamports toward the rent of those bytes, in SOL, on SOL events and league events alike. The fee goes to the canopy account. It is not part of the pool, it is not a house fee, and it is not returned when a coupon is refunded.
- **Reveal.** A reveal is accepted only with a proof that the coupon was entered with that owner, row count, and commitment, and only while that coupon's reveal leaf is empty.
- **Paid once.** A claim or a refund sets the coupon's paid bit. A second claim or refund of the same coupon is refused.
- **Earlier events.** An event created before this rule has no canopy. It keeps the instructions and the full proofs it was created with, and it is not converted.

## 14. Signed totals and the challenge window

An event created from this rule on does not count every coupon on chain before it can pay. After the result signers lock the results, the keeper counts the archive with the same kept-row count as the program and posts the number of rows at each score. The result signers sign that exact list. The same threshold as a result applies. A list that differs in any value, or a different poster, is a different proposal and the approvals start again.

The poster locks 3 SOL with the totals. That bond moves into the event vault on the signature that reaches the threshold, and only when the poster signs that transaction. The 6-hour window then starts. It is 54,000 slots from the slot in which the totals locked, so a halt of the cluster does not shorten it. Wall-clock time does not end it.

During the window, claims are closed. Anyone can challenge by locking a bond. The bond is 10% of the player pot, clamped to the minimum and maximum stored on the event. The player pot is the part of sales the tiers take, plus rollover in: sales times the sum of the tier percents, divided by 100, plus rollover. The limits are whole SOL from 1 to 100, and the minimum cannot be above the maximum. The defaults are 10 and 50. A pot of 100 SOL or less locks 10 SOL, a pot of 250 SOL locks 25 SOL, and a pot of 500 SOL or more locks 50 SOL. An event created before the field existed uses 10 and 50. A minimum under 10 SOL does not cover the fees of a recount at 1,000,000 coupons; the page warns, and the program still accepts it.

Only the first challenge counts. A later one is refused and moves no money. Only the challenger's signing key can receive the bonds.

When the window ends with no challenge, anyone can finalize. That sets the payout per row from the signed totals, runs the settlement in section 7, including the house share, the league 5%, and rollover, opens claims, and returns the poster's bond.

A challenge starts the recount. The on-chain count runs again from the first coupon, one coupon per transaction, in order. Anyone can send those scores. Claims stay closed until it finishes. Each score in the recount records a fee of 5,000 lamports for its fee payer. At 1,000,000 coupons that is 5 SOL. The recorded fee is that fixed amount, so a caller cannot choose a larger one.

If the recount matches the signed totals, those totals stand. The challenger's bond first pays the recorded fees back, up to the bond, and the remainder is added to the pool before settlement. The poster's bond is returned. If the recount differs, the recount totals are used, the event shows that the signed totals were rejected, and the challenger receives their bond plus what remains of the poster's bond after the recorded fees. Fees above the 3 SOL poster bond stay with whoever sent the scores.

A claim adds the rows it pays to a per-tier counter and is refused past the signed total for that tier, or past the recount total when the signed list was rejected. The tier pot is the ceiling. A total that is too high makes every payout smaller and leaves the rest in the vault.

The totals for scores 0 through 13 must add up to the kept rows of every revealed coupon. Each reveal adds that coupon's kept rows to a running sum on the canopy. Totals that add up to anything else are rejected. Scores above the event's game count must be zero. Rows can still be moved between scores, and the challenge is what catches that.

The event admin, or a result signer, can start the on-chain count as soon as the results are locked. Starting it closes the signed-totals path. If no totals are locked within 24 hours of the result approval, anyone can start that count with no bonds. The 24 hours are unix time. An event cannot stay unpaid because nobody signed.

During the window the result signers can cancel with the usual threshold. Every stake is then refunded and the poster bond is returned. A wrong result can still be undone before a claim is paid. Cancelling is not available merely because the results are locked and the totals are not yet posted.

Events created before this rule, and events already counting when the program is upgraded, keep the earlier count. The new path applies to events created after the upgrade.
