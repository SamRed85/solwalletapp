use anchor_lang::prelude::*;
use anchor_lang::system_program;

mod math;

declare_id!("AaXABVJE79W6za8ntvto8wvtP6Kqgs5kQgHCS2uoHoaX");

const EVENT_SEED: &[u8] = b"event";
const VAULT_SEED: &[u8] = b"vault";
const COUPON_SEED: &[u8] = b"coupon";
const ESCROW_SEED: &[u8] = b"rollover-escrow";
const SCHEDULE_SEED: &[u8] = b"schedule";
const SIGNERS_SEED: &[u8] = b"signers";
const GATE_SEED: &[u8] = b"gate";
const UPGRADE_SEED: &[u8] = b"upgrade-council";
const SLIP_SEED: &[u8] = b"slips";
const SLIP_DEPTH: usize = 20;
#[cfg(not(feature = "devnet-testing"))]
const UPGRADE_DELAY: i64 = 24 * 60 * 60;
#[cfg(feature = "devnet-testing")]
const UPGRADE_DELAY: i64 = 5 * 60;
const MAX_SCHEDULE: usize = 16;
const MAX_SIGNERS: usize = 5;
pub const SEALED_LEN: usize = math::MAX_GAMES + 8;

const PROPOSAL_NONE: u8 = 0;
const PROPOSAL_RESOLVE: u8 = 1;
const PROPOSAL_CANCEL: u8 = 2;

#[program]
pub mod predictions {
    use super::*;

    pub fn create_event(ctx: Context<CreateEvent>, args: CreateEventArgs) -> Result<()> {
        require!(
            (math::MAX_GAMES as u8) >= args.game_count && args.game_count >= 7,
            PredError::BadEvent
        );
        require!(args.price > 0 && args.price <= 10_000_000_000, PredError::BadEvent);
        let now = Clock::get()?.unix_timestamp;
        require!(args.deadline > now, PredError::BadEvent);
        require!(args.reveal_deadline > args.deadline, PredError::BadEvent);
        require!(args.end_time >= args.reveal_deadline, PredError::BadEvent);
        require!(args.resolve_timeout > args.end_time, PredError::BadEvent);
        require!(args.threshold >= 2, PredError::BadSigners);
        require!(
            args.signers.len() >= args.threshold as usize && args.signers.len() <= MAX_SIGNERS,
            PredError::BadSigners
        );
        for (index, signer) in args.signers.iter().enumerate() {
            require!(args.signers[..index].iter().all(|prior| prior != signer), PredError::BadSigners);
        }
        let tiers: Vec<math::Tier> = args
            .tiers
            .iter()
            .map(|tier| math::Tier { correct: tier.correct, percent: tier.percent })
            .collect();
        math::validate_tiers(&tiers, args.game_count).map_err(|_| error!(PredError::BadTiers))?;

        let event = &mut ctx.accounts.event;
        event.admin = ctx.accounts.admin.key();
        event.nonce = args.nonce;
        event.game_count = args.game_count;
        event.price = args.price;
        event.tier_count = args.tiers.len() as u8;
        event.tiers = [TierArg::default(); math::MAX_GAMES];
        event.tiers[..args.tiers.len()].copy_from_slice(&args.tiers);
        event.deadline = args.deadline;
        event.reveal_deadline = args.reveal_deadline;
        event.end_time = args.end_time;
        event.resolve_timeout = args.resolve_timeout;
        event.status = EventStatus::Open;
        event.bump = ctx.bumps.event;
        event.vault_bump = ctx.bumps.vault;
        event.rent_reserve = ctx.accounts.vault.to_account_info().lamports();
        if ctx.accounts.escrow.rent_reserve == 0 {
            ctx.accounts.escrow.rent_reserve = ctx.accounts.escrow.to_account_info().lamports();
        }

        let event_key = event.key();
        let signers = &mut ctx.accounts.signers;
        signers.event = event_key;
        signers.threshold = args.threshold;
        signers.count = args.signers.len() as u8;
        signers.signers = [Pubkey::default(); MAX_SIGNERS];
        signers.signers[..args.signers.len()].copy_from_slice(&args.signers);
        signers.bump = ctx.bumps.signers;
        if ctx.accounts.gate.admin == Pubkey::default() {
            ctx.accounts.gate.admin = ctx.accounts.admin.key();
            ctx.accounts.gate.bump = ctx.bumps.gate;
        }
        schedule_insert(&mut ctx.accounts.schedule, event_key, args.deadline)?;
        // Parked rollover belongs to the earliest event that has not started.
        if earliest_future(&ctx.accounts.schedule, now) == Some(event_key) {
            let pending = ctx.accounts.escrow.pending;
            if pending > 0 {
                transfer_from_vault(
                    &ctx.accounts.escrow.to_account_info(),
                    &ctx.accounts.vault.to_account_info(),
                    pending,
                    ctx.accounts.escrow.rent_reserve,
                )?;
                event.rollover_in = pending;
                ctx.accounts.escrow.pending = 0;
            }
        }
        Ok(())
    }

    /// `sealed` is the owner's encrypted copy of the picks and reveal nonce. The program never
    /// reads it; it lets the owner recover a coupon from the chain on a new device.
    pub fn place_coupon(ctx: Context<PlaceCoupon>, commitment: [u8; 32], combinations: u32, sealed: [u8; SEALED_LEN]) -> Result<()> {
        require!(!ctx.accounts.gate.paused, PredError::Paused);
        let event = &mut ctx.accounts.event;
        require!(event.status == EventStatus::Open, PredError::BadStatus);
        require!(Clock::get()?.unix_timestamp < event.deadline, PredError::DeadlinePassed);
        require!(combinations >= 1 && combinations <= math::MAX_COMBINATIONS, PredError::BadPicks);
        let cost = event.price.checked_mul(u64::from(combinations)).ok_or(error!(PredError::Overflow))?;

        system_program::transfer(
            CpiContext::new(
                ctx.accounts.system_program.to_account_info(),
                system_program::Transfer {
                    from: ctx.accounts.player.to_account_info(),
                    to: ctx.accounts.vault.to_account_info(),
                },
            ),
            cost,
        )?;

        let coupon = &mut ctx.accounts.coupon;
        coupon.event = event.key();
        coupon.owner = ctx.accounts.player.key();
        coupon.index = event.coupon_count;
        coupon.game_count = event.game_count;
        coupon.commitment = commitment;
        coupon.revealed = false;
        coupon.combinations = combinations;
        coupon.lamports_paid = cost;
        coupon.bump = ctx.bumps.coupon;
        coupon.sealed = sealed;

        event.coupon_count = event.coupon_count.checked_add(1).ok_or(error!(PredError::Overflow))?;
        event.sales = event.sales.checked_add(cost).ok_or(error!(PredError::Overflow))?;
        Ok(())
    }

