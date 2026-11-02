#!/usr/bin/env bash
# Creates cortex/.venv on a Python version the AI libraries ship wheels for (3.10-3.13).
set -euo pipefail
cd "$(dirname "$0")/../cortex"

PYTHON=""
for candidate in python3.12 python3.13 python3.11 python3.10 python3; do
  if command -v "$candidate" >/dev/null 2>&1 && "$candidate" -c 'import sys; sys.exit(0 if (3, 10) <= sys.version_info[:2] <= (3, 13) else 1)'; then
    PYTHON="$candidate"
    break
  fi
done
if [ -z "$PYTHON" ]; then
  echo "Python 3.10-3.13 is required (langgraph and chromadb have no wheels for newer versions)." >&2
  echo "Install one, for example: brew install python@3.12" >&2
  exit 1
fi

echo "Using $($PYTHON --version) at $(command -v "$PYTHON")"
"$PYTHON" -m venv .venv
.venv/bin/pip install --upgrade pip
.venv/bin/pip install -r requirements.txt
.venv/bin/python -m playwright install chromium || echo "Browser download failed; scraping falls back to plain HTTP."
echo "Done. Optional extras: requirements-ml.txt (sentence-transformers), requirements-observability.txt (Phoenix)."
