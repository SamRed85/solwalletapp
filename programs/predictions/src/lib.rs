use anchor_lang::prelude::*;
use anchor_lang::system_program;

mod math;

declare_id!("35eSgMRzq1xNM4qDDLezingfXmSDM2ZAjRMTWcJ4QNca");

const MAX_COUPONS: u32 = 30;
const EVENT_SEED: &[u8] = b"event";
const VAULT_SEED: &[u8] = b"vault";
const COUPON_SEED: &[u8] = b"coupon";
const ESCROW_SEED: &[u8] = b"rollover-escrow";
const SCHEDULE_SEED: &[u8] = b"schedule";
const SIGNERS_SEED: &[u8] = b"signers";
const GATE_SEED: &[u8] = b"gate";
const UPGRADE_SEED: &[u8] = b"upgrade-council";
#[cfg(not(feature = "devnet-testing"))]
const UPGRADE_DELAY: i64 = 24 * 60 * 60;
#[cfg(feature = "devnet-testing")]
const UPGRADE_DELAY: i64 = 5 * 60;
const MAX_SCHEDULE: usize = 16;
const MAX_SIGNERS: usize = 5;

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
        require!(args.resolve_timeout > args.reveal_deadline, PredError::BadEvent);
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

    pub fn place_coupon(ctx: Context<PlaceCoupon>, commitment: [u8; 32], combinations: u32) -> Result<()> {
        require!(!ctx.accounts.gate.paused, PredError::Paused);
        let event = &mut ctx.accounts.event;
        require!(event.status == EventStatus::Open, PredError::BadStatus);
        require!(Clock::get()?.unix_timestamp < event.deadline, PredError::DeadlinePassed);
        require!(event.coupon_count < MAX_COUPONS, PredError::TooManyCoupons);
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

        event.coupon_count += 1;
        event.sales = event.sales.checked_add(cost).ok_or(error!(PredError::Overflow))?;
        Ok(())
    }

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

    pub fn resolve_event<'info>(ctx: Context<'_, '_, 'info, 'info, ResolveEvent<'info>>, results: Vec<u8>, signer_count: u8) -> Result<()> {
        let game_count = ctx.accounts.event.game_count;
        require!(results.len() == game_count as usize, PredError::BadResults);
        require!(results.iter().all(|result| math::mask_for(*result).is_some()), PredError::BadResults);
        let now = Clock::get()?.unix_timestamp;
        require!(now >= ctx.accounts.event.reveal_deadline, PredError::TooEarly);
        require!(now < ctx.accounts.event.resolve_timeout, PredError::ResolutionExpired);
        require!(ctx.accounts.event.status == EventStatus::Open, PredError::BadStatus);
        let signer_count = signer_count as usize;
        require!(ctx.remaining_accounts.len() >= signer_count, PredError::NotEnoughSigners);
        verify_signers(
            ctx.accounts.signers.threshold,
            &ctx.accounts.signers.signers[..ctx.accounts.signers.count as usize],
            &ctx.remaining_accounts[..signer_count],
        )?;
        let coupons = &ctx.remaining_accounts[signer_count..];
        require!(coupons.len() == ctx.accounts.event.coupon_count as usize, PredError::MissingCoupon);

        let mut winners = vec![0u32; game_count as usize + 1];
        for (position, account) in coupons.iter().enumerate() {
            let coupon = Account::<Coupon>::try_from(account)?;
            require!(coupon.event == ctx.accounts.event.key(), PredError::WrongEvent);
            require!(coupon.index == position as u32, PredError::CouponOrder);
            if !coupon.revealed {
                continue;
            }
            let counts = math::correct_counts(&coupon.picks[..game_count as usize], &results).map_err(|_| error!(PredError::BadResults))?;
            for (score, count) in counts.iter().enumerate() {
                winners[score] = winners[score].checked_add(*count).ok_or(error!(PredError::Overflow))?;
            }
        }

        let event = &mut ctx.accounts.event;
        let tiers: Vec<math::Tier> = event.tiers[..event.tier_count as usize]
            .iter()
            .map(|tier| math::Tier { correct: tier.correct, percent: tier.percent })
            .collect();
        let settlement = math::settle(event.sales, event.rollover_in, game_count, &tiers, &winners).map_err(|_| error!(PredError::BadSettlement))?;
        event.results = [0; math::MAX_GAMES];
        event.results[..results.len()].copy_from_slice(&results);
        event.per_winner = [0; math::MAX_GAMES];
        for (tier, per_winner) in tiers.iter().zip(&settlement.per_winner) {
            event.per_winner[tier.correct as usize] = *per_winner;
        }
        event.house = settlement.house;
        event.paid = settlement.paid;
        event.rollover_out = settlement.rollover_out;
        event.status = EventStatus::Resolved;
        let liquid = ctx.accounts.vault.to_account_info().lamports().checked_sub(event.rent_reserve).ok_or(error!(PredError::Insolvent))?;
        let liability = settlement.paid.checked_add(settlement.house).ok_or(error!(PredError::Overflow))?.checked_add(settlement.rollover_out).ok_or(error!(PredError::Overflow))?;
        require!(liquid >= liability, PredError::Insolvent);
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

    pub fn claim(ctx: Context<Claim>) -> Result<()> {
        let event = &ctx.accounts.event;
        require!(event.status == EventStatus::Resolved, PredError::BadStatus);
        let amount = payout_for(&ctx.accounts.coupon, event)?;
        if amount == 0 {
            return Ok(());
        }
        transfer_from_vault(
            &ctx.accounts.vault.to_account_info(),
            &ctx.accounts.owner.to_account_info(),
            amount,
            event.rent_reserve,
        )?;
        Ok(())
    }

    pub fn cancel_event<'info>(ctx: Context<'_, '_, 'info, 'info, CancelEvent<'info>>, signer_count: u8) -> Result<()> {
        require!(ctx.accounts.event.status == EventStatus::Open, PredError::BadStatus);
        require!(Clock::get()?.unix_timestamp < ctx.accounts.event.resolve_timeout, PredError::ResolutionExpired);
        let signer_count = signer_count as usize;
        require!(ctx.remaining_accounts.len() >= signer_count, PredError::NotEnoughSigners);
        verify_signers(
            ctx.accounts.signers.threshold,
            &ctx.accounts.signers.signers[..ctx.accounts.signers.count as usize],
            &ctx.remaining_accounts[..signer_count],
        )?;
        close_open_event(&mut ctx.accounts.event)?;
        schedule_remove(&mut ctx.accounts.schedule, ctx.accounts.event.key())?;
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
        let instruction = anchor_lang::solana_program::bpf_loader_upgradeable::upgrade(
            &ctx.accounts.program_account.key(),
            &ctx.accounts.buffer.key(),
            &ctx.accounts.council.key(),
            &ctx.accounts.spill.key(),
        );
        anchor_lang::solana_program::program::invoke_signed(
            &instruction,
            &[
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

fn payout_for(coupon: &Coupon, event: &Event) -> Result<u64> {
    let game_count = event.game_count as usize;
    let counts = math::correct_counts(&coupon.picks[..game_count], &event.results[..game_count]).map_err(|_| error!(PredError::BadResults))?;
    let mut amount = 0u64;
    for tier in &event.tiers[..event.tier_count as usize] {
        let rows = u64::from(counts[tier.correct as usize]);
        let per_winner = event.per_winner[tier.correct as usize];
        amount = amount.checked_add(per_winner.checked_mul(rows).ok_or(error!(PredError::Overflow))?).ok_or(error!(PredError::Overflow))?;
    }
    Ok(amount)
}

fn close_open_event(event: &mut Event) -> Result<()> {
    event.rollover_out = event.rollover_in;
    event.status = EventStatus::Cancelled;
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
}

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
}

impl Event {
    pub const SIZE: usize = 320;
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
}

impl Coupon {
    pub const SIZE: usize = 32 + 32 + 4 + 1 + math::MAX_GAMES + 32 + 1 + 4 + 8 + 1;
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
    pub event: Account<'info, Event>,
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
        seeds = [ESCROW_SEED],
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
    pub schedule: Account<'info, Schedule>,
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
    pub event: Account<'info, Event>,
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
pub struct ResolveEvent<'info> {
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Account<'info, Event>,
    #[account(seeds = [SIGNERS_SEED, event.key().as_ref()], bump = signers.bump)]
    pub signers: Account<'info, ResultSigners>,
    #[account(seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Account<'info, Schedule>,
}

#[derive(Accounts)]
pub struct Claim<'info> {
    #[account(mut, close = owner)]
    pub coupon: Account<'info, Coupon>,
    #[account(mut, address = coupon.event)]
    pub event: Account<'info, Event>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, address = coupon.owner)]
    pub owner: Signer<'info>,
}

#[derive(Accounts)]
pub struct CancelEvent<'info> {
    #[account(mut, seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump)]
    pub event: Account<'info, Event>,
    #[account(seeds = [SIGNERS_SEED, event.key().as_ref()], bump = signers.bump)]
    pub signers: Account<'info, ResultSigners>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Account<'info, Schedule>,
}

