#!/bin/bash
# scan_repo.sh

OWNER_REPO=$1
RESULT_FILE="results/${OWNER_REPO//\//_}_results.json"

# Check if the result file already exists and is non-zero in size
if [[ -s "$RESULT_FILE" ]]; then
    echo "Results for ${OWNER_REPO} already exist in $RESULT_FILE, skipping scan."
    exit 0
fi

# Run TruffleHog if result file doesn't exist or is empty
trufflehog git --no-update --json "https://github.com/${OWNER_REPO}" >"$RESULT_FILE"

echo "Scanned ${OWNER_REPO}, results saved to $RESULT_FILE"
