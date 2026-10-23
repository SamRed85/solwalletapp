//! Place, reveal, resolve, and claim against the built program on a local validator.
//! The backend is not involved: the vault pays the winning wallet directly.

use std::io::{Read, Write};
use std::net::TcpStream;
use std::str::FromStr;
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anchor_lang::{AccountDeserialize, InstructionData, ToAccountMetas};
use predictions::{CreateEventArgs, Event, EventStatus, TierArg};
use solana_sdk::{
    instruction::{AccountMeta, Instruction},
    pubkey::Pubkey,
    signature::{Keypair, Signer},
    transaction::Transaction,
};

const PROGRAM: &str = "H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr";
const CLOCK: &str = "SysvarC1ock11111111111111111111111111111111";
const REVEAL_NONCE: u64 = 7;

struct Rpc {
    host: String,
    port: u16,
}

fn main() {
    assert_eq!(PROGRAM, predictions::ID.to_string(), "example program id drifted");
    let url = std::env::args().nth(2).unwrap_or_else(|| "http://127.0.0.1:8899".to_string());
    let rpc = Rpc::parse(&url);
    let admin = Keypair::new();
    let other = Keypair::new();
    let player = Keypair::new();
    let stranger = Keypair::new();
    for key in [&admin, &player, &stranger] {
        rpc.airdrop(&key.pubkey(), 1_000_000_000);
    }

    let now = rpc.unix_time();
    let deadline = now + 12;
    let reveal_deadline = deadline + 40;
    let nonce = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
    let event = pda(&[b"event", admin.pubkey().as_ref(), &nonce.to_le_bytes()]);
    let vault = pda(&[b"vault", event.as_ref()]);
    let coupon = pda(&[b"coupon", event.as_ref(), &0u32.to_le_bytes()]);

    rpc.send(&admin, &[create_ix(&admin, &other, nonce, deadline, reveal_deadline)], true).unwrap();
    rpc.send(&player, &[place_ix(&player, &admin, nonce, 0)], true).unwrap();
    rpc.wait_until(deadline);
    assert!(rpc.send(&player, &[place_ix(&player, &admin, nonce, 1)], true).is_err(), "a late entry must be rejected");
    rpc.send(&player, &[reveal_ix(&player, &admin, nonce, 0)], true).unwrap();
    rpc.wait_until(reveal_deadline);

    rpc.send(&admin, &[approve_ix(&admin, &admin.pubkey(), event)], true).unwrap();
    assert_eq!(rpc.event(&event).status, EventStatus::Open, "one signer must not lock results");
    rpc.send_all(&[&player, &other], &[approve_ix(&other, &admin.pubkey(), event)], true).unwrap();
    assert_eq!(rpc.event(&event).status, EventStatus::Tallying);
    let mut tally = tally_ix(&admin, event);
    tally.accounts.push(AccountMeta::new_readonly(coupon, false));
    rpc.send(&stranger, &[tally], true).unwrap();

    let settled = rpc.event(&event);
    assert_eq!(settled.status, EventStatus::Resolved);
    assert_eq!(settled.per_winner[6], 950_000);
    assert_eq!(settled.house, 50_000);

    let claim = claim_ix(coupon, event, vault, player.pubkey());
    let redirected = claim_ix(coupon, event, vault, stranger.pubkey());
    assert!(rpc.send(&stranger, &[redirected], true).is_err(), "a claim cannot be redirected");
    let before = rpc.balance(&player.pubkey());
    rpc.send(&player, &[claim.clone()], true).unwrap();
    let after = rpc.balance(&player.pubkey());
    assert!(after > before, "the vault must pay the winner");
    assert!(rpc.account(&coupon).is_none(), "a paid coupon account must close");
    assert!(rpc.send(&player, &[claim], true).is_err(), "a claim cannot be repeated");
    assert_eq!(rpc.balance(&vault), settled.rent_reserve + settled.house);

    println!("place, win, and claim succeeded on the local validator");
    println!("player gained {} lamports and the vault kept the house share", after - before);
    println!("program {PROGRAM}");
}

