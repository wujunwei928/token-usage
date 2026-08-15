#!/usr/bin/env bash
# Refresh docs/config-schema.json from the reference implementation's
# generated schema (the same file its npm package ships, version-locked).
set -euo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
REF=${CCUSAGE_REF:-/code/ai/ccusage/ccusage}
cp "$REF/apps/ccusage/config-schema.json" "$REPO/docs/config-schema.json"
echo "docs/config-schema.json refreshed"
