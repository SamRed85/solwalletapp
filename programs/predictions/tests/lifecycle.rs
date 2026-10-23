use std::convert::TryFrom;
use std::str::FromStr;
use std::sync::atomic::{AtomicU32, Ordering};

use anchor_lang::{AccountDeserialize, InstructionData, ToAccountMetas};
use solana_sdk::compute_budget::ComputeBudgetInstruction;
use predictions::{CreateEventArgs, Escrow, Event, EventStatus, TierArg, UpgradeCouncil};
use solana_program_error::ProgramResult;
use solana_program_test::{processor, ProgramTest, ProgramTestContext};
use solana_sdk::{
    account_info::AccountInfo, clock::Clock, instruction::Instruction, pubkey::Pubkey, signature::Keypair, signer::Signer,
    system_instruction, transaction::Transaction,
};

fn process_instruction<'a, 'b, 'c, 'd>(
    program_id: &'a Pubkey,
    accounts: &'b [AccountInfo<'c>],
    data: &'d [u8],
) -> ProgramResult {
    // The test runtime names these borrows separately. Anchor accepts them only when the
    // slice and its account infos share one lifetime. Nothing stores the borrow beyond this call.
    let accounts = unsafe { &*(accounts as *const [AccountInfo<'c>] as *const [AccountInfo<'_>]) };
    predictions::entry(program_id, accounts, data)
}

fn pda(seeds: &[&[u8]]) -> Pubkey {
    Pubkey::find_program_address(seeds, &predictions::ID).0
}

async fn send(context: &mut ProgramTestContext, payer: &Keypair, instruction: Instruction) -> Result<(), solana_program_test::BanksClientError> {
    send_all(context, &[payer], instruction).await
}

static SENT: AtomicU32 = AtomicU32::new(0);

async fn send_all(context: &mut ProgramTestContext, signers: &[&Keypair], instruction: Instruction) -> Result<(), solana_program_test::BanksClientError> {
    let blockhash = context.banks_client.get_latest_blockhash().await.unwrap();
    // A distinct compute limit keeps repeated instructions from being dropped as duplicates.
    let budget = ComputeBudgetInstruction::set_compute_unit_limit(1_400_000 - SENT.fetch_add(1, Ordering::Relaxed));
    let transaction = Transaction::new_signed_with_payer(&[budget, instruction], Some(&signers[0].pubkey()), signers, blockhash);
    context.banks_client.process_transaction(transaction).await
}

async fn clock(context: &mut ProgramTestContext) -> Clock {
    context.banks_client.get_sysvar().await.unwrap()
}

fn set_clock(context: &mut ProgramTestContext, unix_timestamp: i64) {
    let mut next = Clock::default();
    next.unix_timestamp = unix_timestamp;
    context.set_sysvar(&next);
}

async fn fund(context: &mut ProgramTestContext, recipient: &Pubkey, lamports: u64) {
    let payer = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    send(context, &payer, system_instruction::transfer(&payer.pubkey(), recipient, lamports)).await.unwrap();
}

const REVEAL_NONCE: u64 = 7;

fn event_args(nonce: u64, deadline: i64) -> CreateEventArgs {
    CreateEventArgs {
        nonce,
        game_count: 7,
        price: 1_000_000,
        deadline,
        reveal_deadline: deadline + 20,
        end_time: deadline + 20,
        resolve_timeout: deadline + 100_000,
        threshold: 2,
        signers: vec![],
        tiers: vec![TierArg { correct: 7, percent: 95 }],
    }
}

fn all_home() -> [u8; 13] {
    let mut picks = [0u8; 13];
    picks[..7].fill(0b001);
    picks
}

fn schedule_of(admin: &Pubkey) -> Pubkey {
    pda(&[b"schedule", admin.as_ref()])
}

fn gate_of(admin: &Pubkey) -> Pubkey {
    pda(&[b"gate", admin.as_ref()])
}

fn escrow_of(admin: &Pubkey) -> Pubkey {
    pda(&[b"rollover-escrow", admin.as_ref()])
}

fn signers_of(event: &Pubkey) -> Pubkey {
    pda(&[b"signers", event.as_ref()])
}

fn create_ix(admin: Pubkey, other: Pubkey, nonce: u64, deadline: i64) -> (Instruction, Pubkey, Pubkey) {
    let mut args = event_args(nonce, deadline);
    args.signers = vec![admin, other];
    let event = pda(&[b"event", admin.as_ref(), &nonce.to_le_bytes()]);
    let vault = pda(&[b"vault", event.as_ref()]);
    let escrow = escrow_of(&admin);
    let schedule = schedule_of(&admin);
    let instruction = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::CreateEvent {
            admin,
            event,
            vault,
            escrow,
            schedule,
            signers: signers_of(&event),
            gate: gate_of(&admin),
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::CreateEvent { args }.data(),
    };
    (instruction, event, vault)
}

