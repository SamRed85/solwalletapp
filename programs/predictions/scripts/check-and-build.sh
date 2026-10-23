#!/usr/bin/env bash
set -euo pipefail
export PATH="/usr/local/cargo/bin:${PATH}"
cd /app
cargo test --features devnet-testing 2>&1 | grep -E 'test result|^error|FAILED'
cargo test 2>&1 | grep -E 'test result|^error|FAILED'
DEVNET_TESTING=1 bash scripts/build-sbf.sh 2>&1 | grep -E 'error|Error|Finished|program id|devnet-testing'
sha256sum target/deploy/predictions.so
ls -l target/deploy/predictions.so
