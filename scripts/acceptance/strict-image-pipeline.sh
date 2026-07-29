#!/usr/bin/env bash
set -Eeuo pipefail

fail() {
  printf 'strict-image-pipeline acceptance: FAIL: %s\n' "$*" >&2
  exit 1
}

command -v git >/dev/null 2>&1 || fail "git is required"
command -v go >/dev/null 2>&1 || fail "Go 1.22 or newer is required"
command -v vips >/dev/null 2>&1 || fail "libvips CLI is required"
command -v node >/dev/null 2>&1 || fail "Node.js 22.22.2 is required"
command -v npm >/dev/null 2>&1 || fail "npm 10.9.7 is required"
command -v timeout >/dev/null 2>&1 || fail "GNU timeout is required"

expected_sha="${EXPECTED_COMMIT_SHA:-}"
[[ "$expected_sha" =~ ^[0-9a-f]{40}$ ]] ||
  fail "EXPECTED_COMMIT_SHA must be the full 40-character commit SHA"

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" ||
  fail "run this script from a Git checkout"
actual_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$actual_sha" == "$expected_sha" ]] ||
  fail "checked out commit $actual_sha, expected $expected_sha"

if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  fail "worktree must be clean so the tested source matches the commit"
fi

if [[ -n "${FILE_PUBLIC_BASE_URL:-}" ]]; then
  fail "FILE_PUBLIC_BASE_URL must be empty; /uploads must be served by the guarded backend route"
fi

go_version="$(go version)"
go_semver="$(go env GOVERSION)"
vips_version="$(vips --version)"
node_version="$(node --version)"
npm_version="$(npm --version)"
[[ "$node_version" == "v22.22.2" ]] ||
  fail "Node.js v22.22.2 is required, found $node_version"
[[ "$npm_version" == "10.9.7" ]] ||
  fail "npm 10.9.7 is required, found $npm_version"
if ! awk -v version="${go_semver#go}" '
  BEGIN {
    split(version, parts, ".")
    exit((parts[1] > 1 || (parts[1] == 1 && parts[2] >= 22)) ? 0 : 1)
  }
'; then
  fail "Go 1.22 or newer is required, found $go_semver"
fi
printf 'commit=%s\n' "$actual_sha"
printf 'go=%s\n' "$go_version"
printf 'vips=%s\n' "$vips_version"
printf 'node=%s\n' "$node_version"
printf 'npm=%s\n' "$npm_version"
printf 'database=isolated in-memory SQLite fixtures only\n'

command_timeout="${ACCEPTANCE_COMMAND_TIMEOUT:-30m}"
[[ "$command_timeout" =~ ^[1-9][0-9]*[smhd]$ ]] ||
  fail "ACCEPTANCE_COMMAND_TIMEOUT must look like 30m, 2h, or 900s"

evidence_is_temporary=0
if [[ -n "${EVIDENCE_DIR:-}" ]]; then
  mkdir -p -- "$EVIDENCE_DIR"
  evidence_root="$(cd "$EVIDENCE_DIR" && pwd)"
  evidence_dir="$(mktemp -d "$evidence_root/strict-image-${actual_sha:0:12}.XXXXXX")"
else
  evidence_dir="$(mktemp -d)"
  evidence_is_temporary=1