    /// Reveals are open from the entry deadline until the reveal deadline (normally the end
    /// time). Anyone can publish the picks then. The commitment stored at entry rejects any
    /// other picks, and the app keeps the picks time-lock encrypted until the deadline, so
    /// the publisher cannot read them early. A coupon that is never revealed keeps its stake
    /// in the pool and cannot win.
    pub fn reveal_coupon(ctx: Context<RevealCoupon>, picks: [u8; math::MAX_GAMES], nonce: u64) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Open, PredError::BadStatus);
        let now = Clock::get()?.unix_timestamp;
        require!(now >= event.deadline && now < event.reveal_deadline, PredError::RevealClosed);
        let coupon = &mut ctx.accounts.coupon;
        require!(!coupon.revealed, PredError::BadStatus);
        let game_count = coupon.game_count as usize;
        require!(picks[game_count..].iter().all(|mask| *mask == 0), PredError::BadPicks);
        let combinations = math::combination_count(&picks[..game_count]).map_err(|_| error!(PredError::BadPicks))?;
        require!(combinations == coupon.combinations, PredError::BadPicks);
        require!(
            coupon_commitment(&coupon.owner, &picks, nonce) == coupon.commitment,
            PredError::BadCommitment
        );
        coupon.picks = picks;
        coupon.revealed = true;
        Ok(())
    }

    /// Places one coupon without creating a coupon account. The UI submits one coupon,
    /// and this instruction refuses a batch. The stake is one transfer. The coupon is one
    /// log line and one leaf in the slip Merkle root. Picks are not in that line. The index
    /// is the next free one, so an earlier index cannot be submitted again.
    pub fn place_slip(ctx: Context<PlaceSlip>, slip: SlipArg, proof: Vec<[u8; 32]>) -> Result<()> {
        require!(!ctx.accounts.gate.paused, PredError::Paused);
        require!(proof.len() == SLIP_DEPTH, PredError::BadPicks);
        let event = &mut ctx.accounts.event;
        require!(event.status == EventStatus::Open, PredError::BadStatus);
        require!(Clock::get()?.unix_timestamp < event.deadline, PredError::DeadlinePassed);
        let book = &mut ctx.accounts.slips;
        if book.event == Pubkey::default() {
            book.event = event.key();
            book.bump = ctx.bumps.slips;
            book.entry_hash = slip_empty_root();
            book.reveal_hash = slip_empty_root();
            book.scored_root = slip_empty_root();
            book.claimed_root = slip_empty_root();
        }
        require!(book.event == event.key(), PredError::WrongEvent);
        require!(book.count == event.coupon_count, PredError::BadStatus);
        require!(slip.combinations >= 1 && slip.combinations <= math::MAX_COMBINATIONS, PredError::BadPicks);
        let index = book.count;
        require!(index < (1u32 << SLIP_DEPTH), PredError::TooManyCoupons);

        let owner = ctx.accounts.player.key();
        let leaf = slip_leaf(index, &owner, slip.combinations, &slip.commitment);
        let empty_root = slip_fold(index, &slip_zero(0), &proof);
        require!(empty_root == book.entry_hash, PredError::BadCommitment);
        book.entry_hash = slip_fold(index, &leaf, &proof);
        msg!("slip-v1:{}:{}:{}:{}:{}", event.key(), index, owner, slip.combinations, hex_encode(&slip.commitment));
        book.count = book.count.checked_add(1).ok_or(error!(PredError::Overflow))?;
        let cost = event.price.checked_mul(u64::from(slip.combinations)).ok_or(error!(PredError::Overflow))?;

        system_program::transfer(
            CpiContext::new(
                ctx.accounts.system_program.to_account_info(),
                system_program::Transfer {
                    from: ctx.accounts.player.to_account_info(),
                    to: ctx.accounts.vault.to_account_info(),
                },
            ),
            cost,
        )?;
        event.coupon_count = book.count;
        event.sales = event.sales.checked_add(cost).ok_or(error!(PredError::Overflow))?;
        Ok(())
    }

    /// Publishes the picks for one slip. The commitment is checked against those picks.
    /// The reveal leaf starts empty, so the same index cannot be published twice.
    pub fn reveal_slip(ctx: Context<RevealSlip>, reveal: RevealArg, proof: Vec<[u8; 32]>) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Open, PredError::BadStatus);
        let now = Clock::get()?.unix_timestamp;
        require!(now >= event.deadline && now < event.reveal_deadline, PredError::RevealClosed);
        let book = &mut ctx.accounts.slips;
        require!(book.event == event.key(), PredError::WrongEvent);
        check_reveal(&reveal, event.game_count)?;
        require!(reveal.index < book.count, PredError::MissingCoupon);
        let leaf = slip_reveal_leaf(reveal.index, &reveal.owner, reveal.combinations, &reveal.commitment, &reveal.picks);
        write_leaf(&mut book.reveal_hash, reveal.index, &proof, &leaf)?;
        msg!(
            "reveal-v1:{}:{}:{}:{}:{}:{}",
            event.key(),
            reveal.index,
            reveal.owner,
            reveal.combinations,
            hex_encode(&reveal.commitment),
            hex_encode(&reveal.picks)
        );
        book.reveal_count = book.reveal_count.checked_add(1).ok_or(error!(PredError::Overflow))?;
        Ok(())
    }

    /// Scores one slip from its entry and reveal proofs. An empty reveal leaf scores nothing.
    /// The last scored slip settles the event. A leaf that is not in the root is rejected.
    pub fn score_slip(
        ctx: Context<ScoreSlip>,
        reveal: RevealArg,
        revealed: bool,
        entry_proof: Vec<[u8; 32]>,
        reveal_proof: Vec<[u8; 32]>,
        scored_proof: Vec<[u8; 32]>,
    ) -> Result<()> {
        require!(ctx.accounts.event.status == EventStatus::Tallying, PredError::BadStatus);
        require!(reveal.index == ctx.accounts.event.tallied, PredError::CouponOrder);
        let book = &mut ctx.accounts.slips;
        require!(book.event == ctx.accounts.event.key(), PredError::WrongEvent);
        require!(reveal.index < book.count, PredError::MissingCoupon);
        require_leaf(&book.entry_hash, reveal.index, &slip_leaf(reveal.index, &reveal.owner, reveal.combinations, &reveal.commitment), &entry_proof)?;
        let event_key_games = ctx.accounts.event.game_count;
        let results = ctx.accounts.event.results;
        if revealed {
            let leaf = slip_reveal_leaf(reveal.index, &reveal.owner, reveal.combinations, &reveal.commitment, &reveal.picks);
            require_leaf(&book.reveal_hash, reveal.index, &leaf, &reveal_proof)?;
            let counts = math::correct_counts(&reveal.picks[..event_key_games as usize], &results[..event_key_games as usize]).map_err(|_| error!(PredError::BadResults))?;
            for (score, winner) in ctx.accounts.event.winners.iter_mut().enumerate() {
                *winner = winner.checked_add(u64::from(counts[score])).ok_or(error!(PredError::Overflow))?;
            }
        } else {
            prove_empty(&book.reveal_hash, reveal.index, &reveal_proof)?;
        }
        write_leaf(&mut book.scored_root, reveal.index, &scored_proof, &slip_mark())?;
        msg!("score-v1:{}:{}", ctx.accounts.event.key(), reveal.index);
        let event = &mut ctx.accounts.event;
        event.tallied = event.tallied.checked_add(1).ok_or(error!(PredError::Overflow))?;
        if event.tallied < event.coupon_count {
            return Ok(());
        }
        apply_settlement(event, ctx.accounts.vault.to_account_info().lamports())?;
        schedule_remove(&mut ctx.accounts.schedule, event.key())?;
        Ok(())
    }

    /// Pays one scored slip from the vault. The owner signs, the proofs must match the roots,
    /// and a second claim sees a leaf that is no longer empty. There is no account deposit and no draw ticket.
    pub fn claim_slip(
        ctx: Context<ClaimSlip>,
        reveal: RevealArg,
        entry_proof: Vec<[u8; 32]>,
        reveal_proof: Vec<[u8; 32]>,
        claimed_proof: Vec<[u8; 32]>,
    ) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Resolved, PredError::BadStatus);
        require!(ctx.accounts.owner.key() == reveal.owner, PredError::WrongWinner);
        let book = &mut ctx.accounts.slips;
        require!(book.event == event.key(), PredError::WrongEvent);
        require!(reveal.index < book.count, PredError::MissingCoupon);
        check_reveal(&reveal, event.game_count)?;
        require_leaf(&book.entry_hash, reveal.index, &slip_leaf(reveal.index, &reveal.owner, reveal.combinations, &reveal.commitment), &entry_proof)?;
        let leaf = slip_reveal_leaf(reveal.index, &reveal.owner, reveal.combinations, &reveal.commitment, &reveal.picks);
        require_leaf(&book.reveal_hash, reveal.index, &leaf, &reveal_proof)?;
        let game_count = event.game_count as usize;
        let counts = math::correct_counts(&reveal.picks[..game_count], &event.results[..game_count]).map_err(|_| error!(PredError::BadResults))?;
        let mut amount = 0u64;
        for tier in &event.tiers[..event.tier_count as usize] {
            let rows = u64::from(counts[tier.correct as usize]);
            let per_winner = event.per_winner[tier.correct as usize - 1];
            amount = amount.checked_add(per_winner.checked_mul(rows).ok_or(error!(PredError::Overflow))?).ok_or(error!(PredError::Overflow))?;
        }
        require!(amount > 0, PredError::NothingToClaim);
        write_leaf(&mut book.claimed_root, reveal.index, &claimed_proof, &slip_mark())?;
        msg!("claim-v1:{}:{}", event.key(), reveal.index);
        transfer_from_vault(&ctx.accounts.vault.to_account_info(), &ctx.accounts.owner.to_account_info(), amount, event.rent_reserve)?;
        Ok(())
    }

    /// Returns one cancelled slip's stake to its owner. The entry proof identifies the coupon.
    /// The refund proof is empty the first time and fails once that leaf has been marked.
    pub fn refund_slip(
        ctx: Context<RefundSlip>,
        index: u32,
        combinations: u32,
        commitment: [u8; 32],
        entry_proof: Vec<[u8; 32]>,
        refund_proof: Vec<[u8; 32]>,
    ) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Cancelled, PredError::BadStatus);
        let owner = ctx.accounts.owner.key();
        let book = &mut ctx.accounts.slips;
        require!(book.event == event.key(), PredError::WrongEvent);
        require!(index < book.count, PredError::MissingCoupon);
        require!(combinations >= 1, PredError::BadPicks);
        let leaf = slip_leaf(index, &owner, combinations, &commitment);
        require_leaf(&book.entry_hash, index, &leaf, &entry_proof)?;
        let amount = event.price.checked_mul(u64::from(combinations)).ok_or(error!(PredError::Overflow))?;
        write_leaf(&mut book.claimed_root, index, &refund_proof, &slip_mark())?;
        msg!("refund-v1:{}:{}", event.key(), index);
        transfer_from_vault(&ctx.accounts.vault.to_account_info(), &ctx.accounts.owner.to_account_info(), amount, event.rent_reserve)?;
        Ok(())
    }

    /// Each result signer approves the same results in its own transaction. When the
    /// threshold agrees, the results are locked and the event moves to tallying.
    /// `C` marks a void game: every pick on it counts as correct.
    pub fn approve_resolution(ctx: Context<Approve>, results: Vec<u8>) -> Result<()> {
        let event = &ctx.accounts.event;
        let game_count = event.game_count as usize;
        require!(results.len() == game_count, PredError::BadResults);
        require!(results.iter().all(|result| math::result_mask(*result).is_some()), PredError::BadResults);
        let now = Clock::get()?.unix_timestamp;
        require!(now >= event.reveal_deadline && now >= event.end_time, PredError::TooEarly);
        let mut proposal = [0u8; math::MAX_GAMES];
        proposal[..game_count].copy_from_slice(&results);
        if record_approval(&mut ctx.accounts.event, &ctx.accounts.signers, &ctx.accounts.signer.key(), PROPOSAL_RESOLVE, proposal)? {
            let event = &mut ctx.accounts.event;
            event.results = proposal;
            event.status = EventStatus::Tallying;
            event.tallied = 0;
            event.winners = [0; math::MAX_GAMES + 1];
        }
        Ok(())
    }

    /// Cancelling needs the same threshold as resolving, with one approval per transaction.
    pub fn approve_cancel(ctx: Context<Approve>) -> Result<()> {
        if record_approval(
            &mut ctx.accounts.event,
            &ctx.accounts.signers,
            &ctx.accounts.signer.key(),
            PROPOSAL_CANCEL,
            [0; math::MAX_GAMES],
        )? {
            close_open_event(&mut ctx.accounts.event)?;
            schedule_remove(&mut ctx.accounts.schedule, ctx.accounts.event.key())?;
        }
        Ok(())
    }

    /// Counts winning rows for the next coupons in index order. Anyone can call it, in as
    /// many transactions as needed; the last batch settles the event. Locked results cannot
    /// be replaced or expired, and every coupon account exists until settlement.
    pub fn tally<'info>(ctx: Context<'_, '_, 'info, 'info, Tally<'info>>) -> Result<()> {
        require!(ctx.accounts.event.status == EventStatus::Tallying, PredError::BadStatus);
        let event_key = ctx.accounts.event.key();
        let game_count = ctx.accounts.event.game_count as usize;
        let results = ctx.accounts.event.results;
        let mut tallied = ctx.accounts.event.tallied;
        let mut winners = ctx.accounts.event.winners;
        for account in ctx.remaining_accounts {
            let coupon = Account::<Coupon>::try_from(account)?;
            require!(coupon.event == event_key, PredError::WrongEvent);
            require!(coupon.index == tallied, PredError::CouponOrder);
            if coupon.revealed {
                let counts = math::correct_counts(&coupon.picks[..game_count], &results[..game_count]).map_err(|_| error!(PredError::BadResults))?;
                for (score, winner) in winners.iter_mut().enumerate() {
                    *winner = winner.checked_add(u64::from(counts[score])).ok_or(error!(PredError::Overflow))?;
                }
            }
            tallied = tallied.checked_add(1).ok_or(error!(PredError::Overflow))?;
        }

        let event = &mut ctx.accounts.event;
        event.tallied = tallied;
        event.winners = winners;
        if tallied < event.coupon_count {
            return Ok(());
        }
        apply_settlement(event, ctx.accounts.vault.to_account_info().lamports())?;
        schedule_remove(&mut ctx.accounts.schedule, event.key())?;
        Ok(())
    }

    pub fn set_pause(ctx: Context<SetPause>, paused: bool) -> Result<()> {
        ctx.accounts.gate.paused = paused;
        Ok(())
    }

    pub fn expire_event(ctx: Context<ExpireEvent>) -> Result<()> {
        require!(ctx.accounts.event.status == EventStatus::Open, PredError::BadStatus);
        require!(Clock::get()?.unix_timestamp >= ctx.accounts.event.resolve_timeout, PredError::TooEarly);
        close_open_event(&mut ctx.accounts.event)?;
        schedule_remove(&mut ctx.accounts.schedule, ctx.accounts.event.key())?;
        Ok(())
    }

    /// Pays a winning coupon from the event vault and returns its account rent to the owner.
    /// A coupon with no prize returns only that rent. The stake stays in the pool. There is no draw.
    pub fn claim(ctx: Context<Claim>) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Resolved, PredError::BadStatus);
        let amount = payout_for(&ctx.accounts.coupon, event)?;
        if amount > 0 {
            transfer_from_vault(
                &ctx.accounts.vault.to_account_info(),
                &ctx.accounts.owner.to_account_info(),
                amount,
                event.rent_reserve,
            )?;
        }
        Ok(())
    }

    /// Closes a resolved coupon that won nothing. The keeper calls this so a loser does not
    /// have to sign. The account rent goes back to the owner, and the stake stays in the pool.
    pub fn close_lost(ctx: Context<CloseLost>) -> Result<()> {
        require!(ctx.accounts.event.status == EventStatus::Resolved, PredError::BadStatus);
        require!(payout_for(&ctx.accounts.coupon, &ctx.accounts.event)? == 0, PredError::BadStatus);
        Ok(())
    }

    pub fn refund(ctx: Context<Refund>) -> Result<()> {
        require!(ctx.accounts.event.status == EventStatus::Cancelled, PredError::BadStatus);
        let amount = ctx.accounts.coupon.lamports_paid;
        transfer_from_vault(
            &ctx.accounts.vault.to_account_info(),
            &ctx.accounts.owner.to_account_info(),
            amount,
            ctx.accounts.event.rent_reserve,
        )?;
        Ok(())
    }

    /// The event admin may take exactly the settled house share, once. Winnings, rollover and
    /// refunds stay in the vault, so this cannot reach player money.
    pub fn withdraw_house(ctx: Context<WithdrawHouse>) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Resolved, PredError::BadStatus);
        require!(!event.house_withdrawn, PredError::AlreadyWithdrawn);
        require!(event.house > 0, PredError::NothingToClaim);
        transfer_from_vault(
            &ctx.accounts.vault.to_account_info(),
            &ctx.accounts.admin.to_account_info(),
            event.house,
            event.rent_reserve,
        )?;
        ctx.accounts.event.house_withdrawn = true;
        Ok(())
    }

    pub fn forward_rollover(ctx: Context<ForwardRollover>) -> Result<()> {
        let amount = ctx.accounts.source.rollover_out;
        require!(
            ctx.accounts.source.status == EventStatus::Resolved || ctx.accounts.source.status == EventStatus::Cancelled,
            PredError::BadStatus
        );
        require!(!ctx.accounts.source.rollover_forwarded, PredError::AlreadyForwarded);
        require!(amount > 0, PredError::NothingToForward);
        require!(ctx.accounts.destination.key() != ctx.accounts.source.key(), PredError::WrongEvent);
        let now = Clock::get()?.unix_timestamp;
        let earliest = earliest_future(&ctx.accounts.schedule, now).ok_or(error!(PredError::DestinationStarted))?;
        require!(ctx.accounts.destination.key() == earliest, PredError::NotEarliestEvent);
        require!(ctx.accounts.destination.status == EventStatus::Open, PredError::BadStatus);
        require!(now < ctx.accounts.destination.deadline, PredError::DestinationStarted);
        transfer_from_vault(
            &ctx.accounts.source_vault.to_account_info(),
            &ctx.accounts.destination_vault.to_account_info(),
            amount,
            ctx.accounts.source.rent_reserve,
        )?;
        ctx.accounts.destination.rollover_in = ctx.accounts.destination.rollover_in.checked_add(amount).ok_or(error!(PredError::Overflow))?;
        ctx.accounts.source.rollover_forwarded = true;
        Ok(())
    }

    pub fn park_rollover(ctx: Context<ParkRollover>) -> Result<()> {
        let amount = ctx.accounts.source.rollover_out;
        require!(
            ctx.accounts.source.status == EventStatus::Resolved || ctx.accounts.source.status == EventStatus::Cancelled,
            PredError::BadStatus
        );
        require!(!ctx.accounts.source.rollover_forwarded, PredError::AlreadyForwarded);
        require!(amount > 0, PredError::NothingToForward);
        require!(
            earliest_future(&ctx.accounts.schedule, Clock::get()?.unix_timestamp).is_none(),
            PredError::FutureEventExists
        );
        transfer_from_vault(
            &ctx.accounts.source_vault.to_account_info(),
            &ctx.accounts.escrow.to_account_info(),
            amount,
            ctx.accounts.source.rent_reserve,
        )?;
        ctx.accounts.escrow.pending = ctx.accounts.escrow.pending.checked_add(amount).ok_or(error!(PredError::Overflow))?;
        ctx.accounts.source.rollover_forwarded = true;
        Ok(())
    }

    pub fn initialize_upgrade(ctx: Context<InitializeUpgrade>, threshold: u8, signers: Vec<Pubkey>) -> Result<()> {
        require_keys_eq!(ctx.accounts.payer.key(), pubkey!("955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun"), PredError::BadSigners);
        require!(threshold >= 2, PredError::BadSigners);
        require!(signers.len() >= threshold as usize && signers.len() <= MAX_SIGNERS, PredError::BadSigners);
        for (index, signer) in signers.iter().enumerate() {
            require!(signers[..index].iter().all(|prior| prior != signer), PredError::BadSigners);
        }
        let council = &mut ctx.accounts.council;
        council.threshold = threshold;
        council.count = signers.len() as u8;
        council.signers = [Pubkey::default(); MAX_SIGNERS];
        council.signers[..signers.len()].copy_from_slice(&signers);
        council.bump = ctx.bumps.council;
        council.proposal_eta = 0;
        Ok(())
    }

    pub fn schedule_upgrade<'info>(ctx: Context<'_, '_, 'info, 'info, ScheduleUpgrade<'info>>, signer_count: u8) -> Result<()> {
        let signer_count = signer_count as usize;
        require!(ctx.remaining_accounts.len() >= signer_count, PredError::NotEnoughSigners);
        verify_signers(
            ctx.accounts.council.threshold,
            &ctx.accounts.council.signers[..ctx.accounts.council.count as usize],
            &ctx.remaining_accounts[..signer_count],
        )?;
        let now = Clock::get()?.unix_timestamp;
        ctx.accounts.council.buffer = ctx.accounts.buffer.key();
        ctx.accounts.council.spill = ctx.accounts.spill.key();
        ctx.accounts.council.proposal_eta = now.checked_add(UPGRADE_DELAY).ok_or(error!(PredError::Overflow))?;
        Ok(())
    }

    #[cfg(feature = "devnet-testing")]
    pub fn override_upgrade_delay<'info>(
        ctx: Context<'_, '_, 'info, 'info, OverrideUpgradeDelay<'info>>,
        signer_count: u8,
    ) -> Result<()> {
        require!(ctx.accounts.council.proposal_eta != 0, PredError::NothingToForward);
        let signer_count = signer_count as usize;
        let member_count = ctx.accounts.council.count as usize;
        require!(signer_count == member_count, PredError::NotEnoughSigners);
        require!(ctx.remaining_accounts.len() >= signer_count, PredError::NotEnoughSigners);
        verify_signers(
            ctx.accounts.council.count,
            &ctx.accounts.council.signers[..member_count],
            &ctx.remaining_accounts[..signer_count],
        )?;
        ctx.accounts.council.proposal_eta = Clock::get()?.unix_timestamp;
        Ok(())
    }

    pub fn execute_upgrade(ctx: Context<ExecuteUpgrade>) -> Result<()> {
        require!(ctx.accounts.council.proposal_eta != 0, PredError::NothingToForward);
        require!(Clock::get()?.unix_timestamp >= ctx.accounts.council.proposal_eta, PredError::UpgradeTimelock);
        require!(ctx.accounts.buffer.key() == ctx.accounts.council.buffer, PredError::WrongEvent);
        require!(ctx.accounts.spill.key() == ctx.accounts.council.spill, PredError::WrongEvent);
        let bump = ctx.accounts.council.bump;
        let (program_data, _) = Pubkey::find_program_address(
            &[ctx.accounts.program_account.key().as_ref()],
            &anchor_lang::solana_program::bpf_loader_upgradeable::id(),
        );
        require_keys_eq!(ctx.accounts.program_data.key(), program_data, PredError::WrongEvent);
        let instruction = anchor_lang::solana_program::bpf_loader_upgradeable::upgrade(
            &ctx.accounts.program_account.key(),
            &ctx.accounts.buffer.key(),
            &ctx.accounts.council.key(),
            &ctx.accounts.spill.key(),
        );
        anchor_lang::solana_program::program::invoke_signed(
            &instruction,
            &[
                ctx.accounts.program_data.to_account_info(),
                ctx.accounts.program_account.to_account_info(),
                ctx.accounts.buffer.to_account_info(),
                ctx.accounts.spill.to_account_info(),
                ctx.accounts.rent.to_account_info(),
                ctx.accounts.clock.to_account_info(),
                ctx.accounts.council.to_account_info(),
            ],
            &[&[UPGRADE_SEED, &[bump]]],
        )?;
        ctx.accounts.council.proposal_eta = 0;
        Ok(())
    }
}