fn place_ix(player: Pubkey, admin: Pubkey, nonce: u64, index: u32) -> (Instruction, Pubkey) {
    place_picks_ix(player, admin, nonce, index, all_home(), 1)
}

fn place_picks_ix(player: Pubkey, admin: Pubkey, nonce: u64, index: u32, picks: [u8; 13], combinations: u32) -> (Instruction, Pubkey) {
    let event = pda(&[b"event", admin.as_ref(), &nonce.to_le_bytes()]);
    let vault = pda(&[b"vault", event.as_ref()]);
    let coupon = pda(&[b"coupon", event.as_ref(), &index.to_le_bytes()]);
    let instruction = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::PlaceCoupon {
            player,
            event,
            vault,
            coupon,
            gate: gate_of(&admin),
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::PlaceCoupon {
            commitment: predictions::coupon_commitment(&player, &picks, REVEAL_NONCE),
            combinations,
            sealed: [index as u8; predictions::SEALED_LEN],
        }
        .data(),
    };
    (instruction, coupon)
}

fn reveal_ix(player: Pubkey, admin: Pubkey, nonce: u64, index: u32) -> Instruction {
    reveal_picks_ix(player, admin, nonce, index, all_home())
}

fn reveal_picks_ix(player: Pubkey, admin: Pubkey, nonce: u64, index: u32, picks: [u8; 13]) -> Instruction {
    let event = pda(&[b"event", admin.as_ref(), &nonce.to_le_bytes()]);
    let coupon = pda(&[b"coupon", event.as_ref(), &index.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::RevealCoupon { player, coupon, event }.to_account_metas(None),
        data: predictions::instruction::RevealCoupon { picks, nonce: REVEAL_NONCE }.data(),
    }
}

fn claim_ix(coupon: Pubkey, event: Pubkey, owner: Pubkey) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Claim { coupon, event, vault: pda(&[b"vault", event.as_ref()]), owner }.to_account_metas(None),
        data: predictions::instruction::Claim {}.data(),
    }
}

async fn start() -> ProgramTestContext {
    ProgramTest::new("predictions", predictions::ID, processor!(process_instruction)).start_with_context().await
}

async fn load_event(context: &mut ProgramTestContext, event: Pubkey) -> Event {
    let account = context.banks_client.get_account(event).await.unwrap().unwrap();
    Event::try_deserialize(&mut account.data.as_slice()).unwrap()
}

async fn load_escrow(context: &mut ProgramTestContext, admin: &Pubkey) -> Escrow {
    let account = context.banks_client.get_account(escrow_of(admin)).await.unwrap().unwrap();
    Escrow::try_deserialize(&mut account.data.as_slice()).unwrap()
}

async fn load_council(context: &mut ProgramTestContext, council: Pubkey) -> UpgradeCouncil {
    let account = context.banks_client.get_account(council).await.unwrap().unwrap();
    UpgradeCouncil::try_deserialize(&mut account.data.as_slice()).unwrap()
}

fn approve_ix(signer: Pubkey, admin: Pubkey, event: Pubkey, results: Vec<u8>) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Approve { signer, event, signers: signers_of(&event), schedule: schedule_of(&admin) }.to_account_metas(None),
        data: predictions::instruction::ApproveResolution { results }.data(),
    }
}

fn approve_cancel_ix(signer: Pubkey, admin: Pubkey, event: Pubkey) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Approve { signer, event, signers: signers_of(&event), schedule: schedule_of(&admin) }.to_account_metas(None),
        data: predictions::instruction::ApproveCancel {}.data(),
    }
}

fn tally_ix(admin: Pubkey, event: Pubkey, coupons: &[Pubkey]) -> Instruction {
    let mut instruction = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Tally { event, vault: pda(&[b"vault", event.as_ref()]), schedule: schedule_of(&admin) }.to_account_metas(None),
        data: predictions::instruction::Tally {}.data(),
    };
    for coupon in coupons {
        instruction.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(*coupon, false));
    }
    instruction
}

/// Both result signers approve in separate transactions, then a stranger tallies every coupon.
async fn resolve(context: &mut ProgramTestContext, admin: &Keypair, other: &Keypair, event: Pubkey, results: Vec<u8>, coupons: &[Pubkey]) {
    send(context, admin, approve_ix(admin.pubkey(), admin.pubkey(), event, results.clone())).await.unwrap();
    fund(context, &other.pubkey(), 10_000_000).await;
    send(context, other, approve_ix(other.pubkey(), admin.pubkey(), event, results)).await.unwrap();
    send(context, admin, tally_ix(admin.pubkey(), event, coupons)).await.unwrap();
}

