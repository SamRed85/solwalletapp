#!/usr/bin/env bash
# Load the built program into a local validator and run place, win, and claim.
# This does not touch devnet and does not request a public airdrop.
# The validator needs io_uring, so run the container with --security-opt seccomp=unconfined.
set -euo pipefail
export PATH="/root/.local/share/solana/install/active_release/bin:/usr/local/cargo/bin:${PATH}"
cd /app
mkdir -p /root/.config/solana
solana-keygen new --no-bip39-passphrase --force -o /root/.config/solana/id.json >/dev/null 2>&1
ledger=/tmp/predictions-ledger
rm -rf "${ledger}"
solana-test-validator --reset --ledger "${ledger}" --rpc-port 8899 --faucet-port 9900 \
  --bpf-program H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr /app/target/deploy/predictions.so \
  > /tmp/validator.log 2>&1 &
validator_pid=$!
cleanup() { kill "${validator_pid}" >/dev/null 2>&1 || true; }
trap cleanup EXIT
ready=0
for _ in $(seq 1 60); do
  if solana cluster-version --url http://127.0.0.1:8899 >/dev/null 2>&1; then
    ready=1
    break
  fi
  if grep -q "panicked at" /tmp/predictions-ledger/validator.log 2>/dev/null; then
    break
  fi
  sleep 1
done
if [[ "${1:-all}" == "hold" ]]; then
  if [[ "${ready}" -ne 1 ]]; then
    echo "validator did not open port 8899"
    cat /tmp/predictions-ledger/validator.log || true
    exit 1
  fi
  wait "${validator_pid}"
fi
if [[ "${ready}" -ne 1 ]]; then
  echo "validator did not open port 8899"
  echo "---- stdout ----"
  cat /tmp/validator.log || true
  echo "---- validator.log ----"
  cat /tmp/predictions-ledger/validator.log || true
  exit 1
fi
if [[ ! -f keys/council-signer.json ]]; then
  solana-keygen new --no-bip39-passphrase --force -o keys/council-signer.json >/dev/null 2>&1
fi
if [[ "${1:-all}" != "initialize" ]]; then
  cargo run --example local_loop -- --url http://127.0.0.1:8899
fi
cargo run --example initialize_upgrade -- --url http://127.0.0.1:8899 --airdrop