/// Records one signer's approval. A different proposal replaces the old one and restarts
/// the count. Returns true once the threshold agrees on the current proposal.
fn record_approval(event: &mut Event, signers: &ResultSigners, signer: &Pubkey, kind: u8, results: [u8; math::MAX_GAMES]) -> Result<bool> {
    require!(event.status == EventStatus::Open, PredError::BadStatus);
    require!(Clock::get()?.unix_timestamp < event.resolve_timeout, PredError::ResolutionExpired);
    let position = signers.signers[..signers.count as usize]
        .iter()
        .position(|member| member == signer)
        .ok_or(error!(PredError::NotEnoughSigners))?;
    if event.proposal_kind != kind || event.proposal_results != results {
        event.proposal_kind = kind;
        event.proposal_results = results;
        event.approvals = 0;
    }
    event.approvals |= 1 << position;
    Ok(event.approvals.count_ones() >= u32::from(signers.threshold))
}

fn apply_settlement(event: &mut Event, vault_lamports: u64) -> Result<()> {
    let tiers: Vec<math::Tier> = event.tiers[..event.tier_count as usize]
        .iter()
        .map(|tier| math::Tier { correct: tier.correct, percent: tier.percent })
        .collect();
    let settlement = math::settle(event.sales, event.rollover_in, event.game_count, &tiers, &event.winners).map_err(|_| error!(PredError::BadSettlement))?;
    event.per_winner = [0; math::MAX_GAMES];
    for (tier, per_winner) in tiers.iter().zip(&settlement.per_winner) {
        event.per_winner[tier.correct as usize - 1] = *per_winner;
    }
    event.house = settlement.house;
    event.paid = settlement.paid;
    event.rollover_out = settlement.rollover_out;
    event.status = EventStatus::Resolved;
    let liquid = vault_lamports.checked_sub(event.rent_reserve).ok_or(error!(PredError::Insolvent))?;
    let liability = settlement.paid.checked_add(settlement.house).ok_or(error!(PredError::Overflow))?.checked_add(settlement.rollover_out).ok_or(error!(PredError::Overflow))?;
    require!(liquid >= liability, PredError::Insolvent);
    Ok(())
}