async fn cancel(context: &mut ProgramTestContext, admin: &Keypair, other: &Keypair, event: Pubkey) {
    send(context, admin, approve_cancel_ix(admin.pubkey(), admin.pubkey(), event)).await.unwrap();
    fund(context, &other.pubkey(), 10_000_000).await;
    send(context, other, approve_cancel_ix(other.pubkey(), admin.pubkey(), event)).await.unwrap();
}

fn forward_ix(admin: Pubkey, source_nonce: u64, destination_nonce: u64) -> Instruction {
    let source = pda(&[b"event", admin.as_ref(), &source_nonce.to_le_bytes()]);
    let destination = pda(&[b"event", admin.as_ref(), &destination_nonce.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ForwardRollover {
            source,
            source_vault: pda(&[b"vault", source.as_ref()]),
            destination,
            destination_vault: pda(&[b"vault", destination.as_ref()]),
            schedule: schedule_of(&admin),
        }
        .to_account_metas(None),
        data: predictions::instruction::ForwardRollover {}.data(),
    }
}

fn park_ix(admin: Pubkey, source_nonce: u64) -> Instruction {
    let source = pda(&[b"event", admin.as_ref(), &source_nonce.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ParkRollover {
            source,
            source_vault: pda(&[b"vault", source.as_ref()]),
            escrow: escrow_of(&admin),
            schedule: schedule_of(&admin),
        }
        .to_account_metas(None),
        data: predictions::instruction::ParkRollover {}.data(),
    }
}

#[tokio::test]
async fn winner_is_paid_once_from_the_vault() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let (create, event, vault) = create_ix(admin.pubkey(), other.pubkey(), 1, now + 3_600);
    send(&mut context, &admin, create).await.unwrap();
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 1, 0);
    send(&mut context, &player, place).await.unwrap();

    let deadline = now + 3_600;
    set_clock(&mut context, deadline + 1);
    let (late_place, _) = place_ix(player.pubkey(), admin.pubkey(), 1, 1);
    assert!(send(&mut context, &player, late_place).await.is_err());
    send(&mut context, &player, reveal_ix(player.pubkey(), admin.pubkey(), 1, 0)).await.unwrap();

    set_clock(&mut context, deadline + 19);
    assert!(
        send(&mut context, &admin, approve_ix(admin.pubkey(), admin.pubkey(), event, vec![b'1'; 7])).await.is_err(),
        "results cannot be approved before the end time"
    );
    set_clock(&mut context, deadline + 20);
    send(&mut context, &admin, approve_ix(admin.pubkey(), admin.pubkey(), event, vec![b'1'; 7])).await.unwrap();
    send(&mut context, &admin, tally_ix(admin.pubkey(), event, &[])).await.expect_err("one approval does not lock results");
    fund(&mut context, &other.pubkey(), 10_000_000).await;
    send(&mut context, &other, approve_ix(other.pubkey(), admin.pubkey(), event, vec![b'1'; 7])).await.unwrap();
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Tallying);
    send(&mut context, &admin, tally_ix(admin.pubkey(), event, &[])).await.unwrap();
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Tallying, "settlement waits for every coupon");
    send(&mut context, &admin, tally_ix(admin.pubkey(), event, &[coupon])).await.unwrap();

    let settled = load_event(&mut context, event).await;
    assert_eq!(settled.status, EventStatus::Resolved);
    assert_eq!(settled.per_winner[6], 950_000);
    assert_eq!(settled.house, 50_000);
    let stranger = Keypair::new();
    fund(&mut context, &stranger.pubkey(), 1_000_000).await;
    let redirected = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Claim { coupon, event, vault, owner: stranger.pubkey() }.to_account_metas(None),
        data: predictions::instruction::Claim {}.data(),
    };
    assert!(send(&mut context, &stranger, redirected).await.is_err(), "a claim cannot be redirected");
    let before = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    let claim = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Claim { coupon, event, vault, owner: player.pubkey() }.to_account_metas(None),
        data: predictions::instruction::Claim {}.data(),
    };
    send(&mut context, &player, claim.clone()).await.unwrap();
    let after = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    assert!(after > before);
    assert!(context.banks_client.get_account(coupon).await.unwrap().is_none());
    // The same recent blockhash would make this a duplicate of the successful claim.
    let slot = context.banks_client.get_root_slot().await.unwrap();
    context.warp_to_slot(slot + 1).unwrap();
    assert!(send(&mut context, &player, claim).await.is_err(), "a claim cannot be repeated");

    let vault_balance = context.banks_client.get_balance(vault).await.unwrap();
    assert_eq!(vault_balance, settled.rent_reserve + settled.house);
}

