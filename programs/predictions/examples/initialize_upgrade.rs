//! Create the upgrade council with the deployer key.
//! A local run passes --airdrop. Devnet must not: the deployer is expected to already have SOL.

use std::io::{Read, Write};
use std::net::TcpStream;
use std::str::FromStr;
use std::thread;
use std::time::Duration;

use anchor_lang::{InstructionData, ToAccountMetas};
use solana_sdk::{
    instruction::Instruction,
    pubkey::Pubkey,
    signature::{Keypair, Signer},
    transaction::Transaction,
};

const COUNCIL: &str = "9ad7bFZNUVuvGkM2ZbepzK6wADV4LaWpW2weQPxXSWD2";

fn main() {
    let mut url = "http://127.0.0.1:8899".to_string();
    let mut airdrop = false;
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--url" => url = args.next().expect("--url needs an address"),
            "--airdrop" => airdrop = true,
            other => panic!("unknown argument {other}"),
        }
    }
    let rpc = Rpc::parse(&url);
    let deployer = load_keypair("keys/deployer-keypair.json");
    let council_signer = load_keypair("keys/council-signer.json");
    let council = Pubkey::find_program_address(&[b"upgrade-council"], &predictions::ID).0;
    assert_eq!(council.to_string(), COUNCIL, "council address drifted from the deploy script");

    if rpc.account(&council).is_some() {
        println!("upgrade council already exists at {COUNCIL}");
        return;
    }
    if airdrop {
        rpc.airdrop(&deployer.pubkey(), 1_000_000_000);
        let stranger = Keypair::new();
        rpc.airdrop(&stranger.pubkey(), 1_000_000_000);
        let rejected = rpc.send(&stranger, &[initialize_ix(stranger.pubkey(), council_signer.pubkey())]);
        let message = rejected.expect_err("another wallet must not create the council");
        assert!(message.contains("0x1786") || message.contains("6022"), "unexpected rejection: {message}");
    }

    rpc.send(&deployer, &[initialize_ix(deployer.pubkey(), council_signer.pubkey())]).unwrap();
    assert!(rpc.account(&council).is_some(), "council account was not created");
    assert!(rpc.send(&deployer, &[initialize_ix(deployer.pubkey(), council_signer.pubkey())]).is_err(), "the council can only be created once");
    println!("upgrade council {COUNCIL}");
    println!("signers {} {}", deployer.pubkey(), council_signer.pubkey());
}

fn initialize_ix(payer: Pubkey, other: Pubkey) -> Instruction {
    let council = Pubkey::find_program_address(&[b"upgrade-council"], &predictions::ID).0;
    Instruction {
        program_id: predictions::ID,
        accounts: predictions::accounts::InitializeUpgrade {
            payer,
            council,
            system_program: solana_sdk::system_program::ID,
        }
        .to_account_metas(None),
        data: predictions::instruction::InitializeUpgrade { threshold: 2, signers: vec![payer, other] }.data(),
    }
}

fn load_keypair(path: &str) -> Keypair {
    let text = std::fs::read_to_string(path).unwrap_or_else(|err| panic!("cannot read {path}: {err}"));
    let secret: Vec<u8> = text.split(|c: char| !c.is_ascii_digit()).filter(|part| !part.is_empty()).map(|part| part.parse::<u8>().unwrap()).collect();
    Keypair::try_from(secret.as_slice()).unwrap_or_else(|_| panic!("{path} is not a Solana keypair"))
}

struct Rpc {
    host: String,
    port: u16,
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

    fn account(&self, key: &Pubkey) -> Option<Vec<u8>> {
        let result = self.rpc("getAccountInfo", serde_json::json!([key.to_string(), {"encoding":"base64","commitment":"confirmed"}]));
        Some(base64_decode(result["value"]["data"][0].as_str()?))
    }

    fn airdrop(&self, key: &Pubkey, lamports: u64) {
        let signature = self.rpc("requestAirdrop", serde_json::json!([key.to_string(), lamports])).as_str().unwrap().to_string();
        self.confirm(&signature);
    }

    fn confirm(&self, signature: &str) {
        for _ in 0..40 {
            let status = &self.rpc("getSignatureStatuses", serde_json::json!([[signature], {"searchTransactionHistory": true}]))["value"][0];
            if !status["err"].is_null() {
                panic!("transaction {signature} failed: {status}");
            }
            if status["confirmationStatus"].as_str().is_some_and(|level| level == "confirmed" || level == "finalized") {
                return;
            }
            thread::sleep(Duration::from_millis(500));
        }
        panic!("transaction {signature} was not confirmed");
    }

    fn send(&self, payer: &Keypair, instructions: &[Instruction]) -> Result<(), String> {
        let blockhash = self.rpc("getLatestBlockhash", serde_json::json!([{"commitment":"confirmed"}]))["value"]["blockhash"].as_str().unwrap().to_string();
        let transaction = Transaction::new_signed_with_payer(instructions, Some(&payer.pubkey()), &[payer], solana_sdk::hash::Hash::from_str(&blockhash).unwrap());
        let encoded = base64_encode(&bincode::serialize(&transaction).unwrap());
        let body = serde_json::json!({
            "jsonrpc":"2.0","id":1,"method":"sendTransaction",
            "params":[encoded, {"encoding":"base64","preflightCommitment":"confirmed"}]
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
        let json = http_body(&String::from_utf8_lossy(&raw));
        let value: serde_json::Value = serde_json::from_str(json.trim()).unwrap_or_else(|err| panic!("send was not json ({err}): {json}"));
        if let Some(error) = value.get("error") {
            return Err(error.to_string());
        }
        self.confirm(value["result"].as_str().unwrap());
        Ok(())
    }
}

fn http_body(raw: &str) -> String {
    let Some((head, body)) = raw.split_once("\r\n\r\n") else { panic!("rpc response had no body: {raw}") };
    if !head.to_ascii_lowercase().contains("transfer-encoding: chunked") {
        return body.to_string();
    }
    let mut out = String::new();
    let mut rest = body;
    loop {
        let Some((size, next)) = rest.split_once("\r\n") else { break };
        let size = usize::from_str_radix(size.trim(), 16).unwrap_or(0);
        if size == 0 || next.len() < size { break; }
        out.push_str(&next[..size]);
        rest = next[size..].strip_prefix("\r\n").unwrap_or(&next[size..]);
    }
    out
}

fn base64_encode(data: &[u8]) -> String {
    const TABLE: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::new();
    for chunk in data.chunks(3) {
        let mut n = (chunk[0] as u32) << 16;
        if chunk.len() > 1 { n |= (chunk[1] as u32) << 8; }
        if chunk.len() > 2 { n |= chunk[2] as u32; }
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
        if chunk.len() > 2 { out.push((n >> 8) as u8); }
        if chunk.len() > 3 { out.push(n as u8); }
    }
    out
}
