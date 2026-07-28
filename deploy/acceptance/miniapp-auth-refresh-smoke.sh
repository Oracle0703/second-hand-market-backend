#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
miniapp_dir="$repo_dir/miniapp"
evidence_dir="$base_dir/evidence/miniapp-auth-refresh"
expected_node="v22.22.2"
expected_npm="10.9.7"
evidence_publish_tmp=""

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

source_path_is_forbidden() {
  local path="$1" lower component
  local -a components=()
  lower="$(printf '%s' "$path" | LC_ALL=C tr '[:upper:]' '[:lower:]')"
  [[ "$path" == "miniapp/project.private.config.json" ]] && return 0
  case "$lower" in
    *.db|*.db.*|*.sqlite|*.sqlite.*|*.sqlite3|*.sqlite3.*) return 0 ;;
  esac
  IFS=/ read -r -a components <<<"$lower"
  for component in "${components[@]}"; do
    case "$component" in
      .env|.env.*|.git|.tmp|.cache|.swc|cache|caches|secret|secrets|database|databases|upload|uploads|evidence|backup|backups|node_modules|dist)
        return 0
        ;;
    esac
  done
  return 1
}

source_path_is_portable() {
  local path="$1" component
  local -a components=()
  [[ -n "$path" && "$path" != /* && "$path" != -* && "$path" != *//* &&
    "$path" != *[!A-Za-z0-9_./-]* ]] || return 1
  IFS=/ read -r -a components <<<"$path"
  for component in "${components[@]}"; do
    [[ -n "$component" && "$component" != . && "$component" != .. ]] || return 1
  done
}

write_source_file_list() {
  (
    cd "$repo_dir"
    git ls-tree -r --name-only -z HEAD -- Makefile miniapp \
      deploy/acceptance/miniapp-auth-refresh-smoke.sh |
      while IFS= read -r -d '' path; do
        source_path_is_portable "$path" || continue
        source_path_is_forbidden "$path" && continue
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
    source_path_is_portable "$path" || return 1
    source_path_is_forbidden "$path" && return 1
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

source_list_contains_child() {
  local source_list="$1" directory="$2" path
  while IFS= read -r -d '' path; do
    [[ "$path" == "$directory"/* ]] && return 0
  done <"$source_list"
  return 1
}

validate_package_checksums() {
  local package_dir="$1" expected_name actual_hash line line_count=0
  local -a names=(source-files.z source-sha256.txt source.tar)
  exec 3<"$package_dir/package-sha256.txt"
  while [[ "$line_count" -lt 3 ]]; do
    expected_name="${names[$line_count]}"
    IFS= read -r line <&3 || { exec 3<&-; return 1; }
    actual_hash="$(sha256sum "$package_dir/$expected_name" | cut -d ' ' -f1)"
    [[ "$line" == "$actual_hash  $expected_name" ]] || { exec 3<&-; return 1; }
    line_count=$((line_count + 1))
  done
  if IFS= read -r line <&3; then
    exec 3<&-
    return 1
  fi
  exec 3<&-
}

validate_package_artifact_list() {
  local package_dir="$1" path count=0
  local -a names=(package-sha256.txt source-files.z source-sha256.txt source.tar)
  while IFS= read -r -d '' path; do
    [[ "$count" -lt 4 && "$path" == "./${names[$count]}" ]] || return 1
    count=$((count + 1))
  done < <(cd "$package_dir" && find . -mindepth 1 -maxdepth 1 -print0 | LC_ALL=C sort -z)
  [[ "$count" -eq 4 ]]
}

validate_archive_list() {
  local package_dir="$1" source_list="$2" runtime="$3" line path
  : >"$runtime/archive-source-files.z"
  while IFS= read -r line; do
    case "${line:0:1}" in
      -|d) ;;
      *) return 1 ;;
    esac
  done < <(tar -tvf "$package_dir/source.tar")
  while IFS= read -r path; do
    source_path_is_portable "${path%/}" || return 1
    if [[ "$path" == */ ]]; then
      path="${path%/}"
      source_path_is_forbidden "$path" && return 1
      source_list_contains_child "$source_list" "$path" || return 1
    else
      printf '%s\0' "$path" >>"$runtime/archive-source-files.z"
    fi
  done < <(tar -tf "$package_dir/source.tar")
  validate_source_list "$runtime/archive-source-files.z" "$runtime/sorted-archive-source-files.z" || return 1
  cmp -s "$source_list" "$runtime/archive-source-files.z"
}