#[tokio::test]
async fn cancelled_event_refunds_the_entry_price() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let (create, event, vault) = create_ix(admin.pubkey(), other.pubkey(), 2, now + 3_600);
    send(&mut context, &admin, create).await.unwrap();
    let reserve = load_event(&mut context, event).await.rent_reserve;
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 2, 0);
    send(&mut context, &player, place).await.unwrap();
    cancel(&mut context, &admin, &other, event).await;
    let before = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    let refund = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Refund { coupon, event, vault, owner: player.pubkey() }.to_account_metas(None),
        data: predictions::instruction::Refund {}.data(),
    };
    send(&mut context, &player, refund).await.unwrap();
    let after = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    assert!(after > before);
    assert_eq!(context.banks_client.get_balance(vault).await.unwrap(), reserve);
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Cancelled);
}

#[tokio::test]
async fn rollover_reaches_only_the_earliest_future_event() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let (create_a, event_a, vault_a) = create_ix(admin.pubkey(), other.pubkey(), 10, now + 10);
    send(&mut context, &admin, create_a).await.unwrap();
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 10, 0);
    send(&mut context, &player, place).await.unwrap();
    let (create_b, event_b, vault_b) = create_ix(admin.pubkey(), other.pubkey(), 11, now + 500);
    send(&mut context, &admin, create_b).await.unwrap();
    let (create_c, event_c, _) = create_ix(admin.pubkey(), other.pubkey(), 12, now + 5_000);
    send(&mut context, &admin, create_c).await.unwrap();

    set_clock(&mut context, now + 11);
    send(&mut context, &player, reveal_ix(player.pubkey(), admin.pubkey(), 10, 0)).await.unwrap();
    set_clock(&mut context, now + 30);
    resolve(&mut context, &admin, &other, event_a, vec![b'2'; 7], &[coupon]).await;
    let source = load_event(&mut context, event_a).await;
    assert_eq!(source.rollover_out, 950_000);
    assert!(send(&mut context, &admin, park_ix(admin.pubkey(), 10)).await.is_err(), "an open future event must receive the rollover");
    assert!(send(&mut context, &admin, forward_ix(admin.pubkey(), 10, 12)).await.is_err(), "a later event is not the earliest");
    send(&mut context, &admin, forward_ix(admin.pubkey(), 10, 11)).await.unwrap();

    let forwarded = load_event(&mut context, event_a).await;
    let destination = load_event(&mut context, event_b).await;
    assert!(forwarded.rollover_forwarded);
    assert_eq!(destination.rollover_in, 950_000);
    assert_eq!(context.banks_client.get_balance(vault_a).await.unwrap(), source.rent_reserve + source.house);
    assert_eq!(context.banks_client.get_balance(vault_b).await.unwrap(), destination.rent_reserve + 950_000);
    assert_eq!(load_event(&mut context, event_c).await.rollover_in, 0);
}

#[tokio::test]
async fn parked_rollover_is_assigned_to_the_next_event() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let (create_a, event_a, _) = create_ix(admin.pubkey(), other.pubkey(), 20, now + 10);
    send(&mut context, &admin, create_a).await.unwrap();
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 20, 0);
    send(&mut context, &player, place).await.unwrap();
    set_clock(&mut context, now + 11);
    send(&mut context, &player, reveal_ix(player.pubkey(), admin.pubkey(), 20, 0)).await.unwrap();
    set_clock(&mut context, now + 30);
    resolve(&mut context, &admin, &other, event_a, vec![b'2'; 7], &[coupon]).await;
    send(&mut context, &admin, park_ix(admin.pubkey(), 20)).await.unwrap();
    assert_eq!(load_escrow(&mut context, &admin.pubkey()).await.pending, 950_000);

    let intruder = Keypair::new();
    let accomplice = Keypair::new();
    fund(&mut context, &intruder.pubkey(), 2_000_000_000).await;
    let (create_x, event_x, _) = create_ix(intruder.pubkey(), accomplice.pubkey(), 99, now + 400);
    send(&mut context, &intruder, create_x).await.unwrap();
    assert_eq!(load_event(&mut context, event_x).await.rollover_in, 0, "another admin's event must not take parked rollover");
    assert_eq!(load_escrow(&mut context, &admin.pubkey()).await.pending, 950_000);

    let (create_b, event_b, _) = create_ix(admin.pubkey(), other.pubkey(), 21, now + 500);
    send(&mut context, &admin, create_b).await.unwrap();
    assert_eq!(load_event(&mut context, event_b).await.rollover_in, 950_000);
    assert_eq!(load_escrow(&mut context, &admin.pubkey()).await.pending, 0);
    cancel(&mut context, &admin, &other, event_b).await;
    assert_eq!(load_event(&mut context, event_b).await.rollover_out, 950_000);

    let (create_c, event_c, vault_c) = create_ix(admin.pubkey(), other.pubkey(), 22, now + 5_000);
    send(&mut context, &admin, create_c).await.unwrap();
    send(&mut context, &admin, forward_ix(admin.pubkey(), 21, 22)).await.unwrap();
    let destination = load_event(&mut context, event_c).await;
    assert_eq!(destination.rollover_in, 950_000);
    assert_eq!(context.banks_client.get_balance(vault_c).await.unwrap(), destination.rent_reserve + 950_000);
}