fn check_reveal(reveal: &RevealArg, game_count: u8) -> Result<()> {
    let games = game_count as usize;
    require!(reveal.picks[games..].iter().all(|mask| *mask == 0), PredError::BadPicks);
    let combinations = math::combination_count(&reveal.picks[..games]).map_err(|_| error!(PredError::BadPicks))?;
    require!(combinations == reveal.combinations, PredError::BadPicks);
    require!(coupon_commitment(&reveal.owner, &reveal.picks, reveal.nonce) == reveal.commitment, PredError::BadCommitment);
    Ok(())
}

fn prove_empty(root: &[u8; 32], index: u32, proof: &[[u8; 32]]) -> Result<()> {
    require!(proof.len() == SLIP_DEPTH, PredError::BadPicks);
    require!(slip_fold(index, &slip_zero(0), proof) == *root, PredError::BadCommitment);
    Ok(())
}

fn require_leaf(root: &[u8; 32], index: u32, leaf: &[u8; 32], proof: &[[u8; 32]]) -> Result<()> {
    require!(proof.len() == SLIP_DEPTH, PredError::BadPicks);
    require!(slip_fold(index, leaf, proof) == *root, PredError::BadCommitment);
    Ok(())
}

fn write_leaf(root: &mut [u8; 32], index: u32, proof: &[[u8; 32]], leaf: &[u8; 32]) -> Result<()> {
    prove_empty(root, index, proof)?;
    *root = slip_fold(index, leaf, proof);
    Ok(())
}

