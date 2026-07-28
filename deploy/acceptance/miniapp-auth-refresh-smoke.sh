#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
miniapp_dir="$repo_dir/miniapp"
evidence_dir="$base_dir/evidence/miniapp-auth-refresh"
expected_node="v22.22.2"
expected_npm="10.9.7"

source_path_is_allowed() {
  local path="$1"
  case "$path" in
    Makefile | miniapp/.nvmrc | miniapp/babel.config.js | miniapp/package.json | \
      miniapp/package-lock.json | miniapp/project.config.json | \
      miniapp/project.tt.json | miniapp/tsconfig.json | miniapp/vitest.config.mjs | \
      deploy/acceptance/miniapp-auth-refresh-smoke.sh | miniapp/config/* | \
      miniapp/src/* | miniapp/tests/*)
      return 0
      ;;
  esac
  return 1
}

write_source_file_list() {
  (
    cd "$repo_dir"
    git ls-tree -r --name-only -z HEAD -- Makefile miniapp \
      deploy/acceptance/miniapp-auth-refresh-smoke.sh |
      while IFS= read -r -d '' path; do
        source_path_is_allowed "$path" && printf '%s\0' "$path"
      done | LC_ALL=C sort -zu
  )
}

write_directory_manifest() {
  local directory="$1"
  local source_list="$2"
  local output="$3"
  (
    cd "$directory"
    xargs -0 sha256sum <"$source_list"
  ) >"$output"
}

write_context_file_list() {
  local directory="$1"
  (
    cd "$directory"
    find . -type f -print0 |
      while IFS= read -r -d '' path; do
        printf '%s\0' "${path#./}"
      done | LC_ALL=C sort -zu
  )
}

source_list_contains() {
  local source_list="$1"
  local required="$2"
  local path
  while IFS= read -r -d '' path; do
    [[ "$path" == "$required" ]] && return 0
  done <"$source_list"
  return 1
}

validate_source_list() {
  local source_list="$1"
  local sorted_list="$2"
  local path
  local required
  local count=0
  local -a required_paths=(
    Makefile
    miniapp/.nvmrc
    miniapp/package.json
    miniapp/package-lock.json
    miniapp/project.config.json
    miniapp/project.tt.json
    miniapp/src/services/request.ts
    miniapp/tests/request-refresh.test.ts
    deploy/acceptance/miniapp-auth-refresh-smoke.sh
  )

  LC_ALL=C sort -zu "$source_list" >"$sorted_list"
  cmp -s "$source_list" "$sorted_list" || return 1
  while IFS= read -r -d '' path; do
    [[ -n "$path" && "$path" != /* && "$path" != ../* && "$path" != */../* ]] || return 1
    source_path_is_allowed "$path" || return 1
    count=$((count + 1))
  done <"$source_list"
  [[ "$count" -gt 0 ]] || return 1
  for required in "${required_paths[@]}"; do
    source_list_contains "$source_list" "$required" || return 1
  done
}

validate_received_source_files() {
  local directory="$1"
  local source_list="$2"
  local path
  while IFS= read -r -d '' path; do
    [[ -f "$directory/$path" && ! -L "$directory/$path" ]] || return 1
  done <"$source_list"
}

export_head_source() {
  local export_dir="$1"
  local export_runtime=""
  local extracted
  local path
  local -a archive_paths=()

  [[ "$export_dir" == /* && "$export_dir" != "/" && ! -e "$export_dir" ]] || {
    echo "MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2
    return 1
  }
  for command in git sha256sum sort xargs mktemp tar chmod mkdir rm cmp find; do
    command -v "$command" >/dev/null || {
      echo "required source export command is unavailable: $command" >&2
      return 1
    }
  done

  export_runtime="$(mktemp -d)"
  extracted="$export_runtime/extracted"
  if ! mkdir -p "$export_dir" "$extracted"; then
    rm -r -- "$export_runtime"
    rm -r -- "$export_dir"
    return 1
  fi
  if ! chmod 700 "$export_dir" "$export_runtime" "$extracted"; then
    rm -r -- "$export_runtime"
    rm -r -- "$export_dir"
    return 1
  fi
  if ! write_source_file_list >"$export_dir/source-files.z" ||
    ! validate_source_list "$export_dir/source-files.z" "$export_runtime/sorted-source-files.z"; then
    echo "committed HEAD miniapp source list is invalid" >&2
    rm -r -- "$export_runtime"
    rm -r -- "$export_dir"
    return 1
  fi
  while IFS= read -r -d '' path; do
    archive_paths+=("$path")
  done <"$export_dir/source-files.z"
  if [[ "${#archive_paths[@]}" -eq 0 ]] || ! (
    cd "$repo_dir"
    git archive --format=tar --output="$export_dir/source.tar" HEAD -- "${archive_paths[@]}"
  ) || ! tar -C "$extracted" -xf "$export_dir/source.tar" ||
    ! validate_received_source_files "$extracted" "$export_dir/source-files.z" ||
    ! write_context_file_list "$extracted" >"$export_runtime/archive-source-files.z" ||
    ! cmp -s "$export_dir/source-files.z" "$export_runtime/archive-source-files.z" ||
    ! write_directory_manifest "$extracted" "$export_dir/source-files.z" "$export_dir/source-sha256.txt" || ! (
      cd "$export_dir"
      sha256sum source-files.z source-sha256.txt source.tar >package-sha256.txt
    ); then
    echo "committed HEAD miniapp archive does not match its source list" >&2
    rm -r -- "$export_runtime"
    rm -r -- "$export_dir"
    return 1
  fi
  if ! chmod 600 "$export_dir/source-files.z" "$export_dir/source-sha256.txt" \
    "$export_dir/source.tar" "$export_dir/package-sha256.txt"; then
    rm -r -- "$export_runtime"
    rm -r -- "$export_dir"
    return 1
  fi
  rm -r -- "$export_runtime"
}

if [[ "${MINIAPP_AUTH_REFRESH_SOURCE_LIST_ONLY:-0}" == "1" &&
  -n "${MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR:-}" ]]; then
  echo "choose one miniapp auth refresh source mode" >&2
  exit 1
fi
if [[ "${MINIAPP_AUTH_REFRESH_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
  exit 0
fi
if [[ -n "${MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR:-}" ]]; then
  export_head_source "$MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR"
  exit 0
fi

[[ "${MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS" ]] || {
  echo "isolated miniapp auth refresh confirmation is missing" >&2
  exit 1
}

for command in sha256sum sort xargs mktemp tar chmod mkdir rm cmp find wc tr cut grep cp; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done

source_package_dir="${MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_DIR:-$repo_dir/.miniapp-auth-refresh-source}"
[[ "$source_package_dir" == /* && -d "$source_package_dir" && ! -L "$source_package_dir" ]] || {
  echo "MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_DIR must identify the transferred source package" >&2
  exit 1
}
for artifact in source-files.z source-sha256.txt source.tar package-sha256.txt; do
  [[ -f "$source_package_dir/$artifact" && ! -L "$source_package_dir/$artifact" ]] || {
    echo "transferred miniapp source package is incomplete" >&2
    exit 1
  }
done

validate_package_checksums() {
  local package_dir="$1"
  local checksum_file="$package_dir/package-sha256.txt"
  local expected_hash
  local expected_name
  local actual_hash
  local line_count=0
  local -a expected_names=(source-files.z source-sha256.txt source.tar)

  while read -r expected_hash expected_name; do
    [[ "$line_count" -lt "${#expected_names[@]}" &&
      "$expected_name" == "${expected_names[$line_count]}" &&
      "${#expected_hash}" -eq 64 && "$expected_hash" != *[!0-9a-f]* ]] || return 1
    actual_hash="$(sha256sum "$package_dir/$expected_name" | cut -d ' ' -f1)"
    [[ "$actual_hash" == "$expected_hash" ]] || return 1
    line_count=$((line_count + 1))
  done <"$checksum_file"
  [[ "$line_count" -eq "${#expected_names[@]}" ]]
}

authorized_package_manifest_sha256="${MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_MANIFEST_SHA256:-}"
actual_package_manifest_sha256="$(sha256sum "$source_package_dir/package-sha256.txt" | cut -d ' ' -f1)"
[[ "${#authorized_package_manifest_sha256}" -eq 64 &&
  "$authorized_package_manifest_sha256" != *[!0-9a-f]* &&
  "$actual_package_manifest_sha256" == "$authorized_package_manifest_sha256" ]] || {
  echo "source package manifest digest does not match authorization" >&2
  exit 1
}
validate_package_checksums "$source_package_dir" || {
  echo "transferred miniapp source package checksum failed" >&2
  exit 1
}

[[ ! -e "$evidence_dir" ]] || {
  echo "refusing to overwrite existing miniapp auth refresh evidence" >&2
  exit 1
}

runtime_dir="$(mktemp -d)"
runtime_evidence="$runtime_dir/evidence"
build_context="$runtime_dir/build-context"
current_stage="preflight"
success=0
evidence_eligible=0
sanitization_failed=0
source_files="$source_package_dir/source-files.z"
source_manifest="$source_package_dir/source-sha256.txt"

hash_evidence_directory() {
  local directory="$1"
  (
    cd "$directory"
    find . -type f ! -name 'evidence-sha256.txt' -print0 |
      LC_ALL=C sort -z | xargs -0 sha256sum
  ) >"$directory/evidence-sha256.txt"
}

publish_evidence_directory() {
  local directory="$1"
  mkdir -p "$evidence_dir"
  chmod 700 "$evidence_dir"
  (
    cd "$directory"
    tar -cf - .
  ) | tar -C "$evidence_dir" -xf -
}

checkpoint_file_is_safe() {
  local file="$1"
  local line
  local count=0
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS= read -r line; do
    printf '%s\n' "$line" |
      grep -Eq '^classification=(source_package|toolchain|npm_ci|focused_tests|full_tests|build_weapp|build_tt)\|result=PASS\|count=[0-9]+(\|sha256=[0-9a-f]{64})?$' || return 1
    count=$((count + 1))
  done <"$file"
  [[ "$count" -gt 0 ]]
}

scan_evidence_directory() {
  local directory="$1"
  local output="$2"
  local scan_status=0
  grep -ERn --binary-files=text 'Authorization|Bearer[[:space:]]|access_token|refresh_token|token["=:]|password["=:]|DB_DSN=|JWT_(ACCESS|REFRESH)_SECRET=|openid["=:]|raw-miniapp-secret' \
    "$directory" >"$output" || scan_status=$?
  [[ "$scan_status" -eq 1 ]]
}

publish_sanitization_failure() {
  local safe_dir="$runtime_dir/safe-sanitization-failure"
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  printf 'classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n' >"$safe_dir/failure-status.txt"
  printf 'classification=evidence_scan|result=FAIL|count=1\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

retain_failure_evidence() {
  local safe_dir="$runtime_dir/safe-failure-evidence"
  [[ "$sanitization_failed" -eq 0 && "$current_stage" != *[!a-z0-9_]* ]] || {
    publish_sanitization_failure
    return
  }
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' "$current_stage" >"$safe_dir/failure-status.txt"
  checkpoint_file_is_safe "$runtime_evidence/acceptance-results.txt" || {
    publish_sanitization_failure
    return
  }
  cp "$runtime_evidence/acceptance-results.txt" "$safe_dir/acceptance-results.txt"
  scan_evidence_directory "$safe_dir" "$runtime_dir/safe-failure-leaks.txt" || {
    publish_sanitization_failure
    return
  }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 && ! -e "$evidence_dir" ]]; then
    retain_failure_evidence || true
  fi
  if [[ -n "$runtime_dir" && -d "$runtime_dir" ]]; then
    rm -r -- "$runtime_dir"
  fi
  exit "$status"
}
trap on_exit EXIT
trap 'on_exit 130' INT
trap 'on_exit 143' TERM

mkdir -p "$runtime_evidence" "$build_context"
chmod 700 "$runtime_dir" "$runtime_evidence" "$build_context"
validate_source_list "$source_files" "$runtime_dir/sorted-source-files.z" || {
  echo "transferred miniapp source list is invalid" >&2
  exit 1
}
validate_received_source_files "$repo_dir" "$source_files" || {
  echo "received miniapp source contains a missing or unsafe file" >&2
  exit 1
}
write_directory_manifest "$repo_dir" "$source_files" "$runtime_dir/received-source-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/received-source-sha256.txt" || {
  echo "received miniapp source does not match package manifest" >&2
  exit 1
}
tar -C "$build_context" -xf "$source_package_dir/source.tar"
write_context_file_list "$build_context" >"$runtime_dir/build-context-files.z"
cmp -s "$source_files" "$runtime_dir/build-context-files.z" || {
  echo "source archive contents do not match the committed source list" >&2
  exit 1
}
write_directory_manifest "$build_context" "$source_files" "$runtime_dir/build-context-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/build-context-sha256.txt" || {
  echo "temporary miniapp build context does not match package manifest" >&2
  exit 1
}

source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
manifest_sha256="$(sha256sum "$source_manifest" | cut -d ' ' -f1)"
printf 'classification=source_package|result=PASS|count=%s|sha256=%s\n' "$source_count" "$manifest_sha256" >"$runtime_evidence/acceptance-results.txt"
evidence_eligible=1

command -v node >/dev/null || { echo "node is required" >&2; exit 1; }
command -v npm >/dev/null || { echo "npm is required" >&2; exit 1; }
current_stage="toolchain"
node_version="$(node --version)"
npm_version="$(npm --version)"
[[ "$node_version" == "$expected_node" ]] || { echo "node must be $expected_node (found $node_version)" >&2; exit 1; }
[[ "$npm_version" == "$expected_npm" ]] || { echo "npm must be $expected_npm (found $npm_version)" >&2; exit 1; }
printf 'classification=toolchain|result=PASS|count=2\n' >>"$runtime_evidence/acceptance-results.txt"

export TARO_APP_API_BASE_URL="https://example.invalid/api/v1"
run_in_miniapp() {
  local classification="$1"
  shift
  current_stage="$classification"
  (
    cd "$build_context/miniapp"
    "$@"
  ) >"$runtime_dir/$classification.log" 2>&1
  printf 'classification=%s|result=PASS|count=1\n' "$classification" >>"$runtime_evidence/acceptance-results.txt"
}

run_in_miniapp npm_ci npm ci --registry=https://registry.npmmirror.com --replace-registry-host=always
run_in_miniapp focused_tests npm test -- --run tests/request-refresh.test.ts
run_in_miniapp full_tests npm test
run_in_miniapp build_weapp npm run build:weapp
run_in_miniapp build_tt npm run build:tt

scan_evidence_directory "$runtime_evidence" "$runtime_dir/success-evidence-leaks.txt" || {
  sanitization_failed=1
  exit 1
}
printf 'classification=evidence_scan|result=PASS|count=0\n' >"$runtime_evidence/evidence-leak-scan.txt"
hash_evidence_directory "$runtime_evidence"
publish_evidence_directory "$runtime_evidence"
success=1
echo "isolated miniapp auth refresh acceptance passed"