#[tokio::test]
async fn forwarding_rollover_leaves_unclaimed_winnings() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let mut args = event_args(30, now + 10);
    args.tiers = vec![TierArg { correct: 7, percent: 50 }, TierArg { correct: 6, percent: 45 }];
    args.signers = vec![admin.pubkey(), other.pubkey()];
    let event = pda(&[b"event", admin.pubkey().as_ref(), &30u64.to_le_bytes()]);
    let vault = pda(&[b"vault", event.as_ref()]);
    let create = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::CreateEvent {
            admin: admin.pubkey(),
            event,
            vault,
            escrow: escrow_of(&admin.pubkey()),
            schedule: schedule_of(&admin.pubkey()),
            signers: signers_of(&event),
            gate: gate_of(&admin.pubkey()),
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::CreateEvent { args }.data(),
    };
    send(&mut context, &admin, create).await.unwrap();
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 30, 0);
    send(&mut context, &player, place).await.unwrap();
    let (next, _, _) = create_ix(admin.pubkey(), other.pubkey(), 31, now + 500);
    send(&mut context, &admin, next).await.unwrap();

    set_clock(&mut context, now + 11);
    send(&mut context, &player, reveal_ix(player.pubkey(), admin.pubkey(), 30, 0)).await.unwrap();
    set_clock(&mut context, now + 30);
    resolve(&mut context, &admin, &other, event, vec![b'1'; 7], &[coupon]).await;
    let settled = load_event(&mut context, event).await;
    assert_eq!(settled.per_winner[6], 500_000);
    assert_eq!(settled.rollover_out, 450_000);
    send(&mut context, &admin, forward_ix(admin.pubkey(), 30, 31)).await.unwrap();
    assert_eq!(
        context.banks_client.get_balance(vault).await.unwrap(),
        settled.rent_reserve + settled.house + settled.paid
    );

    let before = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    let claim = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Claim { coupon, event, vault, owner: player.pubkey() }.to_account_metas(None),
        data: predictions::instruction::Claim {}.data(),
    };
    send(&mut context, &player, claim).await.unwrap();
    let after = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    assert!(after > before);
    assert_eq!(context.banks_client.get_balance(vault).await.unwrap(), settled.rent_reserve + settled.house);
}

#[tokio::test]
async fn one_signer_cannot_resolve_and_pause_spares_claims() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let deadline = now + 100;
    let (create, event, vault) = create_ix(admin.pubkey(), other.pubkey(), 40, deadline);
    send(&mut context, &admin, create).await.unwrap();
    let pause = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::SetPause { admin: admin.pubkey(), gate: gate_of(&admin.pubkey()) }.to_account_metas(None),
        data: predictions::instruction::SetPause { paused: true }.data(),
    };
    send(&mut context, &admin, pause.clone()).await.unwrap();
    let (blocked, _) = place_ix(player.pubkey(), admin.pubkey(), 40, 0);
    assert!(send(&mut context, &player, blocked).await.is_err(), "pause stops new coupons");
    let resume = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::SetPause { admin: admin.pubkey(), gate: gate_of(&admin.pubkey()) }.to_account_metas(None),
        data: predictions::instruction::SetPause { paused: false }.data(),
    };
    send(&mut context, &admin, resume).await.unwrap();
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 40, 0);
    send(&mut context, &player, place).await.unwrap();
    set_clock(&mut context, deadline + 1);
    send(&mut context, &player, reveal_ix(player.pubkey(), admin.pubkey(), 40, 0)).await.unwrap();
    set_clock(&mut context, deadline + 20);
    let outsider = Keypair::new();
    fund(&mut context, &outsider.pubkey(), 10_000_000).await;
    assert!(
        send(&mut context, &outsider, approve_ix(outsider.pubkey(), admin.pubkey(), event, vec![b'1'; 7])).await.is_err(),
        "only a listed result signer can approve"
    );
    send(&mut context, &admin, approve_ix(admin.pubkey(), admin.pubkey(), event, vec![b'1'; 7])).await.unwrap();
    // The second signer disagrees, which restarts the count instead of locking anything.
    fund(&mut context, &other.pubkey(), 10_000_000).await;
    send(&mut context, &other, approve_ix(other.pubkey(), admin.pubkey(), event, vec![b'2'; 7])).await.unwrap();
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Open, "conflicting approvals must not lock results");
    assert_eq!(load_event(&mut context, event).await.approvals.count_ones(), 1);
    send(&mut context, &admin, approve_ix(admin.pubkey(), admin.pubkey(), event, vec![b'1'; 7])).await.unwrap();
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Open, "one signer is not enough");
    resolve(&mut context, &admin, &other, event, vec![b'1'; 7], &[coupon]).await;
    send(&mut context, &admin, pause).await.unwrap();
    let claim = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Claim { coupon, event, vault, owner: player.pubkey() }.to_account_metas(None),
        data: predictions::instruction::Claim {}.data(),
    };
    send(&mut context, &player, claim).await.unwrap();
}

