use std::convert::TryFrom;

use anchor_lang::{AccountDeserialize, InstructionData, ToAccountMetas};
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

async fn send_all(context: &mut ProgramTestContext, signers: &[&Keypair], instruction: Instruction) -> Result<(), solana_program_test::BanksClientError> {
    let blockhash = context.banks_client.get_latest_blockhash().await.unwrap();
    let transaction = Transaction::new_signed_with_payer(&[instruction], Some(&signers[0].pubkey()), signers, blockhash);
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
        end_time: deadline + 3_600,
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

fn signers_of(event: &Pubkey) -> Pubkey {
    pda(&[b"signers", event.as_ref()])
}

fn create_ix(admin: Pubkey, other: Pubkey, nonce: u64, deadline: i64) -> (Instruction, Pubkey, Pubkey) {
    let mut args = event_args(nonce, deadline);
    args.signers = vec![admin, other];
    let event = pda(&[b"event", admin.as_ref(), &nonce.to_le_bytes()]);
    let vault = pda(&[b"vault", event.as_ref()]);
    let escrow = pda(&[b"rollover-escrow"]);
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
            commitment: predictions::coupon_commitment(&player, &all_home(), REVEAL_NONCE),
            combinations: 1,
        }
        .data(),
    };
    (instruction, coupon)
}

fn reveal_ix(player: Pubkey, admin: Pubkey, nonce: u64, index: u32) -> Instruction {
    let event = pda(&[b"event", admin.as_ref(), &nonce.to_le_bytes()]);
    let coupon = pda(&[b"coupon", event.as_ref(), &index.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::RevealCoupon { player, coupon, event }.to_account_metas(None),
        data: predictions::instruction::RevealCoupon { picks: all_home(), nonce: REVEAL_NONCE }.data(),
    }
}

async fn start() -> ProgramTestContext {
    ProgramTest::new("predictions", predictions::ID, processor!(process_instruction)).start_with_context().await
}

async fn load_event(context: &mut ProgramTestContext, event: Pubkey) -> Event {
    let account = context.banks_client.get_account(event).await.unwrap().unwrap();
    Event::try_deserialize(&mut account.data.as_slice()).unwrap()
}

async fn load_escrow(context: &mut ProgramTestContext) -> Escrow {
    let escrow = pda(&[b"rollover-escrow"]);
    let account = context.banks_client.get_account(escrow).await.unwrap().unwrap();
    Escrow::try_deserialize(&mut account.data.as_slice()).unwrap()
}

async fn load_council(context: &mut ProgramTestContext, council: Pubkey) -> UpgradeCouncil {
    let account = context.banks_client.get_account(council).await.unwrap().unwrap();
    UpgradeCouncil::try_deserialize(&mut account.data.as_slice()).unwrap()
}

fn resolve_ix(admin: Pubkey, other: Pubkey, event: Pubkey, results: Vec<u8>) -> Instruction {
    let mut instruction = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ResolveEvent {
            event,
            signers: signers_of(&event),
            vault: pda(&[b"vault", event.as_ref()]),
            schedule: schedule_of(&admin),
        }
        .to_account_metas(None),
        data: predictions::instruction::ResolveEvent { results, signer_count: 2 }.data(),
    };
    instruction.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(admin, true));
    instruction.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(other, true));
    instruction
}

fn cancel_ix(admin: Pubkey, other: Pubkey, event: Pubkey) -> Instruction {
    let mut instruction = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::CancelEvent { event, signers: signers_of(&event), schedule: schedule_of(&admin) }.to_account_metas(None),
        data: predictions::instruction::CancelEvent { signer_count: 2 }.data(),
    };
    instruction.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(admin, true));
    instruction.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(other, true));
    instruction
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
            escrow: pda(&[b"rollover-escrow"]),
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

    set_clock(&mut context, deadline + 20);
    let mut resolve = resolve_ix(admin.pubkey(), other.pubkey(), event, vec![b'1'; 7]);
    assert!(send_all(&mut context, &[&admin, &other], resolve.clone()).await.is_err(), "resolution must include every coupon");
    resolve.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(coupon, false));
    send_all(&mut context, &[&admin, &other], resolve).await.unwrap();

    let settled = load_event(&mut context, event).await;
    assert_eq!(settled.status, EventStatus::Resolved);
    assert_eq!(settled.per_winner[7], 950_000);
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
    send_all(&mut context, &[&admin, &other], cancel_ix(admin.pubkey(), other.pubkey(), event)).await.unwrap();
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
    let mut resolve = resolve_ix(admin.pubkey(), other.pubkey(), event_a, vec![b'2'; 7]);
    resolve.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(coupon, false));
    send_all(&mut context, &[&admin, &other], resolve).await.unwrap();
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
    let mut resolve = resolve_ix(admin.pubkey(), other.pubkey(), event_a, vec![b'2'; 7]);
    resolve.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(coupon, false));
    send_all(&mut context, &[&admin, &other], resolve).await.unwrap();
    send(&mut context, &admin, park_ix(admin.pubkey(), 20)).await.unwrap();
    assert_eq!(load_escrow(&mut context).await.pending, 950_000);

    let (create_b, event_b, _) = create_ix(admin.pubkey(), other.pubkey(), 21, now + 500);
    send(&mut context, &admin, create_b).await.unwrap();
    assert_eq!(load_event(&mut context, event_b).await.rollover_in, 950_000);
    assert_eq!(load_escrow(&mut context).await.pending, 0);
    send_all(&mut context, &[&admin, &other], cancel_ix(admin.pubkey(), other.pubkey(), event_b)).await.unwrap();
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
            escrow: pda(&[b"rollover-escrow"]),
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
    let mut resolve = resolve_ix(admin.pubkey(), other.pubkey(), event, vec![b'1'; 7]);
    resolve.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(coupon, false));
    send_all(&mut context, &[&admin, &other], resolve).await.unwrap();
    let settled = load_event(&mut context, event).await;
    assert_eq!(settled.per_winner[7], 500_000);
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
    let mut alone = resolve_ix(admin.pubkey(), other.pubkey(), event, vec![b'1'; 7]);
    alone.accounts.pop();
    alone.data = predictions::instruction::ResolveEvent { results: vec![b'1'; 7], signer_count: 1 }.data();
    alone.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(coupon, false));
    assert!(send(&mut context, &admin, alone).await.is_err(), "one result signer is not enough");
    let mut resolve = resolve_ix(admin.pubkey(), other.pubkey(), event, vec![b'1'; 7]);
    resolve.accounts.push(solana_sdk::instruction::AccountMeta::new_readonly(coupon, false));
    send_all(&mut context, &[&admin, &other], resolve).await.unwrap();
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

    let execute = Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::ExecuteUpgrade {
            council,
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
