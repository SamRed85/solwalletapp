#!/usr/bin/env bash
# Build the devnet program inside the rust:bookworm container.
# Keypairs stay under keys/, which is gitignored. This script never prints them.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export PATH="/root/.local/share/solana/install/active_release/bin:/usr/local/cargo/bin:${PATH}"
export CARGO_HOME="${CARGO_HOME:-/usr/local/cargo}"

if ! command -v solana >/dev/null 2>&1; then
  apt-get update
  apt-get install -y --no-install-recommends curl ca-certificates bzip2 pkg-config libssl-dev
  sh -c "$(curl -sSfL https://release.anza.xyz/v4.3.2/install)"
fi

command -v solana
solana --version

mkdir -p /app/keys
if [[ ! -f /app/keys/program-keypair.json ]]; then
  solana-keygen new --no-bip39-passphrase --outfile /app/keys/program-keypair.json --force >/tmp/program-keygen.txt
fi
if [[ ! -f /app/keys/deployer-keypair.json ]]; then
  solana-keygen new --no-bip39-passphrase --outfile /app/keys/deployer-keypair.json --force >/tmp/deployer-keygen.txt
fi

pubkey="$(solana-keygen pubkey /app/keys/program-keypair.json)"
printf '%s\n' "${pubkey}" > /app/keys/program-id.txt
sed -i "s/declare_id!(\"[^\"]*\")/declare_id!(\"${pubkey}\")/" /app/src/lib.rs
echo "program id ${pubkey}"

cd /app
cargo-build-sbf --features devnet-testing