#[tokio::test]
async fn anyone_can_refund_after_the_resolution_timeout() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    let stranger = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    fund(&mut context, &stranger.pubkey(), 1_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let deadline = now + 50;
    let (create, event, vault) = create_ix(admin.pubkey(), other.pubkey(), 41, deadline);
    send(&mut context, &admin, create).await.unwrap();
    let (place, coupon) = place_ix(player.pubkey(), admin.pubkey(), 41, 0);
    send(&mut context, &player, place).await.unwrap();
    let expire = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ExpireEvent { caller: stranger.pubkey(), event, schedule: schedule_of(&admin.pubkey()) }.to_account_metas(None),
        data: predictions::instruction::ExpireEvent {}.data(),
    };
    assert!(send(&mut context, &stranger, expire.clone()).await.is_err(), "timeout has not arrived");
    set_clock(&mut context, deadline + 100_000);
    send(&mut context, &stranger, expire).await.unwrap();
    let refund = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Refund { coupon, event, vault, owner: player.pubkey() }.to_account_metas(None),
        data: predictions::instruction::Refund {}.data(),
    };
    let before = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    send(&mut context, &player, refund).await.unwrap();
    assert!(context.banks_client.get_balance(player.pubkey()).await.unwrap() > before);
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Cancelled);
}

fn withdraw_house_ix(admin: Pubkey, event: Pubkey) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::WithdrawHouse { admin, event, vault: pda(&[b"vault", event.as_ref()]) }.to_account_metas(None),
        data: predictions::instruction::WithdrawHouse {}.data(),
    }
}

