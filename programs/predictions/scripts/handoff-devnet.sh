#!/usr/bin/env bash
# The program is already deployed. Move its upgrade authority to the council PDA.
# The council account itself is created by finish-devnet.mjs --council. This script
# does not write program bytes and does not request an airdrop.
set -euo pipefail
export PATH="/root/.local/share/solana/install/active_release/bin:${PATH}"
url="https://api.devnet.solana.com"
program="$(solana-keygen pubkey /app/keys/program-keypair.json)"
echo "program ${program}"

solana program set-upgrade-authority "${program}" \
  --new-upgrade-authority D6e74RVzp1cTXKQb1UyCygJg17szm9uvphiKZTKfarpt \
  --skip-new-upgrade-authority-signer-check \
  --keypair /app/keys/deployer-keypair.json \
  --url "${url}" \
  --commitment confirmed

solana program show "${program}" --url "${url}"