fn payout_for(coupon: &Coupon, event: &Event) -> Result<u64> {
    let game_count = event.game_count as usize;
    let counts = math::correct_counts(&coupon.picks[..game_count], &event.results[..game_count]).map_err(|_| error!(PredError::BadResults))?;
    let mut amount = 0u64;
    for tier in &event.tiers[..event.tier_count as usize] {
        let rows = u64::from(counts[tier.correct as usize]);
        let per_winner = event.per_winner[tier.correct as usize - 1];
        amount = amount.checked_add(per_winner.checked_mul(rows).ok_or(error!(PredError::Overflow))?).ok_or(error!(PredError::Overflow))?;
    }
    Ok(amount)
}

fn close_open_event(event: &mut Event) -> Result<()> {
    event.rollover_out = event.rollover_in;
    event.status = EventStatus::Cancelled;
    event.proposal_kind = PROPOSAL_NONE;
    Ok(())
}

fn verify_signers(threshold: u8, members: &[Pubkey], provided: &[AccountInfo]) -> Result<()> {
    require!(provided.len() >= threshold as usize, PredError::NotEnoughSigners);
    let mut seen = [false; MAX_SIGNERS];
    for info in provided {
        require!(info.is_signer, PredError::NotEnoughSigners);
        let key = info.key();
        let position = members.iter().position(|signer| *signer == key).ok_or(error!(PredError::NotEnoughSigners))?;
        require!(!seen[position], PredError::NotEnoughSigners);
        seen[position] = true;
    }
    Ok(())
}

