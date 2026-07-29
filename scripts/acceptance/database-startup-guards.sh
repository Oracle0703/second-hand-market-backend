#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() {
  printf 'database-startup-guards acceptance: FAIL: %s\n' "$*" >&2
  exit 1
}

command -v git >/dev/null 2>&1 || fail "git is required"
command -v go >/dev/null 2>&1 || fail "Go 1.22 or newer is required"
command -v timeout >/dev/null 2>&1 || fail "GNU timeout is required"

expected_sha="${EXPECTED_COMMIT_SHA:-}"
[[ "$expected_sha" =~ ^[0-9a-f]{40}$ ]] ||
  fail "EXPECTED_COMMIT_SHA must be the full 40-character commit SHA"

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" ||
  fail "run this script from a Git checkout"
repo_root="$(cd "$repo_root" && pwd -P)"
actual_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$actual_sha" == "$expected_sha" ]] ||
  fail "checked out commit does not match EXPECTED_COMMIT_SHA"
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "worktree must be clean so the tested source matches the commit"

command_timeout="${ACCEPTANCE_COMMAND_TIMEOUT:-20m}"
[[ "$command_timeout" =~ ^[1-9][0-9]*[smhd]$ ]] ||
  fail "ACCEPTANCE_COMMAND_TIMEOUT must look like 20m, 2h, or 900s"

evidence_is_temporary=0
if [[ -n "${EVIDENCE_DIR:-}" ]]; then
  mkdir -p -- "$EVIDENCE_DIR"
  evidence_root="$(cd "$EVIDENCE_DIR" && pwd -P)"
  case "$evidence_root/" in
    "$repo_root/"*) fail "EVIDENCE_DIR must be outside the Git worktree" ;;
  esac
  evidence_dir="$(mktemp -d "$evidence_root/database-startup-guards-${actual_sha:0:12}.XXXXXX")"
else
  evidence_dir="$(mktemp -d)"
  evidence_is_temporary=1
fi

build_dir="$(mktemp -d)"
if [[ -n "${ACCEPTANCE_GOMODCACHE:-}" ]]; then
  [[ "$ACCEPTANCE_GOMODCACHE" == /* ]] ||
    fail "ACCEPTANCE_GOMODCACHE must be an absolute path"
  [[ -d "$ACCEPTANCE_GOMODCACHE" ]] ||
    fail "ACCEPTANCE_GOMODCACHE must be an existing directory"
  module_cache="$(cd "$ACCEPTANCE_GOMODCACHE" && pwd -P)"
  case "$module_cache/" in
    "$repo_root/"*) fail "ACCEPTANCE_GOMODCACHE must be outside the Git worktree" ;;
  esac
else
  module_cache="$build_dir/go-mod-cache"
fi

mkdir -p -- \
  "$build_dir/tmp" \
  "$build_dir/go-cache" \
  "$build_dir/gopath" \
  "$build_dir/bin" \
  "$module_cache"

tool_env=(
  PATH="$PATH"
  TMPDIR="$build_dir/tmp"
  GOTMPDIR="$build_dir/tmp"
  TEMP="$build_dir/tmp"
  TMP="$build_dir/tmp"
  GOCACHE="$build_dir/go-cache"
  GOMODCACHE="$module_cache"
  GOPATH="$build_dir/gopath"
  GOENV=off
  GOWORK=off
  GOTOOLCHAIN=local
  GOFLAGS=-buildvcs=false\ -mod=readonly
  GOPROXY=https://proxy.golang.org,direct
  GOSUMDB=sum.golang.org
  CGO_ENABLED=0
  APP_ENV=test
  DB_TARGET=local
  DB_DRIVER=guardrail-probe
  DB_DSN=acceptance-no-database
  AUTO_MIGRATE=false
  SEED_DEFAULTS=false
  BUYER_WECHAT_LOGIN_MODE=mock
  BUYER_DOUYIN_LOGIN_MODE=mock
)

summary="$evidence_dir/summary.txt"
acceptance_succeeded=0
cleanup() {
  status=$?
  trap - EXIT
  rm -rf -- "$build_dir"
  if [[ "$evidence_is_temporary" -eq 1 && "$acceptance_succeeded" -eq 1 ]]; then
    rm -rf -- "$evidence_dir"
  else
    printf 'evidence_dir=%s\n' "$evidence_dir" >&2
  fi
  exit "$status"
}
trap cleanup EXIT

verify_temp_env() {
  env -i "${tool_env[@]}" "$BASH" -c '
    set -Eeuo pipefail
    expected="$1"
    for name in TMPDIR GOTMPDIR TEMP TMP; do
      [[ "${!name-}" == "$expected" ]] || exit 1
    done
    : >"$GOTMPDIR/database-startup-guards-write-probe"
    rm -f -- "$GOTMPDIR/database-startup-guards-write-probe"
  ' database-startup-guards-temp-check "$build_dir/tmp" ||
    fail "isolated temporary directory environment is incomplete or not writable"
}

run_logged() {
  name="$1"
  shift
  log_path="$evidence_dir/$name.log"
  set +e
  (
    cd "$repo_root/backend"
    timeout --foreground "$command_timeout" "$@"
  ) 2>&1 | tee "$log_path"
  pipeline_status=("${PIPESTATUS[@]}")
  set -e
  {
    printf '%s_exit_code=%d\n' "$name" "${pipeline_status[0]}"
    printf '%s_tee_exit_code=%d\n' "$name" "${pipeline_status[1]}"
  } >>"$summary"
  [[ "${pipeline_status[0]}" -eq 0 && "${pipeline_status[1]}" -eq 0 ]] ||
    fail "$name or its evidence capture failed"
}

go_version="$(env -i "${tool_env[@]}" go env GOVERSION)"
if ! awk -v version="${go_version#go}" '
  BEGIN {
    split(version, parts, ".")
    exit((parts[1] > 1 || (parts[1] == 1 && parts[2] >= 22)) ? 0 : 1)
  }
'; then
  fail "Go 1.22 or newer is required"
fi

{
  printf 'commit=%s\n' "$actual_sha"
  printf 'go=%s\n' "$(env -i "${tool_env[@]}" go version)"
  printf 'external_database=none\n'
} >"$summary"

verify_temp_env
run_logged go-mod-download env -i "${tool_env[@]}" go mod download
run_logged go-mod-verify env -i "${tool_env[@]}" go mod verify
run_logged go-test env -i "${tool_env[@]}" go test -p 1 -count=1 ./...
run_logged go-vet env -i "${tool_env[@]}" go vet -p 1 ./...
run_logged build-server env -i "${tool_env[@]}" go build -o "$build_dir/bin/server" ./cmd/server
run_logged build-migrate env -i "${tool_env[@]}" go build -o "$build_dir/bin/migrate" ./scripts/migrate
run_logged build-bootstrap-admin env -i "${tool_env[@]}" go build -o "$build_dir/bin/bootstrap_admin" ./scripts/bootstrap_admin
run_logged build-seed-categories env -i "${tool_env[@]}" go build -o "$build_dir/bin/seed_categories" ./scripts/seed_categories

[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "acceptance commands changed tracked or untracked source files"
final_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$final_sha" == "$actual_sha" ]] || fail "HEAD changed during acceptance"

printf 'status=PASS\n' >>"$summary"
printf 'database-startup-guards acceptance: PASS\n'
acceptance_succeeded=1