#[derive(Accounts)]
pub struct RevealCoupon<'info> {
    pub player: Signer<'info>,
    #[account(mut, seeds = [COUPON_SEED, event.key().as_ref(), &coupon.index.to_le_bytes()], bump = coupon.bump, constraint = coupon.owner == player.key() @ PredError::WrongEvent)]
    pub coupon: Account<'info, Coupon>,
    #[account(seeds = [EVENT_SEED, event.admin.as_ref(), &event.nonce.to_le_bytes()], bump = event.bump, constraint = coupon.event == event.key() @ PredError::WrongEvent)]
    pub event: Account<'info, Event>,
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
    pub event: Account<'info, Event>,
    #[account(mut, seeds = [SCHEDULE_SEED, event.admin.as_ref()], bump)]
    pub schedule: Account<'info, Schedule>,
}

#[derive(Accounts)]
pub struct Refund<'info> {
    #[account(mut, close = owner)]
    pub coupon: Account<'info, Coupon>,
    #[account(mut, address = coupon.event)]
    pub event: Account<'info, Event>,
    #[account(mut, seeds = [VAULT_SEED, event.key().as_ref()], bump = event.vault_bump)]
    pub vault: Account<'info, Vault>,
    #[account(mut, address = coupon.owner)]
    pub owner: Signer<'info>,
}