#[tokio::test]
async fn batched_tally_settles_a_full_thirteen_game_event() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    let stranger = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    fund(&mut context, &stranger.pubkey(), 10_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let deadline = now + 100;
    let nonce = 50u64;
    let mut args = event_args(nonce, deadline);
    args.game_count = 13;
    args.signers = vec![admin.pubkey(), other.pubkey()];
    args.tiers = vec![
        TierArg { correct: 13, percent: 50 },
        TierArg { correct: 12, percent: 25 },
        TierArg { correct: 11, percent: 15 },
        TierArg { correct: 10, percent: 5 },
    ];
    let event = pda(&[b"event", admin.pubkey().as_ref(), &nonce.to_le_bytes()]);
    let vault = pda(&[b"vault", event.as_ref()]);
    let create = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::CreateEvent {
            admin: admin.pubkey(),
            event,
            vault,
            escrow: escrow_of(&admin.pubkey()),
            schedule: schedule_of(&admin.pubkey()),
            signers: signers_of(&event),
            gate: gate_of(&admin.pubkey()),
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::CreateEvent { args }.data(),
    };
    send(&mut context, &admin, create).await.unwrap();

    let home = [0b001u8; 13];
    let away = [0b100u8; 13];
    let count = 35u32;
    let mut coupons = Vec::new();
    for index in 0..count {
        let picks = if index == 0 { home } else { away };
        let (place, coupon) = place_picks_ix(player.pubkey(), admin.pubkey(), nonce, index, picks, 1);
        send(&mut context, &player, place).await.unwrap();
        coupons.push(coupon);
    }
    let first = context.banks_client.get_account(coupons[3]).await.unwrap().unwrap();
    let stored = predictions::Coupon::try_deserialize(&mut first.data.as_slice()).unwrap();
    assert_eq!(stored.sealed, [3u8; predictions::SEALED_LEN], "the sealed backup is stored as given");

    set_clock(&mut context, deadline + 1);
    for index in 0..count {
        // Coupon 1 is never revealed: its stake stays in the pool and it cannot win.
        if index == 1 {
            continue;
        }
        let picks = if index == 0 { home } else { away };
        send(&mut context, &player, reveal_picks_ix(player.pubkey(), admin.pubkey(), nonce, index, picks)).await.unwrap();
    }

    set_clock(&mut context, deadline + 20);
    let mut results = vec![b'1'; 13];
    results[12] = b'C';
    send(&mut context, &admin, approve_cancel_ix(admin.pubkey(), admin.pubkey(), event)).await.unwrap();
    assert_eq!(load_event(&mut context, event).await.status, EventStatus::Open, "one cancel approval is not enough");
    send(&mut context, &admin, approve_ix(admin.pubkey(), admin.pubkey(), event, results.clone())).await.unwrap();
    fund(&mut context, &other.pubkey(), 10_000_000).await;
    send(&mut context, &other, approve_ix(other.pubkey(), admin.pubkey(), event, results.clone())).await.unwrap();
    assert!(
        send(&mut context, &other, approve_cancel_ix(other.pubkey(), admin.pubkey(), event)).await.is_err(),
        "locked results cannot be cancelled"
    );
    assert!(
        send(&mut context, &player, reveal_picks_ix(player.pubkey(), admin.pubkey(), nonce, 1, away)).await.is_err(),
        "no reveal after results are locked"
    );
    assert!(
        send(&mut context, &stranger, tally_ix(admin.pubkey(), event, &coupons[1..3])).await.is_err(),
        "coupons must be tallied in index order"
    );
    for batch in coupons.chunks(10) {
        send(&mut context, &stranger, tally_ix(admin.pubkey(), event, batch)).await.unwrap();
    }

    let settled = load_event(&mut context, event).await;
    assert_eq!(settled.status, EventStatus::Resolved);
    assert_eq!(settled.tallied, count);
    assert_eq!(settled.winners[13], 1, "the void game counts as correct for the home coupon");
    assert_eq!(settled.winners[1], 33, "away coupons only score the void game");
    let sales = u64::from(count) * 1_000_000;
    assert_eq!(settled.per_winner[12], sales / 2);
    assert_eq!(settled.rollover_out, sales * 45 / 100);
    assert_eq!(settled.house, sales * 5 / 100);

    let expire = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ExpireEvent { caller: stranger.pubkey(), event, schedule: schedule_of(&admin.pubkey()) }.to_account_metas(None),
        data: predictions::instruction::ExpireEvent {}.data(),
    };
    set_clock(&mut context, deadline + 200_000);
    assert!(send(&mut context, &stranger, expire).await.is_err(), "a resolved event cannot expire");

    let before = context.banks_client.get_balance(player.pubkey()).await.unwrap();
    send(&mut context, &player, claim_ix(coupons[0], event, player.pubkey())).await.unwrap();
    assert!(context.banks_client.get_balance(player.pubkey()).await.unwrap() > before + sales / 2 - 10_000_000);
    send(&mut context, &player, claim_ix(coupons[1], event, player.pubkey())).await.unwrap();
    assert!(context.banks_client.get_account(coupons[1]).await.unwrap().is_none(), "an unrevealed coupon closes with no payout");

    assert!(send(&mut context, &stranger, withdraw_house_ix(stranger.pubkey(), event)).await.is_err(), "only the event admin takes the house share");
    let admin_before = context.banks_client.get_balance(admin.pubkey()).await.unwrap();
    send(&mut context, &admin, withdraw_house_ix(admin.pubkey(), event)).await.unwrap();
    assert!(context.banks_client.get_balance(admin.pubkey()).await.unwrap() > admin_before);
    assert!(send(&mut context, &admin, withdraw_house_ix(admin.pubkey(), event)).await.is_err(), "the house share is taken once");
    assert_eq!(
        context.banks_client.get_balance(vault).await.unwrap(),
        settled.rent_reserve + settled.rollover_out,
        "only the unforwarded rollover remains"
    );
}

#[tokio::test]
async fn house_share_waits_for_resolution() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let player = Keypair::new();
    fund(&mut context, &player.pubkey(), 2_000_000_000).await;
    let now = clock(&mut context).await.unix_timestamp;
    let (create, event, _) = create_ix(admin.pubkey(), other.pubkey(), 60, now + 100);
    send(&mut context, &admin, create).await.unwrap();
    let (place, _) = place_ix(player.pubkey(), admin.pubkey(), 60, 0);
    send(&mut context, &player, place).await.unwrap();
    assert!(send(&mut context, &admin, withdraw_house_ix(admin.pubkey(), event)).await.is_err(), "no house share while open");
    cancel(&mut context, &admin, &other, event).await;
    assert!(send(&mut context, &admin, withdraw_house_ix(admin.pubkey(), event)).await.is_err(), "a cancelled event has no house share");
}