export_head_source() (
  set -euo pipefail
  local export_dir="$1"
  local export_runtime=""
  local extracted=""
  local completed=0
  local path
  local -a archive_paths=()

  cleanup_source_export() {
    local status=$?
    trap - EXIT INT TERM
    if [[ -n "$export_runtime" && -d "$export_runtime" ]]; then
      rm -r -- "$export_runtime"
    fi
    if [[ "$completed" -ne 1 && -d "$export_dir" ]]; then
      rm -r -- "$export_dir"
    fi
    exit "$status"
  }
  trap cleanup_source_export EXIT INT TERM

  [[ "$export_dir" == /* && "$export_dir" != "/" && ! -e "$export_dir" ]] || {
    echo "MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2
    return 1
  }
  for command in git sha256sum sort xargs mktemp tar chmod mkdir rm cmp find tr cut; do
    command -v "$command" >/dev/null || {
      echo "required source export command is unavailable: $command" >&2
      return 1
    }
  done

  export_runtime="$(mktemp -d)"
  extracted="$export_runtime/extracted"
  mkdir -p "$export_dir" "$extracted"
  chmod 700 "$export_dir" "$export_runtime" "$extracted"
  if ! write_source_file_list >"$export_dir/source-files.z" ||
    ! validate_source_list "$export_dir/source-files.z" "$export_runtime/sorted-source-files.z"; then
    echo "committed HEAD miniapp source list is invalid" >&2
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
    ) || ! validate_package_artifact_list "$export_dir" ||
    ! validate_package_checksums "$export_dir"; then
    echo "committed HEAD miniapp archive does not match its source list" >&2
    return 1
  fi
  chmod 600 "$export_dir/source-files.z" "$export_dir/source-sha256.txt" \
    "$export_dir/source.tar" "$export_dir/package-sha256.txt"
  completed=1
)

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

for command in sha256sum sort xargs mktemp tar chmod mkdir rm mv cmp find wc tr cut grep cp; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done

source_package_dir="${MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_DIR:-}"
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
validate_package_artifact_list "$source_package_dir" || {
  echo "transferred miniapp source package must contain exactly four artifacts" >&2
  exit 1
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
  local directory="$1" parent="${evidence_dir%/*}"
  [[ ! -e "$evidence_dir" ]] || return 1
  mkdir -p "$parent" || return 1
  evidence_publish_tmp="$(mktemp -d "${evidence_dir}.publish.XXXXXX")" || return 1
  chmod 700 "$evidence_publish_tmp" || return 1
  if ! (cd "$directory" && tar -cf - .) | tar -C "$evidence_publish_tmp" -xf -; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    return 1
  fi
  if ! mv -- "$evidence_publish_tmp" "$evidence_dir"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    return 1
  fi
  evidence_publish_tmp=""
}

checkpoint_pass_line_is_safe() {
  local index="$1" line="$2"
  case "$index" in
    0) [[ "$line" =~ ^classification=source_package\|result=PASS\|count=[1-9][0-9]*\|sha256=[0-9a-f]{64}$ ]] ;;
    1) [[ "$line" == 'classification=toolchain|result=PASS|count=2' ]] ;;
    2) [[ "$line" == 'classification=npm_ci|result=PASS|count=1' ]] ;;
    3) [[ "$line" == 'classification=focused_tests|result=PASS|count=1' ]] ;;
    4) [[ "$line" == 'classification=full_tests|result=PASS|count=1' ]] ;;
    5) [[ "$line" == 'classification=build_weapp|result=PASS|count=1' ]] ;;
    6) [[ "$line" == 'classification=build_tt|result=PASS|count=1' ]] ;;
    *) return 1 ;;
  esac
}

failure_stage_is_safe() {
  case "$1" in
    toolchain|npm_ci|focused_tests|full_tests|build_weapp|build_tt|evidence_scan|evidence_publish) return 0 ;;
  esac
  return 1
}

checkpoint_file_is_safe() {
  local file="$1" mode="$2" line stage pass_count=0 failure_count=0
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS= read -r line; do
    if [[ "$line" == classification=acceptance_failure\|result=FAIL\|stage=*\|count=1 ]]; then
      [[ "$mode" == failure && "$failure_count" -eq 0 ]] || return 1
      stage="${line#classification=acceptance_failure|result=FAIL|stage=}"
      stage="${stage%|count=1}"
      failure_stage_is_safe "$stage" || return 1
      failure_count=1
      continue
    fi
    [[ "$failure_count" -eq 0 ]] || return 1
    checkpoint_pass_line_is_safe "$pass_count" "$line" || return 1
    pass_count=$((pass_count + 1))
  done <"$file"
  if [[ "$mode" == success ]]; then
    [[ "$pass_count" -eq 7 && "$failure_count" -eq 0 ]]
  else
    [[ "$pass_count" -ge 1 && "$pass_count" -le 7 && "$failure_count" -eq 1 ]]
  fi
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
  [[ ! -e "$safe_dir" ]] || rm -r -- "$safe_dir"
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  printf 'classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n' >"$safe_dir/acceptance-results.txt"
  printf 'classification=evidence_scan|result=FAIL|count=1\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

retain_failure_evidence() {
  local safe_dir="$runtime_dir/safe-failure-evidence"
  [[ "$sanitization_failed" -eq 0 ]] || {
    publish_sanitization_failure
    return
  }
  failure_stage_is_safe "$current_stage" || {
    publish_sanitization_failure
    return
  }
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  cp "$runtime_evidence/acceptance-results.txt" "$safe_dir/acceptance-results.txt"
  printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' "$current_stage" >>"$safe_dir/acceptance-results.txt"
  checkpoint_file_is_safe "$safe_dir/acceptance-results.txt" failure || {
    publish_sanitization_failure
    return
  }
  scan_evidence_directory "$safe_dir" "$runtime_dir/safe-failure-leaks.txt" || {
    publish_sanitization_failure
    return
  }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

publish_success_evidence() {
  checkpoint_file_is_safe "$runtime_evidence/acceptance-results.txt" success || {
    publish_sanitization_failure
    return 1
  }
  scan_evidence_directory "$runtime_evidence" "$runtime_dir/success-evidence-leaks.raw" || {
    sanitization_failed=1
    publish_sanitization_failure
    return 1
  }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$runtime_evidence/evidence-leak-scan.txt"
  hash_evidence_directory "$runtime_evidence"
  current_stage="evidence_publish"
  publish_evidence_directory "$runtime_evidence"
}

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 && ! -e "$evidence_dir" ]]; then
    retain_failure_evidence || true
  fi
  if [[ -n "$evidence_publish_tmp" && -d "$evidence_publish_tmp" ]]; then
    rm -r -- "$evidence_publish_tmp"
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
validate_archive_list "$source_package_dir" "$source_files" "$runtime_dir" || {
  echo "source archive list or entry type is unsafe" >&2
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

[[ ! -e "$evidence_dir" ]] || {
  echo "refusing to overwrite existing miniapp auth refresh evidence" >&2
  exit 1
}

source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
manifest_sha256="$(sha256sum "$source_manifest" | cut -d ' ' -f1)"
printf 'classification=source_package|result=PASS|count=%s|sha256=%s\n' "$source_count" "$manifest_sha256" >"$runtime_evidence/acceptance-results.txt"
evidence_eligible=1

current_stage="toolchain"
command -v node >/dev/null || { echo "node is required" >&2; exit 1; }
command -v npm >/dev/null || { echo "npm is required" >&2; exit 1; }
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
  ) >"$runtime_dir/$classification.raw" 2>&1
  printf 'classification=%s|result=PASS|count=1\n' "$classification" >>"$runtime_evidence/acceptance-results.txt"
}

run_in_miniapp npm_ci npm ci --registry=https://registry.npmmirror.com --replace-registry-host=always
run_in_miniapp focused_tests npm test -- --run tests/request-refresh.test.ts
run_in_miniapp full_tests npm test
run_in_miniapp build_weapp npm run build:weapp
run_in_miniapp build_tt npm run build:tt

current_stage="evidence_scan"
publish_success_evidence
success=1
echo "isolated miniapp auth refresh acceptance passed"
