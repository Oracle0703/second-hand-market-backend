#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() {
  printf 'runtime-guardrails acceptance: FAIL: %s\n' "$*" >&2
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
  fail "checked out commit $actual_sha, expected $expected_sha"
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "worktree must be clean so the tested source matches the commit"

go_semver="$(
  env -i \
    PATH="$PATH" \
    GOENV=off \
    GOWORK=off \
    GOTOOLCHAIN=local \
    go env GOVERSION
)"
if ! awk -v version="${go_semver#go}" '
  BEGIN {
    split(version, parts, ".")
    exit((parts[1] > 1 || (parts[1] == 1 && parts[2] >= 22)) ? 0 : 1)
  }
'; then
  fail "Go 1.22 or newer is required, found $go_semver"
fi

command_timeout="${ACCEPTANCE_COMMAND_TIMEOUT:-20m}"
probe_timeout="${ACCEPTANCE_PROBE_TIMEOUT:-10s}"
[[ "$command_timeout" =~ ^[1-9][0-9]*[smhd]$ ]] ||
  fail "ACCEPTANCE_COMMAND_TIMEOUT must look like 20m, 2h, or 900s"
[[ "$probe_timeout" =~ ^[1-9][0-9]*[smhd]$ ]] ||
  fail "ACCEPTANCE_PROBE_TIMEOUT must look like 10s or 1m"

evidence_is_temporary=0
if [[ -n "${EVIDENCE_DIR:-}" ]]; then
  mkdir -p -- "$EVIDENCE_DIR"
  evidence_root="$(cd "$EVIDENCE_DIR" && pwd -P)"
  case "$evidence_root/" in
    "$repo_root/"*) fail "EVIDENCE_DIR must be outside the Git worktree" ;;
  esac
  evidence_dir="$(mktemp -d "$evidence_root/runtime-guardrails-${actual_sha:0:12}.XXXXXX")"
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
  "$module_cache"
tool_env=(
  PATH="$PATH"
  TMPDIR="$build_dir/tmp"
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
  DB_DRIVER=guardrail-probe
  DB_DSN=acceptance-no-database
  ACCESS_TTL_SECONDS=7200
  REFRESH_TTL_SECONDS=604800
  BUYER_WECHAT_LOGIN_MODE=mock
  BUYER_DOUYIN_LOGIN_MODE=mock
)
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

summary="$evidence_dir/summary.txt"
test_log="$evidence_dir/go-test.json"
server_bin="$build_dir/server"

{
  printf 'commit=%s\n' "$actual_sha"
  printf 'go=%s\n' "$(
    env -i \
      PATH="$PATH" \
      GOENV=off \
      GOWORK=off \
      GOTOOLCHAIN=local \
      go version
  )"
  printf 'external_database=none; tests=in-memory-sqlite; probes=unsupported-sentinel\n'
  printf 'external_identity_provider=none\n'
} | tee "$summary"

run_logged() {
  local name="$1"
  shift
  local log_path="$evidence_dir/$name.log"
  local pipeline_status
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
  } | tee -a "$summary"
  [[ "${pipeline_status[0]}" -eq 0 && "${pipeline_status[1]}" -eq 0 ]] ||
    fail "$name or its evidence capture failed"
}

run_logged go-mod-download env -i "${tool_env[@]}" go mod download
run_logged go-mod-verify env -i "${tool_env[@]}" go mod verify

set +e
(
  cd "$repo_root/backend"
  timeout --foreground "$command_timeout" env -i \
    "${tool_env[@]}" \
    go test -json -count=1 ./...
) 2>&1 | tee "$test_log"
test_pipeline_status=("${PIPESTATUS[@]}")
set -e
{
  printf 'go_test_exit_code=%d\n' "${test_pipeline_status[0]}"
  printf 'go_test_tee_exit_code=%d\n' "${test_pipeline_status[1]}"
} | tee -a "$summary"
[[ "${test_pipeline_status[0]}" -eq 0 && "${test_pipeline_status[1]}" -eq 0 ]] ||
  fail "Go regression suite or evidence capture failed"

if awk '
  index($0, "\"Action\":\"skip\"") &&
  index($0, "\"Test\":") { found = 1 }
  END { exit(found ? 0 : 1) }
' "$test_log"; then
  fail "a backend test was skipped"
fi

