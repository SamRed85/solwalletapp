#!/usr/bin/env bash
# Compare the local program binary with the hash recorded from this source.
# When the program is on devnet, also compare that hash with the deployed bytes.
set -euo pipefail
cd "$(dirname "$0")/.."
local_so="target/deploy/predictions.so"
recorded="$(tr -d '[:space:]' < build-hash.txt)"
actual="$(sha256sum "${local_so}" | awk '{print $1}')"
echo "local ${actual}"
if [[ "${actual}" != "${recorded}" ]]; then
  echo "local binary does not match build-hash.txt"
  exit 1
fi

if ! command -v solana >/dev/null 2>&1; then
  echo "recorded hash matches; solana is not on PATH, so the deployed bytes were not compared"
  exit 0
fi

program="35eSgMRzq1xNM4qDDLezingfXmSDM2ZAjRMTWcJ4QNca"
dump="$(mktemp)"
if ! solana program dump "${program}" "${dump}" --url https://api.devnet.solana.com; then
  echo "recorded hash matches; program is not deployed on devnet"
  exit 0
fi
deployed="$(sha256sum "${dump}" | awk '{print $1}')"
echo "devnet ${deployed}"
if [[ "${deployed}" != "${recorded}" ]]; then
  echo "deployed program does not match build-hash.txt"
  exit 1
fi
echo "deployed program matches the recorded source build"
