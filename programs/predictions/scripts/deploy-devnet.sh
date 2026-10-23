#!/usr/bin/env bash
# Publish target/deploy/predictions.so to devnet when the deployer already has SOL.
# Does not request an airdrop. The deployer key then creates the upgrade council.
set -euo pipefail
export PATH="/root/.local/share/solana/install/active_release/bin:/usr/local/cargo/bin:${PATH}"
url="https://api.devnet.solana.com"
payer="$(solana-keygen pubkey /app/keys/deployer-keypair.json)"
program="$(solana-keygen pubkey /app/keys/program-keypair.json)"
echo "payer ${payer}"
echo "program ${program}"

balance="$(solana balance "${payer}" --url "${url}" --lamports | awk '{print $1}')"
echo "balance ${balance}"
buffer_balance=0
if [[ -f /app/keys/buffer-keypair.json ]]; then
  buffer="$(solana-keygen pubkey /app/keys/buffer-keypair.json)"
  buffer_balance="$(solana balance "${buffer}" --url "${url}" --lamports | awk '{print $1}')"
  echo "buffer ${buffer} ${buffer_balance}"
fi
# The program account costs about 2.212 SOL. The rest covers the council account and fees.
# A funded buffer already paid that cost, so a resume must not demand it from the deployer again.
# Stop here, before `solana program deploy` creates a buffer, when nothing is funded.
if [[ "${buffer_balance}" -lt 2000000000 && "${balance}" -lt 2300000000 ]]; then
  echo "deployer has ${balance} lamports and needs 2300000000 before this script can publish the program"
  exit 1
fi

if [[ ! -f /app/keys/buffer-keypair.json ]]; then
  solana-keygen new --no-bip39-passphrase --force -o /app/keys/buffer-keypair.json >/dev/null 2>&1
fi

solana program deploy /app/target/deploy/predictions.so \
  --buffer /app/keys/buffer-keypair.json \
  --use-rpc \
  --program-id /app/keys/program-keypair.json \
  --keypair /app/keys/deployer-keypair.json \
  --url "${url}" \
  --commitment confirmed

# The council PDA is seeds [b"upgrade-council"]. It cannot sign this handoff.
# After this, only execute_upgrade can replace the program. The devnet build waits five
# minutes, with a test-only override that requires every council member to sign.
solana program set-upgrade-authority "${program}" \
  --new-upgrade-authority D6e74RVzp1cTXKQb1UyCygJg17szm9uvphiKZTKfarpt \
  --skip-new-upgrade-authority-signer-check \
  --keypair /app/keys/deployer-keypair.json \
  --url "${url}" \
  --commitment confirmed

if [[ ! -f /app/keys/council-signer.json ]]; then
  solana-keygen new --no-bip39-passphrase --force -o /app/keys/council-signer.json >/dev/null 2>&1
fi
echo "program published; the council transaction is sent by finish-devnet.mjs --council"