required_tests=(
  "TestRuntimeGuardrails/production_accepts_real_plus_disabled"
  "TestRuntimeGuardrails/production_accepts_disabled_plus_real"
  "TestRuntimeGuardrails/production_accepts_all_disabled"
  "TestRuntimeGuardrails/rejects_one_byte_access_secret"
  "TestRuntimeGuardrails/rejects_31_byte_refresh_secret"
  "TestRuntimeGuardrails/rejects_known_long_placeholder"
  "TestRuntimeGuardrails/rejects_equal_jwt_secrets"
  "TestRuntimeGuardrails/rejects_secret_whitespace"
  "TestRuntimeGuardrails/rejects_repeated_secret"
  "TestRuntimeGuardrails/rejects_low_diversity_secret"
  "TestRuntimeGuardrails/rejects_repeating_pattern_secret"
  "TestRuntimeGuardrails/rejects_wechat_mock"
  "TestRuntimeGuardrails/rejects_douyin_mock"
  "TestRuntimeGuardrails/rejects_empty_mode"
  "TestRuntimeGuardrails/rejects_unknown_mode"
  "TestRuntimeGuardrails/rejects_real_provider_without_credentials"
  "TestRuntimeGuardrails/rejects_real_provider_without_positive_timeout"
  "TestRuntimeGuardrails/rejects_real_provider_excessive_timeout"
  "TestRuntimeGuardrails/accepts_real_provider_timeout_boundary"
  "TestRuntimeGuardrails/rejects_nonofficial_production_endpoint"
  "TestRuntimeGuardrails/rejects_empty_app_env"
  "TestRuntimeGuardrails/rejects_unknown_app_env"
  "TestRuntimeGuardrails/development_allows_mock"
  "TestRuntimeGuardrails/test_allows_mock"
  "TestNewServerValidatesRuntimeBeforeDatabase/unsafe_configuration_stops_before_database"
  "TestNewServerValidatesRuntimeBeforeDatabase/safe_configuration_reaches_database_probe"
  "TestLoadConfigProductionDefaultsFailClosed"
  "TestLoadConfigRejectsInvalidRealProviderTimeout/rejects_zero_wechat_timeout"
  "TestLoadConfigRejectsInvalidRealProviderTimeout/rejects_douyin_timeout_above_limit"
  "TestLoadConfigRejectsInvalidRealProviderTimeout/rejects_nonnumeric_wechat_timeout"
  "TestLoadConfigIgnoresUnusedProviderTimeout"
  "TestDisabledProviderReturnsForbidden/wechat"
  "TestDisabledProviderReturnsForbidden/douyin"
  "TestWechatTransportErrorDoesNotExposeAppSecret"
  "TestDouyinTransportErrorDoesNotExposeAppSecret"
  "TestLoadConfigRequiresExplicitAppEnv"
  "TestProductionEnvExamplesEnableRuntimeGuardrails/.env.production.mysql.example"
  "TestProductionEnvExamplesEnableRuntimeGuardrails/.env.production.sqlite.example"
)
runtime_package="second-hand-market-backend/backend/internal/app"
for required_test in "${required_tests[@]}"; do
  if ! awk -v package_name="$runtime_package" -v test_name="$required_test" '
    index($0, "\"Action\":\"pass\"") &&
    index($0, "\"Package\":\"" package_name "\"") &&
    index($0, "\"Test\":\"" test_name "\"") { found = 1 }
    END { exit(found ? 0 : 1) }
  ' "$test_log"; then
    fail "required test did not report PASS: $required_test"
  fi
done

business_package="second-hand-market-backend/backend/tests"
business_test="TestMainFlow_RegisterApproveLoginProductOrder"
if ! awk -v package_name="$business_package" -v test_name="$business_test" '
  index($0, "\"Action\":\"pass\"") &&
  index($0, "\"Package\":\"" package_name "\"") &&
  index($0, "\"Test\":\"" test_name "\"") { found = 1 }
  END { exit(found ? 0 : 1) }
' "$test_log"; then
  fail "business regression did not report PASS: $business_test"
fi

run_logged go-vet env -i "${tool_env[@]}" go vet ./...
run_logged go-build env -i "${tool_env[@]}" go build -o "$server_bin" ./cmd/server

synthetic_access="U8mP2vL9qR4tN7cX1kD6hF3jW5sY0bG8eA2uM9iQ"
synthetic_refresh="C5nT8xK2dR7pV1mH9sL4qY6wF0bJ3gZ8uE2aN7iD"
synthetic_wechat="wechat-probe-secret-not-real"
synthetic_douyin="douyin-probe-secret-not-real"

