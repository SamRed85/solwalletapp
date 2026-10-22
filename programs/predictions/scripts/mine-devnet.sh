#!/usr/bin/env bash
# Mine devnet SOL into the gitignored deployer key. Prints only public output.
set -euo pipefail
export PATH="/usr/local/cargo/bin:/root/.local/share/solana/install/active_release/bin:${PATH}"
export CARGO_HOME="${CARGO_HOME:-/usr/local/cargo}"
url="https://api.devnet.solana.com"
keypair="/app/keys/deployer-keypair.json"

if ! command -v devnet-pow >/dev/null 2>&1; then
  cargo install devnet-pow --locked
fi

echo "faucets"
devnet-pow get-all-faucets -u "${url}" -k "${keypair}"
echo "mining"
devnet-pow mine -d 3 --reward 0.02 --no-infer -t 2000000000 -u "${url}" -k "${keypair}"
payer="$(solana-keygen pubkey "${keypair}")"
solana balance "${payer}" --url "${url}"
solana program deploy /app/target/deploy/predictions.so \
  --program-id /app/keys/program-keypair.json \
  --keypair "${keypair}" \
  --url "${url}" \
  --commitment confirmed