pub fn coupon_commitment(player: &Pubkey, picks: &[u8; math::MAX_GAMES], nonce: u64) -> [u8; 32] {
    anchor_lang::solana_program::hash::hashv(&[player.as_ref(), picks, &nonce.to_le_bytes()]).to_bytes()
}

pub fn slip_leaf(index: u32, owner: &Pubkey, combinations: u32, commitment: &[u8; 32]) -> [u8; 32] {
    anchor_lang::solana_program::hash::hashv(&[
        b"slip-leaf",
        &index.to_le_bytes(),
        owner.as_ref(),
        &combinations.to_le_bytes(),
        commitment,
    ])
    .to_bytes()
}

pub fn slip_zero(level: usize) -> [u8; 32] {
    let mut node = anchor_lang::solana_program::hash::hash(b"slip-empty").to_bytes();
    for _ in 0..level {
        node = anchor_lang::solana_program::hash::hashv(&[&node, &node]).to_bytes();
    }
    node
}

pub fn slip_empty_root() -> [u8; 32] {
    slip_zero(SLIP_DEPTH)
}

pub fn slip_mark() -> [u8; 32] {
    anchor_lang::solana_program::hash::hash(b"slip-marked").to_bytes()
}

pub fn slip_reveal_leaf(index: u32, owner: &Pubkey, combinations: u32, commitment: &[u8; 32], picks: &[u8; math::MAX_GAMES]) -> [u8; 32] {
    anchor_lang::solana_program::hash::hashv(&[
        b"slip-reveal",
        &index.to_le_bytes(),
        owner.as_ref(),
        &combinations.to_le_bytes(),
        commitment,
        picks,
    ])
    .to_bytes()
}

/// Folds a leaf up `SLIP_DEPTH` sibling hashes. `proof[0]` is the leaf's sibling.
pub fn slip_fold(index: u32, leaf: &[u8; 32], proof: &[[u8; 32]]) -> [u8; 32] {
    let mut node = *leaf;
    let mut cursor = index;
    for sibling in proof {
        node = if cursor & 1 == 0 {
            anchor_lang::solana_program::hash::hashv(&[&node, sibling]).to_bytes()
        } else {
            anchor_lang::solana_program::hash::hashv(&[sibling, &node]).to_bytes()
        };
        cursor >>= 1;
    }
    node
}

pub fn slip_reveal_hash(previous: &[u8; 32], index: u32, owner: &Pubkey, combinations: u32, commitment: &[u8; 32]) -> [u8; 32] {
    anchor_lang::solana_program::hash::hashv(&[
        b"reveal-v1",
        previous,
        &index.to_le_bytes(),
        owner.as_ref(),
        &combinations.to_le_bytes(),
        commitment,
    ])
    .to_bytes()
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

fn schedule_insert(schedule: &mut Schedule, event: Pubkey, deadline: i64) -> Result<()> {
    require!((schedule.count as usize) < MAX_SCHEDULE, PredError::ScheduleFull);
    let index = schedule.count as usize;
    schedule.entries[index] = ScheduleEntry { event, deadline };
    schedule.count += 1;
    Ok(())
}

fn schedule_remove(schedule: &mut Schedule, event: Pubkey) -> Result<()> {
    let count = schedule.count as usize;
    let position = schedule.entries[..count]
        .iter()
        .position(|entry| entry.event == event)
        .ok_or(error!(PredError::NotScheduled))?;
    schedule.entries[position] = schedule.entries[count - 1];
    schedule.entries[count - 1] = ScheduleEntry::default();
    schedule.count -= 1;
    Ok(())
}

fn earliest_future(schedule: &Schedule, now: i64) -> Option<Pubkey> {
    let mut best: Option<(i64, Pubkey)> = None;
    for entry in &schedule.entries[..schedule.count as usize] {
        if entry.deadline > now {
            match best {
                Some((deadline, _)) if deadline <= entry.deadline => {}
                _ => best = Some((entry.deadline, entry.event)),
            }
        }
    }
    best.map(|(_, event)| event)
}

fn transfer_from_vault(vault: &AccountInfo, to: &AccountInfo, amount: u64, reserve: u64) -> Result<()> {
    let available = vault.lamports().checked_sub(reserve).ok_or(error!(PredError::InsufficientVault))?;
    require!(available >= amount, PredError::InsufficientVault);
    let to_balance = to.lamports();
    **vault.try_borrow_mut_lamports()? = vault.lamports().checked_sub(amount).ok_or(error!(PredError::InsufficientVault))?;
    **to.try_borrow_mut_lamports()? = to_balance.checked_add(amount).ok_or(error!(PredError::Overflow))?;
    Ok(())
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default)]
pub struct TierArg {
    pub correct: u8,
    pub percent: u8,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct CreateEventArgs {
    pub nonce: u64,
    pub game_count: u8,
    pub price: u64,
    pub deadline: i64,
    pub reveal_deadline: i64,
    pub end_time: i64,
    pub resolve_timeout: i64,
    pub threshold: u8,
    pub signers: Vec<Pubkey>,
    pub tiers: Vec<TierArg>,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq, Eq)]
pub enum EventStatus {
    Open,
    Resolved,
    Cancelled,
    Tallying,
}

/// `per_winner[n - 1]` is the payout per row with `n` correct picks.
/// `winners[n]` counts the rows with `n` correct picks while tallying.
#[account]
pub struct Event {
    pub admin: Pubkey,
    pub nonce: u64,
    pub game_count: u8,
    pub price: u64,
    pub tier_count: u8,
    pub tiers: [TierArg; math::MAX_GAMES],
    pub deadline: i64,
    pub reveal_deadline: i64,
    pub end_time: i64,
    pub resolve_timeout: i64,
    pub status: EventStatus,
    pub results: [u8; math::MAX_GAMES],
    pub coupon_count: u32,
    pub sales: u64,
    pub rollover_in: u64,
    pub house: u64,
    pub paid: u64,
    pub rollover_out: u64,
    pub rollover_forwarded: bool,
    pub per_winner: [u64; math::MAX_GAMES],
    pub bump: u8,
    pub vault_bump: u8,
    pub rent_reserve: u64,
    pub house_withdrawn: bool,
    pub proposal_kind: u8,
    pub proposal_results: [u8; math::MAX_GAMES],
    pub approvals: u8,
    pub tallied: u32,
    pub winners: [u64; math::MAX_GAMES + 1],
}