fn create_ix(admin: &Keypair, other: &Keypair, nonce: u64, deadline: i64, reveal_deadline: i64) -> Instruction {
    let admin_key = admin.pubkey();
    let event = pda(&[b"event", admin_key.as_ref(), &nonce.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::CreateEvent {
            admin: admin_key,
            event,
            vault: pda(&[b"vault", event.as_ref()]),
            escrow: pda(&[b"rollover-escrow", admin_key.as_ref()]),
            schedule: pda(&[b"schedule", admin_key.as_ref()]),
            signers: pda(&[b"signers", event.as_ref()]),
            gate: pda(&[b"gate", admin_key.as_ref()]),
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::CreateEvent {
            args: CreateEventArgs {
                nonce,
                game_count: 7,
                price: 1_000_000,
                deadline,
                reveal_deadline,
                end_time: reveal_deadline,
                resolve_timeout: reveal_deadline + 10_000,
                threshold: 2,
                signers: vec![admin_key, other.pubkey()],
                tiers: vec![TierArg { correct: 7, percent: 95 }],
            },
        }
        .data(),
    }
}

fn picks() -> [u8; 13] {
    let mut picks = [0u8; 13];
    picks[..7].fill(0b001);
    picks
}

fn place_ix(player: &Keypair, admin: &Keypair, nonce: u64, index: u32) -> Instruction {
    let event = pda(&[b"event", admin.pubkey().as_ref(), &nonce.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::PlaceCoupon {
            player: player.pubkey(),
            event,
            vault: pda(&[b"vault", event.as_ref()]),
            coupon: pda(&[b"coupon", event.as_ref(), &index.to_le_bytes()]),
            gate: pda(&[b"gate", admin.pubkey().as_ref()]),
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::PlaceCoupon {
            commitment: predictions::coupon_commitment(&player.pubkey(), &picks(), REVEAL_NONCE),
            combinations: 1,
            sealed: [0; predictions::SEALED_LEN],
        }
        .data(),
    }
}

fn reveal_ix(player: &Keypair, admin: &Keypair, nonce: u64, index: u32) -> Instruction {
    let event = pda(&[b"event", admin.pubkey().as_ref(), &nonce.to_le_bytes()]);
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::RevealCoupon {
            player: player.pubkey(),
            coupon: pda(&[b"coupon", event.as_ref(), &index.to_le_bytes()]),
            event,
        }
        .to_account_metas(None),
        data: predictions::instruction::RevealCoupon { picks: picks(), nonce: REVEAL_NONCE }.data(),
    }
}

fn approve_ix(signer: &Keypair, admin: &Pubkey, event: Pubkey) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Approve {
            signer: signer.pubkey(),
            event,
            signers: pda(&[b"signers", event.as_ref()]),
            schedule: pda(&[b"schedule", admin.as_ref()]),
        }
        .to_account_metas(None),
        data: predictions::instruction::ApproveResolution { results: vec![b'1'; 7] }.data(),
    }
}

fn tally_ix(admin: &Keypair, event: Pubkey) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Tally {
            event,
            vault: pda(&[b"vault", event.as_ref()]),
            schedule: pda(&[b"schedule", admin.pubkey().as_ref()]),
        }
        .to_account_metas(None),
        data: predictions::instruction::Tally {}.data(),
    }
}

fn claim_ix(coupon: Pubkey, event: Pubkey, vault: Pubkey, owner: Pubkey) -> Instruction {
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::Claim { coupon, event, vault, owner }.to_account_metas(None),
        data: predictions::instruction::Claim {}.data(),
    }
}

fn pda(seeds: &[&[u8]]) -> Pubkey {
    Pubkey::find_program_address(seeds, &predictions::ID).0
}

