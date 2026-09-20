#!/usr/bin/env bash
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
python3 - "$repo" <<'PY'
from pathlib import Path
import sys
root = Path(sys.argv[1])
workflow = (root / '.github/workflows/release.yaml').read_text()
assert workflow.count("--candidate-repository 'autopus-ai/autopus-adk'") == 2
assert "--candidate-repository 'Insajin/autopus-adk'" not in workflow
current = (root / 'scripts/companion-release/verify-current-release.sh').read_text()
assert "readonly RELEASE_REPOSITORY='autopus-ai/autopus-adk'" in current
assert '--source-repository "$RELEASE_REPOSITORY"' in current
assert '--candidate-repository "$RELEASE_REPOSITORY"' in current
lineage = (root / 'scripts/companion-release/produce-omp-context-lineage.sh').read_text()
assert "--source-repository 'autopus-ai/autopus-adk'" in lineage
assert "--source-repository 'Insajin/autopus-adk'" not in lineage
signature = (root / 'scripts/companion-release/verify-current-release-signatures.sh').read_text()
assert "readonly COSIGN_IDENTITY='https://github.com/autopus-ai/autopus-adk/.github/workflows/release.yaml@refs/tags/v0.50.118'" in signature
preflight = (root / 'scripts/release-tools/preflight-release.sh').read_text()
assert "readonly repository='autopus-ai/autopus-adk'" in preflight
assert 'identity="https://github.com/Insajin/autopus-adk/.github/workflows/release.yaml@refs/tags/${predecessor_tag}"' in preflight
tap = (root / 'scripts/companion-release/publish-homebrew-formula-bridge.sh').read_text()
assert "readonly TAP_REPOSITORY='Insajin/homebrew-autopus'" in tap
print('release current repository contracts passed')
PY
