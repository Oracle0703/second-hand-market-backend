#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-license-privacy-acceptance"
retained_evidence_dir="$base_dir/evidence/license-file-privacy"
runtime_dir=""
evidence_dir=""
project_touched=0
evidence_eligible=0
success=0
current_stage="preflight"
sanitization_failed=0
evidence_publish_tmp=""
evidence_publish_lock=""
source_export_dir=""
source_export_runtime=""
source_export_complete=0
evidence_forbidden_pattern='Authorization|Bearer[[:space:]]|access_token|refresh_token|token["=:]|password["=:]|DB_DSN=|MYSQL_(DATABASE|USER|PASSWORD|ROOT_PASSWORD)=|JWT_(ACCESS|REFRESH)_SECRET=|license-privacy-secret|TestLicenseFilePrivacy|000[0-9]_[[:alnum:]_-]+\.(preflight|up|postflight)\.sql|missing-file-records'
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)

required_source_paths=(
  Makefile
  backend/Dockerfile
  backend/go.mod
  backend/go.sum
  backend/migrations/0007_license_file_privacy.preflight.sql
  backend/migrations/0007_license_file_privacy.up.sql
  backend/migrations/0007_license_file_privacy.postflight.sql
  backend/migrations/license_file_privacy_migration_test.go
  backend/tests/file_schema_mysql_test.go
  backend/tests/license_file_privacy_test.go
  backend/tests/license_file_privacy_acceptance_contract_test.go
  deploy/acceptance/docker-compose.yml
  deploy/acceptance/license-file-privacy-smoke.sh
)