impl Rpc {
    fn parse(url: &str) -> Self {
        let rest = url.trim_start_matches("http://");
        let (host, port) = rest.split_once(':').unwrap_or((rest, "8899"));
        Self { host: host.to_string(), port: port.parse().unwrap() }
    }

    fn rpc(&self, method: &str, params: serde_json::Value) -> serde_json::Value {
        let body = serde_json::json!({"jsonrpc":"2.0","id":1,"method":method,"params":params}).to_string();
        let request = format!(
            "POST / HTTP/1.1\r\nHost: {}:{}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            self.host, self.port, body.len(), body
        );
        let mut stream = TcpStream::connect((self.host.as_str(), self.port)).unwrap_or_else(|err| panic!("validator is not reachable: {err}"));
        stream.set_read_timeout(Some(Duration::from_secs(20))).unwrap();
        stream.write_all(request.as_bytes()).unwrap();
        let mut raw = Vec::new();
        stream.read_to_end(&mut raw).unwrap();
        let text = String::from_utf8_lossy(&raw);
        let json = http_body(&text);
        let value: serde_json::Value = serde_json::from_str(json.trim()).unwrap_or_else(|err| panic!("rpc {method} was not json ({err}): {json}"));
        if let Some(error) = value.get("error") {
            panic!("{method} failed: {error}");
        }
        value["result"].clone()
    }

    fn balance(&self, key: &Pubkey) -> u64 {
        self.rpc("getBalance", serde_json::json!([key.to_string(), {"commitment":"confirmed"}]))["value"].as_u64().unwrap()
    }

    fn account(&self, key: &Pubkey) -> Option<Vec<u8>> {
        let result = self.rpc("getAccountInfo", serde_json::json!([key.to_string(), {"encoding":"base64","commitment":"confirmed"}]));
        let encoded = result["value"]["data"][0].as_str()?;
        Some(base64_decode(encoded))
    }

    fn event(&self, key: &Pubkey) -> Event {
        let data = self.account(key).unwrap_or_else(|| panic!("event account is missing"));
        Event::try_deserialize(&mut data.as_slice()).unwrap()
    }

    fn unix_time(&self) -> i64 {
        let data = self.account(&Pubkey::from_str(CLOCK).unwrap()).unwrap();
        i64::from_le_bytes(data[32..40].try_into().unwrap())
    }

    fn wait_until(&self, timestamp: i64) {
        for _ in 0..90 {
            if self.unix_time() >= timestamp {
                return;
            }
            thread::sleep(Duration::from_secs(1));
        }
        panic!("clock did not reach {timestamp}");
    }

    fn airdrop(&self, key: &Pubkey, lamports: u64) {
        let signature = self.rpc("requestAirdrop", serde_json::json!([key.to_string(), lamports])).as_str().unwrap().to_string();
        self.confirm(&signature);
    }

    fn confirm(&self, signature: &str) {
        for _ in 0..40 {
            let status = &self.rpc(
                "getSignatureStatuses",
                serde_json::json!([[signature], {"searchTransactionHistory": true}]),
            )["value"][0];
            if status["err"].is_object() || status["err"].is_string() {
                panic!("transaction {signature} failed: {status}");
            }
            if status["confirmationStatus"].as_str().is_some_and(|level| level == "confirmed" || level == "finalized") {
                return;
            }
            thread::sleep(Duration::from_millis(500));
        }
        panic!("transaction {signature} was not confirmed");
    }

    fn send(&self, payer: &Keypair, instructions: &[Instruction], expect_ok: bool) -> Result<(), String> {
        self.send_all(&[payer], instructions, expect_ok)
    }

