#!/bin/bash
# scan_repo.sh

OWNER_REPO=$1

# Run TruffleHog
trufflehog git --no-verification --no-update --json "https://github.com/${OWNER_REPO}" > "results/${OWNER_REPO//\//_}_results.json"

echo "Scanned ${OWNER_REPO}, results saved to ${OWNER_REPO//\//_}_results.json"