source_path_is_forbidden() {
  local path="$1" lower component
  local -a components=()
  lower="$(printf '%s' "$path" | LC_ALL=C tr '[:upper:]' '[:lower:]')"
  [[ "$path" == "backend/app.db" || "$path" == docs/* ]] && return 0
  case "$lower" in *.db|*.db.*|*.sqlite|*.sqlite.*|*.sqlite3|*.sqlite3.*) return 0;; esac
  IFS=/ read -r -a components <<<"$lower"
  for component in "${components[@]}"; do
    case "$component" in
      .env|.env.*|.git|.tmp|.cache|cache|caches|secret|secrets|database|databases|upload|uploads|evidence|backup|backups|node_modules) return 0;;
    esac
  done
  return 1
}

source_path_is_allowed() {
  local path="$1"
  case "$path" in
    Makefile|backend/Dockerfile|backend/go.mod|backend/go.sum|backend/*.go|backend/migrations/*.sql|deploy/acceptance/license-file-privacy-smoke.sh|deploy/acceptance/docker-compose.yml|deploy/acceptance/sql/*.sql) return 0;;
  esac
  return 1
}

source_path_is_portable() {
  local path="$1" component
  local -a components=()
  [[ -n "$path" && "$path" != /* && "$path" != *//* &&
    "$path" != *[!A-Za-z0-9_./-]* ]] || return 1
  IFS=/ read -r -a components <<<"$path"
  for component in "${components[@]}"; do
    [[ -n "$component" && "$component" != . && "$component" != .. ]] || return 1
  done
}

write_source_file_list() {
  (
    cd "$repo_dir"
    git ls-tree -r --name-only -z HEAD -- Makefile backend deploy/acceptance |
      while IFS= read -r -d '' path; do
        source_path_is_portable "$path" || continue
        source_path_is_forbidden "$path" && continue
        source_path_is_allowed "$path" && printf '%s\0' "$path"
      done | LC_ALL=C sort -zu
  )
}

source_list_contains() {
  local source_list="$1" required="$2" path
  while IFS= read -r -d '' path; do [[ "$path" == "$required" ]] && return 0; done <"$source_list"
  return 1
}

validate_source_list() {
  local source_list="$1" sorted_list="$2" path required count=0
  LC_ALL=C sort -zu "$source_list" >"$sorted_list"
  cmp -s "$source_list" "$sorted_list" || return 1
  while IFS= read -r -d '' path; do
    source_path_is_portable "$path" || return 1
    source_path_is_forbidden "$path" && return 1
    source_path_is_allowed "$path" || return 1
    count=$((count + 1))
  done <"$source_list"
  [[ "$count" -gt 0 ]] || return 1
  for required in "${required_source_paths[@]}"; do source_list_contains "$source_list" "$required" || return 1; done
}

write_context_file_list() {
  local directory="$1"
  ( cd "$directory"; find . -type f -print0 | while IFS= read -r -d '' path; do printf '%s\0' "${path#./}"; done | LC_ALL=C sort -zu )
}

validate_received_source_files() {
  local directory="$1" source_list="$2" path
  while IFS= read -r -d '' path; do [[ -f "$directory/$path" && ! -L "$directory/$path" ]] || return 1; done <"$source_list"
}

write_directory_manifest() {
  local directory="$1" source_list="$2" output="$3"
  ( cd "$directory"; xargs -0 sha256sum <"$source_list" ) >"$output"
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
  if IFS= read -r line <&3; then exec 3<&-; return 1; fi
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

source_export_on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$source_export_complete" -ne 1 && -n "$source_export_dir" &&
    "$source_export_dir" == /* && "$source_export_dir" != / && -e "$source_export_dir" ]]; then
    rm -r -- "$source_export_dir"
  fi
  if [[ -n "$source_export_runtime" && -d "$source_export_runtime" ]]; then
    rm -r -- "$source_export_runtime"
  fi
  exit "$status"
}

export_head_source() {
  local export_dir="$1" export_runtime extracted path
  local -a archive_paths=()
  [[ "$export_dir" == /* && "$export_dir" != / && ! -e "$export_dir" ]] || { echo "LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2; return 1; }
  for command in git sha256sum sort mktemp tar chmod mkdir rm tr cmp find xargs; do command -v "$command" >/dev/null || { echo "required source export command is unavailable: $command" >&2; return 1; }; done
  source_export_dir="$export_dir"
  source_export_complete=0
  trap source_export_on_exit EXIT
  trap 'source_export_on_exit 130' INT
  trap 'source_export_on_exit 143' TERM
  export_runtime="$(mktemp -d)"; source_export_runtime="$export_runtime"; extracted="$export_runtime/extracted"
  mkdir -p "$export_dir" "$extracted"
  chmod 700 "$export_dir" "$export_runtime" "$extracted"
  write_source_file_list >"$export_dir/source-files.z"
  validate_source_list "$export_dir/source-files.z" "$export_runtime/sorted-source-files.z" || { echo "committed HEAD source list is invalid" >&2; return 1; }
  while IFS= read -r -d '' path; do archive_paths+=("$path"); done <"$export_dir/source-files.z"
  [[ "${#archive_paths[@]}" -gt 0 ]] || { echo "committed HEAD source whitelist is empty" >&2; return 1; }
  ( cd "$repo_dir"; git archive --format=tar --output="$export_dir/source.tar" HEAD -- "${archive_paths[@]}" )
  tar -C "$extracted" -xf "$export_dir/source.tar"
  validate_received_source_files "$extracted" "$export_dir/source-files.z" || return 1
  write_context_file_list "$extracted" >"$export_runtime/archive-files.z"
  cmp -s "$export_dir/source-files.z" "$export_runtime/archive-files.z" || return 1
  write_directory_manifest "$extracted" "$export_dir/source-files.z" "$export_dir/source-sha256.txt"
  ( cd "$export_dir"; sha256sum source-files.z source-sha256.txt source.tar >package-sha256.txt )
  chmod 600 "$export_dir/source-files.z" "$export_dir/source-sha256.txt" "$export_dir/source.tar" "$export_dir/package-sha256.txt"
  validate_package_artifact_list "$export_dir"
  validate_package_checksums "$export_dir"
  rm -r -- "$export_runtime"
  source_export_runtime=""
  source_export_complete=1
  trap - EXIT INT TERM
}

checkpoint_pass_line_is_safe() {
  local index="$1" line="$2"
  case "$index" in
    0) [[ "$line" =~ ^classification=source_package\|result=PASS\|count=[1-9][0-9]*\|sha256=[0-9a-f]{64}$ ]];;
    1) [[ "$line" == 'classification=mysql_version|result=PASS|count=1' ]];;
    2) [[ "$line" == 'classification=license_preflight_failures|result=PASS|count=14' ]];;
    3) [[ "$line" == 'classification=clean_migration|result=PASS|count=1' ]];;
    4) [[ "$line" == 'classification=api_auto_migrate_false|result=PASS|count=1' ]];;
    5) [[ "$line" == 'classification=api_auto_migrate_true|result=PASS|count=1' ]];;
    6) [[ "$line" == 'classification=production_snapshot|result=PASS|count=3' ]];;
    *) return 1;;
  esac
}

failure_stage_is_safe() {
  case "$1" in
    production_before|mysql_start|mysql_version|license_preflight_failures|clean_migration|build_test_image|api_auto_migrate_false|api_auto_migrate_true|production_after|evidence_scan|evidence_publish) return 0;;
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
  if [[ "$mode" == success ]]; then [[ "$pass_count" -eq 7 && "$failure_count" -eq 0 ]]
  else [[ "$pass_count" -ge 1 && "$pass_count" -lt 7 && "$failure_count" -eq 1 ]]
  fi
}

snapshot_file_is_safe() {
  local file="$1" name id state rest extra count=0
  local -a expected=(/secondhand-market-api /secondhand-market-web /secondhand-market-mysql)
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS='|' read -r name id state rest extra; do
    [[ "$count" -lt 3 && "$name" == "${expected[$count]}" && -z "$extra" ]] || return 1
    if [[ "$id" == absent ]]; then [[ "$state" == absent && "$rest" == absent ]] || return 1
    else [[ "${#id}" -eq 64 && "$id" != *[!0-9a-f]* && "$state" =~ ^[a-z]+$ && "$rest" != *[!0-9]* ]] || return 1
    fi
    count=$((count + 1))
  done <"$file"
  [[ "$count" -eq 3 ]]
}

hash_evidence_directory() {
  local directory="$1"
  ( cd "$directory"; find . -type f ! -name evidence-sha256.txt -print0 | LC_ALL=C sort -z | xargs -0 sha256sum ) >"$directory/evidence-sha256.txt"
}

publish_evidence_directory() {
  local directory="$1" parent="${retained_evidence_dir%/*}" staging_name=""
  [[ ! -e "$retained_evidence_dir" && ! -L "$retained_evidence_dir" ]] || return 1
  mkdir -p "$parent" || return 1
  evidence_publish_lock="${retained_evidence_dir}.publish.lock"
  mkdir "$evidence_publish_lock" || {
    evidence_publish_lock=""
    return 1
  }
  if [[ -e "$retained_evidence_dir" || -L "$retained_evidence_dir" ]]; then
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! evidence_publish_tmp="$(mktemp -d "${retained_evidence_dir}.publish.XXXXXX")"; then
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  staging_name="${evidence_publish_tmp##*/}"
  if ! chmod 700 "$evidence_publish_tmp"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! ( cd "$directory" && tar -cf - . 2>>"$runtime_dir/evidence-copy-errors.raw" ) |
    tar -C "$evidence_publish_tmp" -xf - 2>>"$runtime_dir/evidence-copy-errors.raw"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! validate_evidence_staging_copy "$directory" "$evidence_publish_tmp"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! mv -n -- "$evidence_publish_tmp" "$retained_evidence_dir" 2>>"$runtime_dir/evidence-publish-errors.raw" ||
    [[ -e "$evidence_publish_tmp" ]]; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if [[ -d "$retained_evidence_dir/$staging_name" ]]; then
    rm -r -- "$retained_evidence_dir/$staging_name" || return 1
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! validate_evidence_staging_copy "$directory" "$retained_evidence_dir"; then
    evidence_publish_tmp=""
    evidence_publish_lock=""
    return 1
  fi
  evidence_publish_tmp=""
  if ! rmdir "$evidence_publish_lock"; then
    evidence_publish_lock=""
    return 1
  fi
  evidence_publish_lock=""
}

validate_evidence_staging_copy() {
  local source="$1" staged="$2" path
  local source_list="$runtime_dir/evidence-publish-source-files.z"
  local staged_list="$runtime_dir/evidence-publish-staged-files.z"
  [[ -d "$source" && ! -L "$source" && -d "$staged" && ! -L "$staged" ]] || return 1
  [[ -z "$(find "$source" -mindepth 1 ! -type f -print -quit)" ]] || return 1
  [[ -z "$(find "$staged" -mindepth 1 ! -type f -print -quit)" ]] || return 1
  write_context_file_list "$source" >"$source_list" || return 1
  write_context_file_list "$staged" >"$staged_list" || return 1
  cmp -s "$source_list" "$staged_list" || return 1
  while IFS= read -r -d '' path; do
    [[ -f "$staged/$path" && ! -L "$staged/$path" ]] || return 1
  done <"$source_list"
  write_directory_manifest "$source" "$source_list" "$runtime_dir/evidence-publish-source-sha256.txt" || return 1
  write_directory_manifest "$staged" "$staged_list" "$runtime_dir/evidence-publish-staged-sha256.txt" || return 1
  cmp -s "$runtime_dir/evidence-publish-source-sha256.txt" "$runtime_dir/evidence-publish-staged-sha256.txt"
}

scan_evidence_directory() {
  local directory="$1" scan_output="$2" scan_status=0
  grep -ERn --binary-files=text "$evidence_forbidden_pattern" "$directory" >"$scan_output" || scan_status=$?
  [[ "$scan_status" -eq 1 ]]
}

publish_sanitization_failure() {
  local fallback_dir="$runtime_dir/safe-sanitization-failure"
  if [[ -e "$fallback_dir" || -L "$fallback_dir" ]]; then rm -r -- "$fallback_dir" || return 1; fi
  mkdir "$fallback_dir" || return 1
  chmod 700 "$fallback_dir" || return 1
  printf 'classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n' >"$fallback_dir/acceptance-results.txt" || return 1
  printf 'classification=evidence_scan|result=FAIL|count=1\n' >"$fallback_dir/evidence-leak-scan.txt" || return 1
  hash_evidence_directory "$fallback_dir" || return 1
  chmod 600 "$fallback_dir"/*.txt || return 1
  publish_evidence_directory "$fallback_dir"
}

retain_failure_evidence() {
  local safe_dir="$runtime_dir/safe-failure-evidence" snapshot
  mkdir "$safe_dir" || { publish_sanitization_failure; return; }
  chmod 700 "$safe_dir" || { publish_sanitization_failure; return; }
  [[ "$sanitization_failed" -eq 0 ]] || { publish_sanitization_failure; return; }
  failure_stage_is_safe "$current_stage" || { publish_sanitization_failure; return; }
  cp "$evidence_dir/acceptance-results.txt" "$safe_dir/acceptance-results.txt" || { publish_sanitization_failure; return; }
  printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' "$current_stage" >>"$safe_dir/acceptance-results.txt" || { publish_sanitization_failure; return; }
  checkpoint_file_is_safe "$safe_dir/acceptance-results.txt" failure || { publish_sanitization_failure; return; }
  for snapshot in production-before.txt production-after.txt; do
    if [[ -e "$evidence_dir/$snapshot" ]]; then
      snapshot_file_is_safe "$evidence_dir/$snapshot" || { publish_sanitization_failure; return; }
      cp "$evidence_dir/$snapshot" "$safe_dir/$snapshot" || { publish_sanitization_failure; return; }
    fi
  done
  scan_evidence_directory "$safe_dir" "$runtime_dir/failure-evidence-leaks.raw" || { sanitization_failed=1; publish_sanitization_failure; return; }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt" || { publish_sanitization_failure; return; }
  hash_evidence_directory "$safe_dir" || { publish_sanitization_failure; return; }
  chmod 600 "$safe_dir"/*.txt || { publish_sanitization_failure; return; }
  publish_evidence_directory "$safe_dir"
}

publish_success_evidence() {
  local safe_dir="$runtime_dir/safe-success-evidence" snapshot
  mkdir "$safe_dir" || { publish_sanitization_failure; return 1; }
  chmod 700 "$safe_dir" || { publish_sanitization_failure; return 1; }
  checkpoint_file_is_safe "$evidence_dir/acceptance-results.txt" success || { publish_sanitization_failure; return 1; }
  cp "$evidence_dir/acceptance-results.txt" "$safe_dir/acceptance-results.txt" || { publish_sanitization_failure; return 1; }
  for snapshot in production-before.txt production-after.txt; do
    snapshot_file_is_safe "$evidence_dir/$snapshot" || { publish_sanitization_failure; return 1; }
    cp "$evidence_dir/$snapshot" "$safe_dir/$snapshot" || { publish_sanitization_failure; return 1; }
  done
  scan_evidence_directory "$safe_dir" "$runtime_dir/success-evidence-leaks.raw" || { sanitization_failed=1; publish_sanitization_failure; return 1; }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt" || { publish_sanitization_failure; return 1; }
  hash_evidence_directory "$safe_dir" || { publish_sanitization_failure; return 1; }
  chmod 600 "$safe_dir"/*.txt || { publish_sanitization_failure; return 1; }
  publish_evidence_directory "$safe_dir"
}

record_pass() {
  local classification="$1" count="$2" sha="${3:-}"
  if [[ "$classification" == source_package ]]; then
    [[ "$count" =~ ^[1-9][0-9]*$ && "$sha" =~ ^[0-9a-f]{64}$ ]] || { sanitization_failed=1; echo "refusing unsafe source evidence classification" >&2; exit 1; }
  else
    [[ -z "$sha" ]] || { sanitization_failed=1; echo "refusing unexpected evidence digest" >&2; exit 1; }
    case "$classification|$count" in
      mysql_version\|1|license_preflight_failures\|14|clean_migration\|1|api_auto_migrate_false\|1|api_auto_migrate_true\|1|production_snapshot\|3) ;;
      *) sanitization_failed=1; echo "refusing unsafe evidence classification" >&2; exit 1;;
    esac
  fi
  if [[ -n "$sha" ]]; then printf 'classification=%s|result=PASS|count=%s|sha256=%s\n' "$classification" "$count" "$sha" >>"$evidence_dir/acceptance-results.txt"
  else printf 'classification=%s|result=PASS|count=%s\n' "$classification" "$count" >>"$evidence_dir/acceptance-results.txt"; fi
}

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 && ! -e "$retained_evidence_dir" ]]; then
    if [[ "$project_touched" -eq 1 && ! -e "$evidence_dir/production-after.txt" ]]; then snapshot_production "$evidence_dir/production-after.txt" || true; fi
  fi
  if [[ "$project_touched" -eq 1 ]]; then "${compose[@]}" stop >"$runtime_dir/isolated-stop.raw" 2>&1 || true; fi
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 && ! -e "$retained_evidence_dir" ]]; then
    retain_failure_evidence || true
  fi
  if [[ -n "$evidence_publish_tmp" && -d "$evidence_publish_tmp" ]]; then rm -r -- "$evidence_publish_tmp"; fi
  if [[ -n "$evidence_publish_lock" && -d "$evidence_publish_lock" ]]; then rmdir "$evidence_publish_lock" || true; fi
  [[ -z "$runtime_dir" || ! -d "$runtime_dir" ]] || rm -r -- "$runtime_dir"
  exit "$status"
}

if [[ "${LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY:-0}" == "1" && -n "${LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR:-}" ]]; then
  echo "choose one license privacy source mode" >&2; exit 1
fi
if [[ "${LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY:-0}" == "1" ]]; then write_source_file_list; exit 0; fi
if [[ -n "${LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR:-}" ]]; then export_head_source "$LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR"; exit 0; fi

[[ "${LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA" ]] || {
  echo "isolated license file privacy confirmation is missing" >&2
  exit 1
}
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || {
  echo "ACCEPTANCE_DB_ENGINE must be mysql8.4" >&2
  exit 1
}
[[ "${COMPOSE_PROJECT_NAME:-}" == "$project_name" ]] || {
  echo "COMPOSE_PROJECT_NAME must be $project_name" >&2
  exit 1
}
[[ "$project_name" == "secondhand-license-privacy-acceptance" ]] || {
  echo "unexpected license privacy Compose project" >&2
  exit 1
}
for command in sha256sum sort xargs mktemp grep cmp tar chmod mkdir rm find wc tr cut cp mv; do
  command -v "$command" >/dev/null || { echo "required provenance command is unavailable: $command" >&2; exit 1; }
done
source_package_dir="${LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_DIR:-}"
[[ "$source_package_dir" == /* && -d "$source_package_dir" && ! -L "$source_package_dir" ]] || {
  echo "LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_DIR must identify the transferred source package" >&2
  exit 1
}
validate_package_artifact_list "$source_package_dir" || {
  echo "transferred license privacy source package must contain exactly four artifacts" >&2
  exit 1
}
for artifact in source-files.z source-sha256.txt source.tar package-sha256.txt; do
  [[ -f "$source_package_dir/$artifact" && ! -L "$source_package_dir/$artifact" ]] || {
    echo "transferred license privacy source package is incomplete" >&2; exit 1
  }
done
authorized_package_manifest_sha256="${LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_MANIFEST_SHA256:-}"
actual_package_manifest_sha256="$(sha256sum "$source_package_dir/package-sha256.txt" | cut -d ' ' -f1)"
[[ "${#authorized_package_manifest_sha256}" -eq 64 && "$authorized_package_manifest_sha256" != *[!0-9a-f]* && "$actual_package_manifest_sha256" == "$authorized_package_manifest_sha256" ]] || {
  echo "source package manifest digest does not match authorization" >&2; exit 1
}
validate_package_checksums "$source_package_dir" || { echo "transferred license privacy source package checksum failed" >&2; exit 1; }
runtime_dir="$(mktemp -d)"
trap on_exit EXIT
trap 'on_exit 130' INT
trap 'on_exit 143' TERM
build_context="$runtime_dir/build-context"
evidence_dir="$runtime_dir/evidence"
mkdir -p "$build_context" "$evidence_dir"
chmod 700 "$runtime_dir" "$build_context" "$evidence_dir"
: >"$evidence_dir/acceptance-results.txt"
source_files="$source_package_dir/source-files.z"
source_manifest="$source_package_dir/source-sha256.txt"
validate_source_list "$source_files" "$runtime_dir/sorted-source-files.z" || { echo "transferred license privacy source list is invalid" >&2; exit 1; }
validate_received_source_files "$repo_dir" "$source_files" || { echo "received license privacy source contains a missing or unsafe file" >&2; exit 1; }
write_directory_manifest "$repo_dir" "$source_files" "$runtime_dir/received-source-sha256.txt" || { echo "received license privacy source does not match package manifest" >&2; exit 1; }
cmp -s "$source_manifest" "$runtime_dir/received-source-sha256.txt" || { echo "received license privacy source does not match package manifest" >&2; exit 1; }
if ! tar -tvf "$source_package_dir/source.tar" >"$runtime_dir/source-archive-validation.raw" 2>&1; then
  echo "transferred license privacy source archive is invalid" >&2
  exit 1
fi
grep -Eq '^[lh]' "$runtime_dir/source-archive-validation.raw" && { echo "source archive contains a link" >&2; exit 1; }
if ! tar -C "$build_context" -xf "$source_package_dir/source.tar" >"$runtime_dir/source-archive-extract.raw" 2>&1; then
  echo "transferred license privacy source archive extraction failed" >&2
  exit 1
fi
[[ -z "$(find "$build_context" -type l -print -quit)" ]] || { echo "source archive contains a symlink" >&2; exit 1; }
write_context_file_list "$build_context" >"$runtime_dir/build-context-files.z"
cmp -s "$source_files" "$runtime_dir/build-context-files.z" || { echo "source archive contents do not match the committed source list" >&2; exit 1; }
write_directory_manifest "$build_context" "$source_files" "$runtime_dir/build-context-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/build-context-sha256.txt" || { echo "temporary build context does not match the committed source manifest" >&2; exit 1; }
source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
source_manifest_sha256="$(sha256sum "$source_manifest" | cut -d ' ' -f1)"
[[ ! -e "$retained_evidence_dir" && ! -L "$retained_evidence_dir" &&
  ! -e "${retained_evidence_dir}.publish.lock" && ! -L "${retained_evidence_dir}.publish.lock" ]] || {
  echo "refusing to overwrite existing license privacy evidence" >&2
  exit 1
}
[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}

existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q 2>>"$runtime_dir/project-collision.raw")"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q 2>>"$runtime_dir/project-collision.raw")"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q 2>>"$runtime_dir/project-collision.raw")"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}

compose_override="$runtime_dir/license-privacy-compose.yml"
cat >"$compose_override" <<EOF
services:
  mysql:
    volumes:
      - mysql-data:/var/lib/mysql
      - "$build_context/backend/migrations:/acceptance/migrations:ro"
  bootstrap-admin:
    build:
      context: "$build_context"
      dockerfile: backend/Dockerfile
      target: build
EOF
compose+=(--file "$compose_override")

snapshot_production() {
  local output="$1" container matches
  : >"$output"
  for container in "${production_containers[@]}"; do
    matches="$(docker container ls -a --filter "name=^/$container$" --format '{{.Names}}' 2>>"$runtime_dir/production-snapshot.raw")" || return 1
    if [[ -z "$matches" ]]; then printf '/%s|absent|absent|absent\n' "$container" >>"$output"; continue; fi
    [[ "$matches" == "$container" ]] || return 1
    docker inspect --type container --format '{{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}}' "$container" >>"$output" 2>>"$runtime_dir/production-snapshot.raw" || return 1
  done
}

record_pass source_package "$source_count" "$source_manifest_sha256"
current_stage="production_before"
snapshot_production "$evidence_dir/production-before.txt"
snapshot_file_is_safe "$evidence_dir/production-before.txt" || { echo "production-before snapshot is invalid" >&2; exit 1; }
evidence_eligible=1

mysql_sql() {
  local sql="$1"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="$1"
  ' sh "$sql" 2>>"$runtime_dir/mysql-sql.raw"
}

mysql_file() {
  local container_path="$1"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" < "$1"
  ' sh "$container_path"
}

reset_schema() {
  mysql_sql "
    SET FOREIGN_KEY_CHECKS=0;
    DROP TABLE IF EXISTS buyer_intents, buyer_histories, buyer_favorites,
      buyer_device_bindings, buyer_users, idempotency_records, auth_sessions,
      operation_logs, file_records, files, order_events, orders, product_images,
      products, categories, merchant_audit_logs, admin_users, merchant_accounts,
      merchants;
    SET FOREIGN_KEY_CHECKS=1;
  "
}

apply_chain_0001_0006() {
  reset_schema
  mysql_file /acceptance/migrations/0001_init.up.sql
  mysql_file /acceptance/migrations/0002_buyer_domain.up.sql
  mysql_file /acceptance/migrations/0003_buyer_auth_provider.up.sql
  mysql_file /acceptance/migrations/0004_merchant_multi_stock.preflight.sql
  mysql_file /acceptance/migrations/0004_merchant_multi_stock.up.sql
  mysql_file /acceptance/migrations/0004_merchant_multi_stock.postflight.sql
  mysql_file /acceptance/migrations/0005_file_records_table.preflight.sql
  mysql_file /acceptance/migrations/0005_file_records_table.up.sql
  mysql_file /acceptance/migrations/0005_file_records_table.postflight.sql
  mysql_file /acceptance/migrations/0006_file_binding_ownership.preflight.sql
  mysql_file /acceptance/migrations/0006_file_binding_ownership.up.sql
  mysql_file /acceptance/migrations/0006_file_binding_ownership.postflight.sql
}

valid_fixture_sql="
  INSERT INTO merchants (id,merchant_no,merchant_name,contact_name,contact_phone,review_status)
  VALUES (1,'M-PRIVACY-1','Privacy Merchant','Owner One','13800138001','APPROVED');
  INSERT INTO merchant_accounts (id,merchant_id,username,password_hash,role,status)
  VALUES (11,1,'privacy_owner_1','test-hash','OWNER','ACTIVE');
  INSERT INTO file_records
    (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status,owner_merchant_id)
  VALUES
    (301,'MERCHANT_LICENSE','merchant_license/historical.jpg','/uploads/merchant_license/historical.jpg','image/jpeg',22,'MERCHANT',11,'PASS',1),
    (302,'PRODUCT_IMAGE','product_image/historical.jpg','/uploads/product_image/historical.jpg','image/jpeg',22,'MERCHANT',11,'PASS',1);
  UPDATE merchants SET license_file_id=301 WHERE id=1;
"

capture_file_state() {
  local file_records_count files_count
  file_records_count="$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='file_records'")"
  files_count="$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='files'")"
  printf 'tables=%s/%s\n' "$file_records_count" "$files_count"
  if [[ "$file_records_count" == "1" ]]; then
    mysql_sql "
      SELECT CONCAT(
        'rows=', COUNT(*),
        '|licenses=', SUM(biz_type='MERCHANT_LICENSE'),
        '|digest=', COALESCE(SHA2(GROUP_CONCAT(
          CONCAT_WS('#',id,biz_type,object_key,url,mime_type,scan_status)
          ORDER BY id SEPARATOR '|'
        ),256),'EMPTY'))
      FROM file_records
    "
  fi
}

expect_preflight_failure() {
  local name="$1"
  local expected_message="$2"
  local fixture_sql="$3"
  apply_chain_0001_0006 >"$runtime_dir/$name-setup.raw" 2>&1
  mysql_sql "$valid_fixture_sql $fixture_sql" >>"$runtime_dir/$name-setup.raw" 2>&1
  capture_file_state >"$runtime_dir/$name-before.raw"
  if mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql >"$runtime_dir/$name-preflight.raw" 2>&1; then
    echo "expected license privacy preflight failure for $name" >&2
    exit 1
  fi
  grep -Eq -- 'ERROR 1644 \(45000\)' "$runtime_dir/$name-preflight.raw" || {
    echo "license privacy preflight $name did not fail with SQLSTATE 45000" >&2
    exit 1
  }
  grep -Fq -- "$expected_message" "$runtime_dir/$name-preflight.raw" || {
    echo "license privacy preflight $name failed for an unexpected reason" >&2
    exit 1
  }
  grep -Fq -- 'license_file_privacy_preflight_passed' "$runtime_dir/$name-preflight.raw" && {
    echo "license privacy preflight $name emitted a success marker" >&2
    exit 1
  }
  capture_file_state >"$runtime_dir/$name-after.raw"
  cmp -s "$runtime_dir/$name-before.raw" "$runtime_dir/$name-after.raw" || {
    echo "license privacy preflight $name changed file rows or license URLs" >&2
    exit 1
  }
}

current_stage="mysql_start"
project_touched=1
"${compose[@]}" up -d --wait mysql >"$runtime_dir/mysql-start.raw" 2>&1
current_stage="mysql_version"
mysql_version="$(mysql_sql 'SELECT VERSION()')"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated license file privacy acceptance requires MySQL 8.4.x, got $mysql_version" >&2
  exit 1
}
record_pass mysql_version 1
current_stage="license_preflight_failures"

expect_preflight_failure missing-file-records \
  'license privacy preflight: canonical file_records table is required' \
  'DROP TABLE file_records;'
expect_preflight_failure both-file-tables \
  'license privacy preflight: legacy files table must not exist' \
  'CREATE TABLE files LIKE file_records;'

for column in owner_merchant_id capability_token_hash capability_expires_at; do
  expect_preflight_failure "missing-column-$column" \
    "license privacy preflight: $column is missing or drifted" \
    "ALTER TABLE file_records DROP COLUMN $column;"
done

expect_preflight_failure missing-index-owner \
  'license privacy preflight: owner/biz/scan index is missing or drifted' \
  'ALTER TABLE file_records DROP INDEX idx_file_owner_biz_scan;'
expect_preflight_failure missing-index-token \
  'license privacy preflight: capability token index is missing or drifted' \
  'ALTER TABLE file_records DROP INDEX uk_file_capability_token;'
expect_preflight_failure missing-index-expiry \
  'license privacy preflight: capability expiry index is missing or drifted' \
  'ALTER TABLE file_records DROP INDEX idx_file_capability_expires;'
expect_preflight_failure empty-license-object-key \
  'license privacy preflight: invalid merchant license record' \
  "UPDATE file_records SET object_key='' WHERE id=301;"
expect_preflight_failure disallowed-license-mime \
  'license privacy preflight: invalid merchant license record' \
  "UPDATE file_records SET mime_type='application/pdf' WHERE id=301;"
expect_preflight_failure illegal-license-status \
  'license privacy preflight: invalid merchant license record' \
  "UPDATE file_records SET scan_status='UNKNOWN' WHERE id=301;"
expect_preflight_failure missing-license-owner \
  'license privacy preflight: invalid bound merchant license' \
  'UPDATE file_records SET owner_merchant_id=NULL WHERE id=301;'
expect_preflight_failure owner-reference-mismatch \
  'license privacy preflight: invalid bound merchant license' \
  'UPDATE file_records SET owner_merchant_id=2 WHERE id=301;'
expect_preflight_failure merchant-uploader-mismatch \
  'license privacy preflight: invalid bound merchant license' \
  "INSERT INTO merchants (id,merchant_no,merchant_name,contact_name,contact_phone,review_status)
   VALUES (2,'M-PRIVACY-2','Other Merchant','Owner Two','13800138002','APPROVED');
   INSERT INTO merchant_accounts (id,merchant_id,username,password_hash,role,status)
   VALUES (22,2,'privacy_owner_2','test-hash','OWNER','ACTIVE');
   UPDATE file_records SET uploader_id=22 WHERE id=301;"
record_pass license_preflight_failures 14

prepare_clean_fixture() {
apply_chain_0001_0006
mysql_sql "$valid_fixture_sql"
}

current_stage="clean_migration"
prepare_clean_fixture >"$runtime_dir/clean-setup.raw" 2>&1
product_url_before="$(mysql_sql "SELECT url FROM file_records WHERE id=302")"
license_url_before="$(mysql_sql "SELECT url FROM file_records WHERE id=301")"
file_count_before="$(mysql_sql 'SELECT COUNT(*) FROM file_records')"
license_count_before="$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE biz_type='MERCHANT_LICENSE'")"
[[ -n "$product_url_before" && -n "$license_url_before" ]] || {
  echo "valid historical fixture must begin with public product and license URLs" >&2
  exit 1
}

{
  mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql
  mysql_file /acceptance/migrations/0007_license_file_privacy.up.sql
  mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql
  mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql
  mysql_file /acceptance/migrations/0008_anonymous_upload_governance.up.sql
  mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql
  mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
  mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql
  mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
} >"$runtime_dir/clean-migration.raw" 2>&1

[[ "$(mysql_sql "SELECT url FROM file_records WHERE id=302")" == "$product_url_before" ]]
[[ -z "$(mysql_sql "SELECT url FROM file_records WHERE id=301")" ]]
[[ "$(mysql_sql 'SELECT COUNT(*) FROM file_records')" == "$file_count_before" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE biz_type='MERCHANT_LICENSE'")" == "$license_count_before" ]]
grep -q license_file_privacy_preflight_passed "$runtime_dir/clean-migration.raw"
grep -q license_file_privacy_postflight_passed "$runtime_dir/clean-migration.raw"
record_pass clean_migration 1

current_stage="build_test_image"
"${compose[@]}" --profile tools build bootstrap-admin >"$runtime_dir/build-test-image.raw" 2>&1
current_stage="api_auto_migrate_false"
"${compose[@]}" --profile tools run --rm \
  -e FILE_SCHEMA_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=false \
  -e FILE_UPLOAD_LOCAL_DIR=/tmp/license-file-privacy-uploads \
  bootstrap-admin go test ./tests -run '^TestLicenseFilePrivacyWithMigrationOnlyMySQL$' -count=1 -v \
  >"$runtime_dir/api-auto-migrate-false.raw" 2>&1
grep -q -- '--- PASS: TestLicenseFilePrivacyWithMigrationOnlyMySQL' "$runtime_dir/api-auto-migrate-false.raw"
mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql \
  >"$runtime_dir/post-api-auto-migrate-false.raw" 2>&1
record_pass api_auto_migrate_false 1

current_stage="api_auto_migrate_true"
"${compose[@]}" --profile tools run --rm \
  -e FILE_SCHEMA_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=true \
  -e FILE_UPLOAD_LOCAL_DIR=/tmp/license-file-privacy-uploads \
  bootstrap-admin go test ./tests -run '^TestLicenseFilePrivacyWithMigrationOnlyMySQL$' -count=1 -v \
  >"$runtime_dir/api-auto-migrate-true.raw" 2>&1
grep -q -- '--- PASS: TestLicenseFilePrivacyWithMigrationOnlyMySQL' "$runtime_dir/api-auto-migrate-true.raw"
mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql \
  >"$runtime_dir/post-api-auto-migrate-true.raw" 2>&1
record_pass api_auto_migrate_true 1

current_stage="production_after"
snapshot_production "$evidence_dir/production-after.txt"
snapshot_file_is_safe "$evidence_dir/production-after.txt" || { echo "production-after snapshot is invalid" >&2; exit 1; }
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}
record_pass production_snapshot 3
current_stage="evidence_scan"
checkpoint_file_is_safe "$evidence_dir/acceptance-results.txt" success || { sanitization_failed=1; echo "acceptance checkpoint validation failed" >&2; exit 1; }
current_stage="evidence_publish"
publish_success_evidence
success=1

echo "isolated license file privacy acceptance passed"
echo "mysql version: $mysql_version"
echo "isolated Compose project stopped: $project_name"