fi
acceptance_succeeded=0
cleanup() {
  status=$?
  if [[ "$evidence_is_temporary" -eq 1 && "$acceptance_succeeded" -eq 1 ]]; then
    rm -rf -- "$evidence_dir"
  else
    printf 'evidence_dir=%s\n' "$evidence_dir" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
test_log="$evidence_dir/go-test-$actual_sha.json"

set +e
(
  cd "$repo_root/backend"
  timeout --foreground "$command_timeout" env \
    -u DB_DRIVER \
    -u DB_DSN \
    STRICT_IMAGE_VIPS_INTEGRATION=1 \
    IMAGE_PROCESSOR_BIN=vips \
    go test -json -count=1 ./internal/media ./tests
) | tee "$test_log"
test_pipeline_status=("${PIPESTATUS[@]}")
set -e

printf 'go_test_exit_code=%d\n' "${test_pipeline_status[0]}"
printf 'go_test_tee_exit_code=%d\n' "${test_pipeline_status[1]}"
[[ "${test_pipeline_status[0]}" -eq 0 && "${test_pipeline_status[1]}" -eq 0 ]] ||
  fail "Go regression suite or evidence capture failed"

if grep -Fq '"Action":"skip"' "$test_log"; then
  fail "a selected test was skipped"
fi

required_tests=(
  "second-hand-market-backend/backend/internal/media|TestCanonicalImageMIMENormalizesGenericHEIF"
  "second-hand-market-backend/backend/internal/media|TestImageMIMEMatchesClaimAllowsHEIFAliasesOnly"
  "second-hand-market-backend/backend/internal/media|TestPassthroughProcessorReencodesJPEG"
  "second-hand-market-backend/backend/internal/media|TestPassthroughProcessorReencodesPNG"
  "second-hand-market-backend/backend/internal/media|TestPassthroughProcessorRejectsHTMLDeclaredAsJPEG"
  "second-hand-market-backend/backend/internal/media|TestPassthroughProcessorRejectsTruncatedJPEG"
  "second-hand-market-backend/backend/internal/media|TestVipsProcessorNeverFallsBackToOriginalBytes"
  "second-hand-market-backend/backend/internal/media|TestVipsProcessorKeepsSanitizedResultWhenCompressionFails"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/jpeg"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/png"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/webp"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/heic"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/heif"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/heic_declared_heif"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/heif_declared_heic"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/html_rejected"
  "second-hand-market-backend/backend/internal/media|TestVipsCLIProcessorIntegration/orientation"
  "second-hand-market-backend/backend/tests|TestFileUploadReencodesAndServesCanonicalImages/jpeg"
  "second-hand-market-backend/backend/tests|TestFileUploadReencodesAndServesCanonicalImages/png"
  "second-hand-market-backend/backend/tests|TestFileUploadRejectsDisguisedAndMalformedImages/html_declared_as_jpeg"
  "second-hand-market-backend/backend/tests|TestFileUploadRejectsDisguisedAndMalformedImages/truncated_jpeg"
  "second-hand-market-backend/backend/tests|TestFileUploadRejectsReservedMIMEMismatch"
  "second-hand-market-backend/backend/tests|TestFileUploadRejectsInvalidProcessorContract/unsafe_output_extension"
  "second-hand-market-backend/backend/tests|TestFileUploadRejectsInvalidProcessorContract/output_MIME_differs_from_reservation"
  "second-hand-market-backend/backend/tests|TestPublicUploadHandlerBlocksExecutableAndMismatchedFiles"
  "second-hand-market-backend/backend/tests|TestLocalFileResponsesIgnorePersistedExternalURL"
  "second-hand-market-backend/backend/tests|TestConfirmCannotPromotePendingFile"
  "second-hand-market-backend/backend/tests|TestServerRejectsUnknownImageProcessorDriver"
  "second-hand-market-backend/backend/tests|TestServerRejectsExternalPublicBaseURLForLocalStorage"
  "second-hand-market-backend/backend/tests|TestServerRejectsUnsupportedFileStorageProvider"
  "second-hand-market-backend/backend/tests|TestFileUploadRejectsOversizedMultipartBeforeParsing"
  "second-hand-market-backend/backend/tests|TestApprovedMerchantProductImageRejectsHTMLDeclaredAsJPEG"
  "second-hand-market-backend/backend/tests|TestStrictImageVipsHTTPIntegration/jpeg"
  "second-hand-market-backend/backend/tests|TestStrictImageVipsHTTPIntegration/webp"
  "second-hand-market-backend/backend/tests|TestStrictImageVipsHTTPIntegration/heif"
  "second-hand-market-backend/backend/tests|TestStrictImageVipsHTTPIntegration/heic_declared_heif"
  "second-hand-market-backend/backend/tests|TestStrictImageVipsHTTPIntegration/heif_declared_heic"
)

for required_test in "${required_tests[@]}"; do
  package_name="${required_test%%|*}"
  test_name="${required_test#*|}"
  if ! awk -v package_name="$package_name" -v test_name="$test_name" '
    index($0, "\"Action\":\"pass\"") &&
    index($0, "\"Package\":\"" package_name "\"") &&
    index($0, "\"Test\":\"" test_name "\"") { found = 1 }
    END { exit(found ? 0 : 1) }
  ' "$test_log"; then
    fail "required test did not report PASS: $package_name $test_name"
  fi
done

run_logged() {
  local name="$1"
  local directory="$2"
  shift 2
  local log_path="$evidence_dir/$name.log"
  local pipeline_status
  set +e
  (
    cd "$directory"
    timeout --foreground "$command_timeout" "$@"
  ) 2>&1 | tee "$log_path"
  pipeline_status=("${PIPESTATUS[@]}")
  set -e
  printf '%s_exit_code=%d\n' "$name" "${pipeline_status[0]}"
  printf '%s_tee_exit_code=%d\n' "$name" "${pipeline_status[1]}"
  [[ "${pipeline_status[0]}" -eq 0 && "${pipeline_status[1]}" -eq 0 ]] ||
    fail "$name or its evidence capture failed"
}

run_logged frontend-install "$repo_root/frontend" npm ci --no-audit --no-fund
run_logged miniapp-install "$repo_root/miniapp" npm ci --no-audit --no-fund

frontend_report="$evidence_dir/frontend-vitest.json"
miniapp_report="$evidence_dir/miniapp-vitest.json"
run_logged frontend-test "$repo_root/frontend" npm test -- --reporter=json --outputFile="$frontend_report"
run_logged frontend-build "$repo_root/frontend" npm run build
run_logged miniapp-test "$repo_root/miniapp" npm test -- --pool=threads --maxWorkers=1 --minWorkers=1 --reporter=json --outputFile="$miniapp_report"
run_logged miniapp-build-weapp "$repo_root/miniapp" npm run build:weapp
run_logged miniapp-build-tt "$repo_root/miniapp" npm run build:tt

node "$repo_root/scripts/acceptance/validate-vitest-report.mjs" \
  "$frontend_report" \
  "src/vite-proxy.test.ts::development asset proxy routes API and guarded uploads through the same backend" \
  "src/utils/imageMime.test.ts::image MIME normalization falls back to the server-supported extension for application/octet-stream / photo.heic" \
  "src/utils/imageMime.test.ts::image MIME normalization does not pass an unsupported browser MIME to presign" ||
  fail "frontend Vitest report validation failed"

node "$repo_root/scripts/acceptance/validate-vitest-report.mjs" \
  "$miniapp_report" \
  "tests/asset-url.test.ts::小程序图片地址解析 将后端 /uploads 路径绑定到 API 源站" \
  "tests/asset-url.test.ts::小程序图片地址解析 拒绝非图片或未知协议 javascript:alert(1)" \
  "tests/config-api-base-url.test.ts::小程序 API 基址选择 显式传入 TARO_APP_API_BASE_URL 时优先使用覆盖值" ||
  fail "miniapp Vitest report validation failed"

pass_count="$(
  awk '
    index($0, "\"Action\":\"pass\"") &&
    index($0, "\"Test\":") { count++ }
    END { print count + 0 }
  ' "$test_log"
)"
printf 'passed_test_actions=%s\n' "$pass_count"
if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  fail "acceptance commands changed tracked or untracked source files"
fi
printf 'strict-image-pipeline acceptance: PASS\n'
acceptance_succeeded=1