    fn send_all(&self, signers: &[&Keypair], instructions: &[Instruction], expect_ok: bool) -> Result<(), String> {
        let blockhash = self.rpc("getLatestBlockhash", serde_json::json!([{"commitment":"confirmed"}]))["value"]["blockhash"]
            .as_str()
            .unwrap()
            .to_string();
        let transaction = Transaction::new_signed_with_payer(
            instructions,
            Some(&signers[0].pubkey()),
            signers,
            solana_sdk::hash::Hash::from_str(&blockhash).unwrap(),
        );
        let encoded = base64_encode(&bincode::serialize(&transaction).unwrap());
        let body = serde_json::json!({
            "jsonrpc":"2.0","id":1,"method":"sendTransaction",
            "params":[encoded, {"encoding":"base64","skipPreflight": !expect_ok,"preflightCommitment":"confirmed"}]
        }).to_string();
        let request = format!(
            "POST / HTTP/1.1\r\nHost: {}:{}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            self.host, self.port, body.len(), body
        );
        let mut stream = TcpStream::connect((self.host.as_str(), self.port)).unwrap();
        stream.set_read_timeout(Some(Duration::from_secs(30))).unwrap();
        stream.write_all(request.as_bytes()).unwrap();
        let mut raw = Vec::new();
        stream.read_to_end(&mut raw).unwrap();
        let text = String::from_utf8_lossy(&raw);
        let json = http_body(&text);
        let value: serde_json::Value = serde_json::from_str(json.trim()).unwrap_or_else(|err| panic!("send was not json ({err}): {json}"));
        if let Some(error) = value.get("error") {
            return Err(error.to_string());
        }
        if !expect_ok {
            return Err("transaction was accepted".to_string());
        }
        self.confirm(value["result"].as_str().unwrap());
        Ok(())
    }
}

fn http_body(raw: &str) -> String {
    let Some((head, body)) = raw.split_once("\r\n\r\n") else {
        panic!("rpc response had no body: {raw}");
    };
    if !head.to_ascii_lowercase().contains("transfer-encoding: chunked") {
        return body.to_string();
    }
    let mut out = String::new();
    let mut rest = body;
    loop {
        let Some((size, next)) = rest.split_once("\r\n") else { break };
        let size = usize::from_str_radix(size.trim(), 16).unwrap_or(0);
        if size == 0 || next.len() < size {
            break;
        }
        out.push_str(&next[..size]);
        rest = &next[size..];
        rest = rest.strip_prefix("\r\n").unwrap_or(rest);
    }
    out
}

fn base64_encode(data: &[u8]) -> String {
    const TABLE: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::new();
    for chunk in data.chunks(3) {
        let mut n = (chunk[0] as u32) << 16;
        if chunk.len() > 1 {
            n |= (chunk[1] as u32) << 8;
        }
        if chunk.len() > 2 {
            n |= chunk[2] as u32;
        }
        out.push(TABLE[(n >> 18) as usize] as char);
        out.push(TABLE[((n >> 12) & 63) as usize] as char);
        out.push(if chunk.len() > 1 { TABLE[((n >> 6) & 63) as usize] as char } else { '=' });
        out.push(if chunk.len() > 2 { TABLE[(n & 63) as usize] as char } else { '=' });
    }
    out
}

fn base64_decode(text: &str) -> Vec<u8> {
    let mut out = Vec::new();
    let bytes: Vec<u8> = text.bytes().filter(|byte| *byte != b'=').collect();
    for chunk in bytes.chunks(4) {
        let mut n = 0u32;
        for (index, byte) in chunk.iter().enumerate() {
            let value = match byte {
                b'A'..=b'Z' => byte - b'A',
                b'a'..=b'z' => byte - b'a' + 26,
                b'0'..=b'9' => byte - b'0' + 52,
                b'+' => 62,
                b'/' => 63,
                _ => panic!("bad base64"),
            } as u32;
            n |= value << (18 - 6 * index);
        }
        out.push((n >> 16) as u8);
        if chunk.len() > 2 {
            out.push((n >> 8) as u8);
        }
        if chunk.len() > 3 {
            out.push(n as u8);
        }
    }
    out
}
