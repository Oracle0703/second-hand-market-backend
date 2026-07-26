#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
miniapp_dir="$repo_dir/miniapp"
evidence_dir="$base_dir/evidence/miniapp-auth-refresh"
expected_node="v22.22.2"
expected_npm="10.9.7"

[[ "${MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS" ]] || {
  echo "isolated miniapp auth refresh confirmation is missing" >&2
  exit 1
}

command -v node >/dev/null || {
  echo "node is required" >&2
  exit 1
}
command -v npm >/dev/null || {
  echo "npm is required" >&2
  exit 1
}
command -v sha256sum >/dev/null || {
  echo "sha256sum is required" >&2
  exit 1
}

node_version="$(node --version)"
npm_version="$(npm --version)"
[[ "$node_version" == "$expected_node" ]] || {
  echo "node must be $expected_node (found $node_version)" >&2
  exit 1
}
[[ "$npm_version" == "$expected_npm" ]] || {
  echo "npm must be $expected_npm (found $npm_version)" >&2
  exit 1
}

mkdir -p "$evidence_dir"
chmod 700 "$evidence_dir"
export TARO_APP_API_BASE_URL="https://example.invalid/api/v1"

run_in_miniapp() {
  local output="$1"
  shift
  (
    cd "$miniapp_dir"
    "$@"
  ) 2>&1 | tee "$evidence_dir/$output"
}

{
  printf 'node=%s\n' "$node_version"
  printf 'npm=%s\n' "$npm_version"
  printf 'api_base_url=%s\n' "$TARO_APP_API_BASE_URL"
} | tee "$evidence_dir/toolchain.txt"

run_in_miniapp npm-ci.txt npm ci \
  --registry=https://registry.npmmirror.com \
  --replace-registry-host=always
run_in_miniapp focused-tests.txt npm test -- --run tests/request-refresh.test.ts
run_in_miniapp full-tests.txt npm test
run_in_miniapp build-weapp.txt npm run build:weapp
run_in_miniapp build-tt.txt npm run build:tt

(
  cd "$evidence_dir"
  sha256sum toolchain.txt npm-ci.txt focused-tests.txt full-tests.txt build-weapp.txt build-tt.txt >sha256.txt
)

echo "isolated miniapp auth refresh acceptance passed"