#[derive(Accounts)]
pub struct ForwardRollover<'info> {
    #[account(mut, seeds = [EVENT_SEED, source.admin.as_ref(), &source.nonce.to_le_bytes()], bump = source.bump)]
    pub source: Account<'info, Event>,
    #[account(mut, seeds = [VAULT_SEED, source.key().as_ref()], bump = source.vault_bump)]
    pub source_vault: Account<'info, Vault>,
    #[account(
        mut,
        seeds = [EVENT_SEED, destination.admin.as_ref(), &destination.nonce.to_le_bytes()],
        bump = destination.bump,
        constraint = destination.admin == source.admin @ PredError::WrongEvent
    )]
    pub destination: Account<'info, Event>,
    #[account(mut, seeds = [VAULT_SEED, destination.key().as_ref()], bump = destination.vault_bump)]
    pub destination_vault: Account<'info, Vault>,
    #[account(mut, seeds = [SCHEDULE_SEED, source.admin.as_ref()], bump)]
    pub schedule: Account<'info, Schedule>,
}

#[derive(Accounts)]
pub struct ParkRollover<'info> {
    #[account(mut, seeds = [EVENT_SEED, source.admin.as_ref(), &source.nonce.to_le_bytes()], bump = source.bump)]
    pub source: Account<'info, Event>,
    #[account(mut, seeds = [VAULT_SEED, source.key().as_ref()], bump = source.vault_bump)]
    pub source_vault: Account<'info, Vault>,
    #[account(mut, seeds = [ESCROW_SEED], bump)]
    pub escrow: Account<'info, Escrow>,
    #[account(mut, seeds = [SCHEDULE_SEED, source.admin.as_ref()], bump)]
    pub schedule: Account<'info, Schedule>,
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