run_probe() {
  local name="$1"
  local expected="$2"
  local expect_database="$3"
  shift 3
  local log_path="$evidence_dir/probe-$name.log"
  local status
  local assignment
  local env_name
  local -a env_names=()
  local -a unique_env=()
  local -A env_by_name=()

  for assignment in "$@"; do
    [[ "$assignment" == *=* ]] || fail "probe $name received an invalid environment assignment"
    env_name="${assignment%%=*}"
    if [[ -z "${env_by_name[$env_name]+present}" ]]; then
      env_names+=("$env_name")
    fi
    env_by_name["$env_name"]="$assignment"
  done
  for env_name in "${env_names[@]}"; do
    unique_env+=("${env_by_name[$env_name]}")
  done

  set +e
  timeout --foreground "$probe_timeout" env -i \
    PATH="$PATH" \
    "${unique_env[@]}" \
    "$server_bin" >"$log_path" 2>&1
  status=$?
  set -e

  printf 'probe_%s_exit_code=%d\n' "$name" "$status" | tee -a "$summary"
  [[ "$status" -ne 0 && "$status" -ne 124 ]] ||
    fail "probe $name did not fail promptly"
  grep -Fq "$expected" "$log_path" ||
    fail "probe $name did not report the expected safe field"
  if [[ "$expect_database" == "yes" ]]; then
    grep -Fq "unsupported db driver: guardrail-probe" "$log_path" ||
      fail "safe probe did not reach the database sentinel"
  elif grep -Fq "unsupported db driver" "$log_path"; then
    fail "probe $name reached the database before runtime validation"
  fi
}

safe_runtime=(
  APP_ENV=production
  DB_DRIVER=guardrail-probe
  DB_DSN=unused
  JWT_ACCESS_SECRET="$synthetic_access"
  JWT_REFRESH_SECRET="$synthetic_refresh"
  BUYER_WECHAT_LOGIN_MODE=real
  BUYER_WECHAT_APP_ID=wx-probe-app-id
  BUYER_WECHAT_APP_SECRET="$synthetic_wechat"
  BUYER_WECHAT_CODE2SESSION_URL=https://api.weixin.qq.com/sns/jscode2session
  BUYER_WECHAT_HTTP_TIMEOUT_SECONDS=5
  BUYER_DOUYIN_LOGIN_MODE=disabled
  BUYER_DOUYIN_CODE2SESSION_URL=https://developer.toutiao.com/api/apps/v2/jscode2session
  BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS=5
)

run_probe missing-app-env "APP_ENV" no \
  DB_DRIVER=guardrail-probe \
  BUYER_WECHAT_LOGIN_MODE=mock \
  BUYER_DOUYIN_LOGIN_MODE=mock
run_probe invalid-app-env "APP_ENV" no \
  "${safe_runtime[@]}" APP_ENV=prodution
run_probe weak-access "JWT_ACCESS_SECRET" no \
  "${safe_runtime[@]}" JWT_ACCESS_SECRET=a
run_probe weak-refresh "JWT_REFRESH_SECRET" no \
  "${safe_runtime[@]}" JWT_REFRESH_SECRET=b
run_probe equal-jwt "must be different" no \
  "${safe_runtime[@]}" JWT_REFRESH_SECRET="$synthetic_access"
run_probe wechat-mock "BUYER_WECHAT_LOGIN_MODE" no \
  "${safe_runtime[@]}" BUYER_WECHAT_LOGIN_MODE=mock
run_probe douyin-mock "BUYER_DOUYIN_LOGIN_MODE" no \
  "${safe_runtime[@]}" BUYER_DOUYIN_LOGIN_MODE=mock
run_probe all-disabled "unsupported db driver: guardrail-probe" yes \
  "${safe_runtime[@]}" BUYER_WECHAT_LOGIN_MODE=disabled
run_probe missing-wechat-credential "BUYER_WECHAT_APP_SECRET" no \
  "${safe_runtime[@]}" BUYER_WECHAT_APP_SECRET=
run_probe nonofficial-endpoint "BUYER_WECHAT_CODE2SESSION_URL" no \
  "${safe_runtime[@]}" BUYER_WECHAT_CODE2SESSION_URL=https://example.invalid/sns/jscode2session
run_probe safe-control "unsupported db driver: guardrail-probe" yes \
  "${safe_runtime[@]}"

for secret in \
  "$synthetic_access" \
  "$synthetic_refresh" \
  "$synthetic_wechat" \
  "$synthetic_douyin"; do
  if grep -R -a -Fq -- "$secret" "$evidence_dir"; then
    fail "acceptance evidence leaked a synthetic credential"
  fi
done

passed_test_actions="$(
  awk '
    index($0, "\"Action\":\"pass\"") &&
    index($0, "\"Test\":") { count++ }
    END { print count + 0 }
  ' "$test_log"
)"
{
  printf 'required_runtime_tests=%d\n' "${#required_tests[@]}"
  printf 'passed_test_actions=%s\n' "$passed_test_actions"
} | tee -a "$summary"

[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "acceptance commands changed tracked or untracked source files"
final_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$final_sha" == "$actual_sha" ]] ||
  fail "HEAD changed during acceptance: started at $actual_sha, ended at $final_sha"
printf 'runtime-guardrails acceptance: PASS\n' | tee -a "$summary"
acceptance_succeeded=1