impl Event {
    pub const SIZE: usize = 448;
}

#[account]
pub struct Coupon {
    pub event: Pubkey,
    pub owner: Pubkey,
    pub index: u32,
    pub game_count: u8,
    pub picks: [u8; math::MAX_GAMES],
    pub commitment: [u8; 32],
    pub revealed: bool,
    pub combinations: u32,
    pub lamports_paid: u64,
    pub bump: u8,
    pub sealed: [u8; SEALED_LEN],
}

impl Coupon {
    pub const SIZE: usize = 32 + 32 + 4 + 1 + math::MAX_GAMES + 32 + 1 + 4 + 8 + 1 + SEALED_LEN;
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct SlipArg {
    pub commitment: [u8; 32],
    pub combinations: u32,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct RevealArg {
    pub index: u32,
    pub owner: Pubkey,
    pub combinations: u32,
    pub picks: [u8; math::MAX_GAMES],
    pub nonce: u64,
    pub commitment: [u8; 32],
}

/// One book per event. `entry_hash` is the Merkle root of the coupons entered so far.
/// One place transaction adds one coupon. This is not a coupon account.
#[account]
pub struct SlipBook {
    pub event: Pubkey,
    pub count: u32,
    pub entry_hash: [u8; 32],
    pub reveal_count: u32,
    /// Merkle root of reveal leaves. An empty leaf means that index was not revealed.
    pub reveal_hash: [u8; 32],
    pub scored_root: [u8; 32],
    pub claimed_root: [u8; 32],
    pub bump: u8,
}

impl SlipBook {
    pub const SIZE: usize = 32 + 4 + 32 + 4 + 32 + 32 + 32 + 1;
}

#[derive(Accounts)]
pub struct PlaceSlip<'info> {
    #[account(mut)]
    pub player: Signer<'info>,
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(
        init_if_needed,
        payer = player,
        space = 8 + SlipBook::SIZE,
        seeds = [SLIP_SEED, event.key().as_ref()],
        bump
    )]
    pub slips: Account<'info, SlipBook>,
    #[account(seeds = [GATE_SEED, event.admin.as_ref()], bump = gate.bump)]
    pub gate: Account<'info, Gate>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct RevealSlip<'info> {
    pub caller: Signer<'info>,
    #[account(seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [SLIP_SEED, event.key().as_ref()], bump = slips.bump)]
    pub slips: Account<'info, SlipBook>,
}

#[derive(Accounts)]
pub struct ScoreSlip<'info> {
    pub caller: Signer<'info>,
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, seeds = [SLIP_SEED, event.key().as_ref()], bump = slips.bump)]
    pub slips: Account<'info, SlipBook>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Box<Account<'info, Schedule>>,
}

#[derive(Accounts)]
pub struct ClaimSlip<'info> {
    #[account(mut)]
    pub owner: Signer<'info>,
    #[account(seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, seeds = [SLIP_SEED, event.key().as_ref()], bump = slips.bump)]
    pub slips: Account<'info, SlipBook>,
}

#[derive(Accounts)]
pub struct RefundSlip<'info> {
    #[account(mut)]
    pub owner: Signer<'info>,
    #[account(seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, seeds = [SLIP_SEED, event.key().as_ref()], bump = slips.bump)]
    pub slips: Account<'info, SlipBook>,
}

#[account]
pub struct Vault {}

#[account]
pub struct Escrow {
    pub pending: u64,
    pub rent_reserve: u64,
}

impl Escrow {
    pub const SIZE: usize = 8 + 8;
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default)]
pub struct ScheduleEntry {
    pub event: Pubkey,
    pub deadline: i64,
}

#[account]
pub struct Schedule {
    pub admin: Pubkey,
    pub count: u8,
    pub entries: [ScheduleEntry; MAX_SCHEDULE],
}

impl Schedule {
    pub const SIZE: usize = 32 + 1 + MAX_SCHEDULE * (32 + 8);
}

#[derive(Accounts)]
#[instruction(args: CreateEventArgs)]
pub struct CreateEvent<'info> {
    #[account(mut)]
    pub admin: Signer<'info>,
    #[account(
        init,
        payer = admin,
        space = 8 + Event::SIZE,
        seeds = [EVENT_SEED, admin.key().as_ref(), &args.nonce.to_le_bytes()],
        bump
    )]
    pub event: Box<Account<'info, Event>>,
    #[account(
        init,
        payer = admin,
        space = 8,
        seeds = [VAULT_SEED, event.key().as_ref()],
        bump
    )]
    pub vault: Account<'info, Vault>,
    #[account(
        init_if_needed,
        payer = admin,
        space = 8 + Escrow::SIZE,
        seeds = [ESCROW_SEED, admin.key().as_ref()],
        bump
    )]
    pub escrow: Account<'info, Escrow>,
    #[account(
        init_if_needed,
        payer = admin,
        space = 8 + Schedule::SIZE,
        seeds = [SCHEDULE_SEED, admin.key().as_ref()],
        bump
    )]
    pub schedule: Box<Account<'info, Schedule>>,
    #[account(
        init,
        payer = admin,
        space = 8 + ResultSigners::SIZE,
        seeds = [SIGNERS_SEED, event.key().as_ref()],
        bump
    )]
    pub signers: Account<'info, ResultSigners>,
    #[account(
        init_if_needed,
        payer = admin,
        space = 8 + Gate::SIZE,
        seeds = [GATE_SEED, admin.key().as_ref()],
        bump
    )]
    pub gate: Account<'info, Gate>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct PlaceCoupon<'info> {
    #[account(mut)]
    pub player: Signer<'info>,
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(
        init,
        payer = player,
        space = 8 + Coupon::SIZE,
        seeds = [COUPON_SEED, event.key().as_ref(), &event.coupon_count.to_le_bytes()],
        bump
    )]
    pub coupon: Account<'info, Coupon>,
    #[account(seeds = [GATE_SEED, event.admin.as_ref()], bump = gate.bump)]
    pub gate: Account<'info, Gate>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct Approve<'info> {
    pub signer: Signer<'info>,
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(seeds = [SIGNERS_SEED, event.key().as_ref()], bump = signers.bump)]
    pub signers: Account<'info, ResultSigners>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Box<Account<'info, Schedule>>,
}

#[derive(Accounts)]
pub struct Tally<'info> {
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Box<Account<'info, Schedule>>,
}

#[derive(Accounts)]
pub struct Claim<'info> {
    #[account(mut, close = owner)]
    pub coupon: Account<'info, Coupon>,
    #[account(mut, address = coupon.event)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, address = coupon.owner)]
    pub owner: Signer<'info>,
}

#[derive(Accounts)]
pub struct CloseLost<'info> {
    #[account(mut, close = owner)]
    pub coupon: Account<'info, Coupon>,
    #[account(seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump, constraint = coupon.event == event.key() @ PredError::WrongEvent)]
    pub event: Box<Account<'info, Event>>,
    /// CHECK: the rent goes back to the coupon owner. The keeper signs instead.
    #[account(mut, address = coupon.owner)]
    pub owner: UncheckedAccount<'info>,
    pub caller: Signer<'info>,
}