#[tokio::test]
async fn upgrade_is_public_and_delayed() {
    let mut context = start().await;
    let admin = Keypair::try_from(context.payer.to_bytes().as_slice()).unwrap();
    let other = Keypair::new();
    let council = pda(&[b"upgrade-council"]);
    let text = std::fs::read_to_string("keys/deployer-keypair.json").unwrap();
    let secret: Vec<u8> = text.split(|c: char| !c.is_ascii_digit()).filter(|part| !part.is_empty()).map(|part| part.parse().unwrap()).collect();
    let bootstrap = Keypair::try_from(secret.as_slice()).unwrap();
    fund(&mut context, &bootstrap.pubkey(), 2_000_000_000).await;
    let initialize = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::InitializeUpgrade { payer: bootstrap.pubkey(), council, system_program: solana_sdk::system_program::ID }.to_account_metas(None),
        data: predictions::instruction::InitializeUpgrade { threshold: 2, signers: vec![admin.pubkey(), other.pubkey()] }.data(),
    };
    send(&mut context, &bootstrap, initialize).await.unwrap();

    let buffer = Keypair::new().pubkey();
    let spill = admin.pubkey();
    let mut schedule = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ScheduleUpgrade { council, buffer, spill }.to_account_metas(None),
        data: predictions::instruction::ScheduleUpgrade { signer_count: 1 }.data(),
    };
    schedule.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(admin.pubkey(), true));
    assert!(send(&mut context, &admin, schedule.clone()).await.is_err(), "one signer cannot schedule an upgrade");
    schedule.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(other.pubkey(), true));
    schedule.data = predictions::instruction::ScheduleUpgrade { signer_count: 2 }.data();
    send_all(&mut context, &[&admin, &other], schedule.clone()).await.unwrap();

    let loader = Pubkey::from_str("BPFLoaderUpgradeab1e11111111111111111111111").unwrap();
    let execute = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ExecuteUpgrade {
            council,
            program_data: Pubkey::find_program_address(&[predictions::ID.as_ref()], &loader).0,
            program_account: predictions::ID,
            buffer,
            spill,
            rent: solana_sdk::sysvar::rent::ID,
            clock: solana_sdk::sysvar::clock::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::ExecuteUpgrade {}.data(),
    };
    let early = format!("{:?}", send(&mut context, &admin, execute.clone()).await.unwrap_err());
    assert!(early.contains("6029"), "an upgrade inside the delay must fail the timelock: {early}");

    #[cfg(feature = "devnet-testing")]
    {
        let mut override_delay = Instruction {
            program_id: predictions::ID,
            accounts: predictions::accounts::OverrideUpgradeDelay { council }.to_account_metas(None),
            data: predictions::instruction::OverrideUpgradeDelay { signer_count: 1 }.data(),
        };
        override_delay.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(admin.pubkey(), true));
        assert!(
            send(&mut context, &admin, override_delay.clone()).await.is_err(),
            "one signer cannot override the devnet delay"
        );
        override_delay.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(other.pubkey(), true));
        override_delay.data = predictions::instruction::OverrideUpgradeDelay { signer_count: 2 }.data();
        send_all(&mut context, &[&admin, &other], override_delay).await.unwrap();
        let bypassed = format!("{:?}", send(&mut context, &admin, execute.clone()).await.unwrap_err());
        assert!(!bypassed.contains("6029"), "all council members can override the devnet delay: {bypassed}");

        send_all(&mut context, &[&admin, &other], schedule).await.unwrap();
        let early_again = format!("{:?}", send(&mut context, &admin, execute.clone()).await.unwrap_err());
        assert!(early_again.contains("6029"), "a new proposal must restart the delay: {early_again}");
        let now = clock(&mut context).await.unix_timestamp;
        let eta = load_council(&mut context, council).await.proposal_eta;
        assert!(eta > now && eta - now <= 5 * 60, "devnet delay must be five minutes");
        let mut next = clock(&mut context).await;
        next.unix_timestamp = eta + 1;
        context.set_sysvar(&next);
        let late = format!("{:?}", send(&mut context, &admin, execute).await.unwrap_err());
        assert!(!late.contains("6029"), "after five minutes the devnet timelock opens: {late}");
    }

    #[cfg(not(feature = "devnet-testing"))]
    {
        let now = clock(&mut context).await.unix_timestamp;
        let eta = load_council(&mut context, council).await.proposal_eta;
        assert!(eta > now && eta - now <= 24 * 60 * 60, "production delay must be one day");
        let mut next = clock(&mut context).await;
        next.unix_timestamp = eta + 1;
        context.set_sysvar(&next);
        assert!(clock(&mut context).await.unix_timestamp > eta, "test clock must pass the proposal ETA");
        let late = format!("{:?}", send(&mut context, &admin, execute).await.unwrap_err());
        assert!(!late.contains("6029"), "after one day the timelock opens and the loader is reached: {late}");
    }
}
