#!/usr/bin/env bash
# Refresh the embedded pricing snapshots under internal/core/pricingdata/.
#
# The snapshots must match the reference BINARY's embedded data byte-for-byte
# (the repo checkout's copies can be ahead of the published release). This
# script extracts the three JSON blobs directly from the reference binary.
set -euo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
DEST=$REPO/internal/core/pricingdata
REF_BIN=${1:-/home/wujunwei/.nvm/versions/node/v24.17.0/lib/node_modules/ccusage/node_modules/@ccusage/ccusage-linux-x64/bin/ccusage}

python3 - "$REF_BIN" "$DEST" <<'EOF'
import json, sys

binary, dest = sys.argv[1], sys.argv[2]
data = open(binary, "rb").read()

def extract(start):
    depth = 0; in_str = False; esc = False
    for i in range(start, len(data)):
        c = data[i:i+1]
        if in_str:
            if esc: esc = False
            elif c == b"\\": esc = True
            elif c == b'"': in_str = False
            continue
        if c == b'"': in_str = True
        elif c == b"{": depth += 1
        elif c == b"}":
            depth -= 1
            if depth == 0:
                return data[start:i+1]
    return None

def blob_at(marker, offset=0):
    i = data.find(marker, offset)
    if i < 0:
        raise SystemExit(f"marker {marker!r} not found in binary")
    start = data.rfind(b"{", max(0, i - 2000), i)
    return extract(start)

models_dev = blob_at(b"@cf/moonshotai/kimi-k2.6")
fast_mult = blob_at(b'{\n\t"exact"')
# The LiteLLM blob: first marker hit near the fast-multiplier blob belongs to
# the compact pricing table; find a key with the compact schema.
litellm = None
pos = 0
marker = b"anthropic.claude-3-5-haiku-20241022-v1:0"
while True:
    i = data.find(marker, pos)
    if i < 0:
        break
    pos = i + 1
    start = data.rfind(b"{", max(0, i - 500), i)
    if start < 0:
        continue
    blob = extract(start)
    if not blob:
        continue
    try:
        parsed = json.loads(blob)
    except Exception:
        continue
    first = next(iter(parsed.values()), None)
    if isinstance(first, dict) and any(k in first for k in ("i", "o", "cc", "ctx")):
        litellm = blob
        break
if litellm is None:
    raise SystemExit("litellm snapshot not found in binary")

for name, blob in [
    ("models-dev-pricing.json", models_dev),
    ("fast-multiplier-overrides.json", fast_mult),
    ("litellm-pricing.json", litellm),
]:
    json.loads(blob)  # validate
    with open(f"{dest}/{name}", "wb") as f:
        f.write(blob)
    print(f"{name}: {len(blob)} bytes")
EOF

echo "pricing snapshots extracted from the reference binary; run scripts/golden.sh to check for drift"