#[derive(Accounts)]
pub struct RevealCoupon<'info> {
    /// CHECK: must equal the coupon owner. It does not sign; the commitment binds the picks.
    pub player: UncheckedAccount<'info>,
    #[account(mut, seeds = [COUPON_SEED, event.key().as_ref(), &coupon.index.to_le_bytes()], bump = coupon.bump, constraint = coupon.owner == player.key() @ PredError::WrongEvent)]
    pub coupon: Account<'info, Coupon>,
    #[account(seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump, constraint = coupon.event == event.key() @ PredError::WrongEvent)]
    pub event: Box<Account<'info, Event>>,
}

#[derive(Accounts)]
pub struct SetPause<'info> {
    pub admin: Signer<'info>,
    #[account(mut, seeds = [GATE_SEED, admin.key().as_ref()], bump = gate.bump, has_one = admin)]
    pub gate: Account<'info, Gate>,
}

#[derive(Accounts)]
pub struct ExpireEvent<'info> {
    pub caller: Signer<'info>,
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Box<Account<'info, Schedule>>,
}

#[derive(Accounts)]
pub struct Refund<'info> {
    #[account(mut, close = owner)]
    pub coupon: Account<'info, Coupon>,
    #[account(mut, address = coupon.event)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, address = coupon.owner)]
    pub owner: Signer<'info>,
}

#[derive(Accounts)]
pub struct WithdrawHouse<'info> {
    #[account(mut)]
    pub admin: Signer<'info>,
    #[account(mut, seeds = [EVENT_SEED, admin.key().as_ref(), &event.nonce.to_le_bytes()], bump = event.bump, has_one = admin)]
    pub event: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
}

#[derive(Accounts)]
pub struct ForwardRollover<'info> {
    #[account(mut, seeds = [EVENT_SEED, source.admin.as_ref(), &source.nonce.to_le_bytes()], bump = source.bump)]
    pub source: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, source.key().as_ref()], bump = source.vault_bump)]
    pub source_vault: Account<'info, Vault>,
    #[account(
        mut,
        seeds = [EVENT_SEED, destination.admin.as_ref(), &destination.nonce.to_le_bytes()],
        bump = destination.bump,
        constraint = destination.admin == source.admin @ PredError::WrongEvent
    )]
    pub destination: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, destination.key().as_ref()], bump = destination.vault_bump)]
    pub destination_vault: Account<'info, Vault>,
    #[account(mut, seeds = [SCHEDULE_SEED, source.admin.as_ref()], bump)]
    pub schedule: Box<Account<'info, Schedule>>,
}

#[derive(Accounts)]
pub struct ParkRollover<'info> {
    #[account(mut, seeds = [EVENT_SEED, source.admin.as_ref(), &source.nonce.to_le_bytes()], bump = source.bump)]
    pub source: Box<Account<'info, Event>>,
    #[account(mut, seeds = [VAULT_SEED, source.key().as_ref()], bump = source.vault_bump)]
    pub source_vault: Account<'info, Vault>,
    #[account(mut, seeds = [ESCROW_SEED, source.admin.as_ref()], bump)]
    pub escrow: Account<'info, Escrow>,
    #[account(mut, seeds = [SCHEDULE_SEED, source.admin.as_ref()], bump)]
    pub schedule: Box<Account<'info, Schedule>>,
}

#[error_code]
pub enum PredError {
    BadEvent,
    BadTiers,
    BadStatus,
    DeadlinePassed,
    TooManyCoupons,
    BadPicks,
    Overflow,
    BadResults,
    TooEarly,
    MissingCoupon,
    WrongEvent,
    CouponOrder,
    BadSettlement,
    NothingToClaim,
    InsufficientVault,
    AlreadyForwarded,
    NothingToForward,
    DestinationStarted,
    ScheduleFull,
    NotScheduled,
    NotEarliestEvent,
    FutureEventExists,
    BadSigners,
    NotEnoughSigners,
    RevealClosed,
    ResolutionExpired,
    Paused,
    BadCommitment,
    Insolvent,
    UpgradeTimelock,
    AlreadyWithdrawn,
    DrawInProgress,
    DrawRequired,
    LotteryFull,
    DrawNotReady,
    WrongWinner,
}

#[account]
pub struct UpgradeCouncil {
    pub threshold: u8,
    pub count: u8,
    pub signers: [Pubkey; MAX_SIGNERS],
    pub bump: u8,
    pub proposal_eta: i64,
    pub buffer: Pubkey,
    pub spill: Pubkey,
}

impl UpgradeCouncil {
    pub const SIZE: usize = 1 + 1 + MAX_SIGNERS * 32 + 1 + 8 + 32 + 32;
}

#[account]
pub struct ResultSigners {
    pub event: Pubkey,
    pub threshold: u8,
    pub count: u8,
    pub signers: [Pubkey; MAX_SIGNERS],
    pub bump: u8,
}

impl ResultSigners {
    pub const SIZE: usize = 32 + 1 + 1 + MAX_SIGNERS * 32 + 1;
}

#[account]
pub struct Gate {
    pub admin: Pubkey,
    pub paused: bool,
    pub bump: u8,
}

impl Gate {
    pub const SIZE: usize = 32 + 1 + 1;
}

#[derive(Accounts)]
pub struct InitializeUpgrade<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,
    #[account(init, payer = payer, space = 8 + UpgradeCouncil::SIZE, seeds = [UPGRADE_SEED], bump)]
    pub council: Account<'info, UpgradeCouncil>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct ScheduleUpgrade<'info> {
    #[account(mut, seeds = [UPGRADE_SEED], bump = council.bump)]
    pub council: Account<'info, UpgradeCouncil>,
    /// CHECK: the buffer is recorded now and checked again when the delay expires.
    pub buffer: UncheckedAccount<'info>,
    /// CHECK: receives leftover buffer rent after a successful upgrade.
    pub spill: UncheckedAccount<'info>,
}

#[cfg(feature = "devnet-testing")]
#[derive(Accounts)]
pub struct OverrideUpgradeDelay<'info> {
    #[account(mut, seeds = [UPGRADE_SEED], bump = council.bump)]
    pub council: Account<'info, UpgradeCouncil>,
}

#[derive(Accounts)]
pub struct ExecuteUpgrade<'info> {
    #[account(mut, seeds = [UPGRADE_SEED], bump = council.bump)]
    pub council: Account<'info, UpgradeCouncil>,
    /// CHECK: the program-data account. The loader upgrade instruction requires it, and the
    /// council can sign only the accounts this instruction passes through.
    #[account(mut)]
    pub program_data: UncheckedAccount<'info>,
    /// CHECK: the upgradeable loader identifies the deployed program.
    pub program_account: UncheckedAccount<'info>,
    /// CHECK: must match the buffer stored in the council proposal.
    #[account(mut)]
    pub buffer: UncheckedAccount<'info>,
    /// CHECK: must match the spill account stored in the council proposal.
    #[account(mut)]
    pub spill: UncheckedAccount<'info>,
    pub rent: Sysvar<'info, Rent>,
    pub clock: Sysvar<'info, Clock>,
}
