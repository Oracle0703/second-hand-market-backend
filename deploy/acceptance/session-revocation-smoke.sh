#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-session-revocation-acceptance"
evidence_dir="$base_dir/evidence/session-access-revocation"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)
runtime_dir=""
success=0

required_source_paths=(
  Makefile
  backend/Dockerfile
  backend/go.mod
  backend/go.sum
  backend/internal/app/admin_handlers.go
  backend/internal/app/auth_handlers.go
  backend/internal/app/server.go
  backend/migrations/0001_init.up.sql
  backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
  backend/migrations/0009_buyer_intent_open_uniqueness.up.sql
  backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
  backend/tests/session_revocation_acceptance_contract_test.go
  backend/tests/session_revocation_mysql_test.go
  deploy/acceptance/README.md
  deploy/acceptance/docker-compose.yml
  deploy/acceptance/session-revocation-smoke.sh
)

source_path_is_forbidden() {
  local path="$1"
  local lower
  local component
  local -a components=()

  lower="$(printf '%s' "$path" | LC_ALL=C tr '[:upper:]' '[:lower:]')"
  [[ "$path" == "backend/app.db" || "$path" == docs/superpowers/* ]] && return 0
  case "$lower" in
    *.db | *.db.* | *.sqlite | *.sqlite.* | *.sqlite3 | *.sqlite3.*)
      return 0
      ;;
  esac
  IFS=/ read -r -a components <<<"$lower"
  for component in "${components[@]}"; do
    case "$component" in
      .env | .env.* | .git | .tmp | .cache | cache | caches | secret | secrets | \
        database | databases | upload | uploads | evidence | backup | backups | node_modules)
        return 0
        ;;
    esac
  done
  return 1
}

source_path_is_allowed() {
  local path="$1"
  case "$path" in
    Makefile | backend/Dockerfile | backend/go.mod | backend/go.sum | \
      backend/*.go | backend/migrations/*.sql | \
      deploy/acceptance/*.sh | deploy/acceptance/*.yml | \
      deploy/acceptance/*.yaml | deploy/acceptance/*.conf | \
      deploy/acceptance/*.md | deploy/acceptance/*.Dockerfile | \
      deploy/acceptance/sql/*.sql)
      return 0
      ;;
  esac
  return 1
}

write_source_file_list() {
  (
    cd "$repo_dir"
    git ls-tree -r --name-only -z HEAD -- Makefile backend deploy/acceptance |
      while IFS= read -r -d '' path; do
        source_path_is_forbidden "$path" && continue
        source_path_is_allowed "$path" && printf '%s\0' "$path"
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

source_list_contains_child() {
  local source_list="$1"
  local directory="$2"
  local path
  while IFS= read -r -d '' path; do
    [[ "$path" == "$directory"/* ]] && return 0
  done <"$source_list"
  return 1
}

validate_source_list() {
  local source_list="$1"
  local sorted_list="$2"
  local path
  local required
  local count=0

  LC_ALL=C sort -zu "$source_list" >"$sorted_list"
  cmp -s "$source_list" "$sorted_list" || return 1
  while IFS= read -r -d '' path; do
    [[ -n "$path" && "$path" != /* && "$path" != -* &&
      "$path" != ../* && "$path" != */../* && "$path" != */.. &&
      "$path" != ./* && "$path" != */./* && "$path" != *//* &&
      "$path" != *[!A-Za-z0-9._/-]* ]] || return 1
    source_path_is_forbidden "$path" && return 1
    source_path_is_allowed "$path" || return 1
    count=$((count + 1))
  done <"$source_list"
  [[ "$count" -gt 0 ]] || return 1
  for required in "${required_source_paths[@]}"; do
    source_list_contains "$source_list" "$required" || return 1
  done
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

validate_received_source_files() {
  local directory="$1"
  local source_list="$2"
  local path
  while IFS= read -r -d '' path; do
    [[ -f "$directory/$path" && ! -L "$directory/$path" ]] || return 1
  done <"$source_list"
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
    echo "SESSION_REVOCATION_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2
    return 1
  }
  for command in git sha256sum sort xargs mktemp tar chmod mkdir rm tr cmp find; do
    command -v "$command" >/dev/null || {
      echo "required source export command is unavailable: $command" >&2
      return 1
    }
  done

  export_runtime="$(mktemp -d)"
  extracted="$export_runtime/extracted"
  mkdir -p "$export_dir" "$extracted"
  chmod 700 "$export_dir" "$export_runtime" "$extracted"
  write_source_file_list >"$export_dir/source-files.z"
  validate_source_list "$export_dir/source-files.z" "$export_runtime/sorted-source-files.z" || {
    echo "committed HEAD session revocation source list is invalid" >&2
    return 1
  }
  while IFS= read -r -d '' path; do
    archive_paths+=("$path")
  done <"$export_dir/source-files.z"
  [[ "${#archive_paths[@]}" -gt 0 ]] || {
    echo "committed HEAD session revocation source whitelist is empty" >&2
    return 1
  }
  (
    cd "$repo_dir"
    git archive --format=tar --output="$export_dir/source.tar" HEAD -- "${archive_paths[@]}"
  )
  tar -C "$extracted" -xf "$export_dir/source.tar"
  validate_received_source_files "$extracted" "$export_dir/source-files.z" || {
    echo "committed HEAD session revocation archive contains a missing or unsafe file" >&2
    return 1
  }
  write_context_file_list "$extracted" >"$export_runtime/archive-source-files.z"
  cmp -s "$export_dir/source-files.z" "$export_runtime/archive-source-files.z" || {
    echo "committed HEAD session revocation archive does not match its source list" >&2
    return 1
  }
  write_directory_manifest "$extracted" "$export_dir/source-files.z" \
    "$export_dir/source-sha256.txt"
  (
    cd "$export_dir"
    sha256sum source-files.z source-sha256.txt source.tar >package-sha256.txt
  )
  chmod 600 "$export_dir/source-files.z" "$export_dir/source-sha256.txt" \
    "$export_dir/source.tar" "$export_dir/package-sha256.txt"
  completed=1
)

if [[ "${SESSION_REVOCATION_SOURCE_LIST_ONLY:-0}" == "1" &&
  -n "${SESSION_REVOCATION_SOURCE_EXPORT_DIR:-}" ]]; then
  echo "choose one session revocation source mode" >&2
  exit 1
fi
if [[ "${SESSION_REVOCATION_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
  exit 0
fi
if [[ -n "${SESSION_REVOCATION_SOURCE_EXPORT_DIR:-}" ]]; then
  export_head_source "$SESSION_REVOCATION_SOURCE_EXPORT_DIR"
  exit 0
fi

[[ "${SESSION_REVOCATION_ACCEPTANCE_CONFIRM:-}" == \
  "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA" ]] || {
  echo "isolated session revocation confirmation is missing" >&2
  exit 1
}
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || {
  echo "ACCEPTANCE_DB_ENGINE must be mysql8.4" >&2
  exit 1
}
[[ -z "${COMPOSE_PROJECT_NAME:-}" || "$COMPOSE_PROJECT_NAME" == "$project_name" ]] || {
  echo "COMPOSE_PROJECT_NAME must be $project_name when set" >&2
  exit 1
}
[[ "$project_name" == "secondhand-session-revocation-acceptance" ]] || {
  echo "unexpected session revocation Compose project" >&2
  exit 1
}
for command in sha256sum sort find xargs mktemp grep cmp tar chmod mkdir rm mv cp; do
  command -v "$command" >/dev/null || {
    echo "required source package command is unavailable: $command" >&2
    exit 1
  }
done

runtime_dir="$(mktemp -d)"
chmod 700 "$runtime_dir"
build_context="$runtime_dir/build-context"
source_results="$runtime_dir/acceptance-results.txt"
production_before="$runtime_dir/production-before.txt"
production_after="$runtime_dir/production-after.txt"
evidence_parent="$(dirname -- "$evidence_dir")"
evidence_published=0
evidence_eligible=0
evidence_publish_tmp=""
evidence_publish_lock=""
docker_available=0
project_may_exist=0
current_stage="source_package"
source_count=0
source_manifest_sha256=""

snapshot_production() {
  local output="$1"
  local line matches
  local container
  : >"$output"
  for container in "${production_containers[@]}"; do
    if ! matches="$(docker container ls -a \
      --filter "name=^/${container}$" --format '{{.Names}}' \
      2>>"$runtime_dir/production-snapshot-errors.raw")"; then
      return 1
    fi
    if [[ -z "$matches" ]]; then
      printf '/%s|absent|absent|absent\n' "$container" >>"$output"
    elif [[ "$matches" == "$container" ]]; then
      if ! line="$(docker inspect --type container \
        --format '{{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}}' \
        "$container" 2>>"$runtime_dir/production-snapshot-errors.raw")" ||
        [[ -z "$line" ]]; then
        return 1
      fi
      printf '%s\n' "$line" >>"$output"
    else
      return 1
    fi
  done
  snapshot_file_is_safe "$output"
}

snapshot_file_is_safe() {
  local file="$1" name id state restart_count extra count=0
  local -a expected=(/secondhand-market-api /secondhand-market-web /secondhand-market-mysql)
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS='|' read -r name id state restart_count extra; do
    [[ "$count" -lt 3 && "$name" == "${expected[$count]}" && -z "$extra" ]] || return 1
    if [[ "$id" == absent ]]; then
      [[ "$state" == absent && "$restart_count" == absent ]] || return 1
    else
      [[ "${#id}" -eq 64 && "$id" != *[!0-9a-f]* &&
        "$state" =~ ^[a-z]+$ && "$restart_count" =~ ^[0-9]+$ ]] || return 1
    fi
    count=$((count + 1))
  done <"$file"
  [[ "$count" -eq 3 ]]
}

write_evidence_hashes() {
  local directory="$1"
  local temporary="$directory/.evidence-sha256.tmp"
  if ! (
    cd "$directory"
    find . -maxdepth 1 -type f -name '*.txt' ! -name 'evidence-sha256.txt' -print0 |
      LC_ALL=C sort -z | xargs -0 sha256sum
  ) >"$temporary"; then
    rm -f -- "$temporary"
    return 1
  fi
  mv -- "$temporary" "$directory/evidence-sha256.txt" || {
    rm -f -- "$temporary"
    return 1
  }
}

scan_evidence_directory() {
  local directory="$1" output="$2" scan_status=0
  grep -ERn --binary-files=text \
    'Authorization|access_token|refresh_token|DB_DSN=|MYSQL_PASSWORD=|MYSQL_ROOT_PASSWORD=|JWT_ACCESS_SECRET=|JWT_REFRESH_SECRET=|FILE_UPLOAD_IP_HASH_SECRET=|eyJ[A-Za-z0-9_-]+\.|openid["=:]|session_id["=:]|user_id["=:]|actor_id["=:]' \
    "$directory" >"$output" || scan_status=$?
  [[ "$scan_status" -eq 1 ]]
}

failure_stage_is_safe() {
  case "$1" in
    production_before | mysql_start | mysql_version | bootstrap_build | migration_chain | \
      session_auto_migrate_false | migration_chain_true_reset | session_auto_migrate_true | \
      backend_tests | go_vet | production_snapshot | evidence_publication)
      return 0
      ;;
  esac
  return 1
}

classification_file_is_safe() {
  local file="$1" mode="$2" line count=0
  local -a expected=(
    'classification=mysql_version|result=PASS|count=1'
    'classification=migration_chain|result=PASS|count=1'
    'classification=session_auto_migrate_false|result=PASS|count=1'
    'classification=session_auto_migrate_true|result=PASS|count=1'
    'classification=backend_tests|result=PASS|count=1'
    'classification=go_vet|result=PASS|count=1'
    'classification=production_snapshot|result=PASS|count=3'
  )
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS= read -r line; do
    if [[ "$count" -eq 0 ]]; then
      [[ "$line" =~ ^classification=source_package\|result=PASS\|count=[1-9][0-9]*\|sha256=[0-9a-f]{64}$ ]] || return 1
    else
      [[ "$count" -le 7 && "$line" == "${expected[$((count - 1))]}" ]] || return 1
    fi
    count=$((count + 1))
  done <"$file"
  if [[ "$mode" == success ]]; then
    [[ "$count" -eq 8 ]]
  else
    [[ "$count" -ge 1 && "$count" -lt 8 ]]
  fi
}

validate_evidence_copy() {
  local source="$1" copied="$2" path
  local source_list="$runtime_dir/evidence-source-files.z"
  local copied_list="$runtime_dir/evidence-copied-files.z"
  [[ -d "$source" && ! -L "$source" && -d "$copied" && ! -L "$copied" ]] || return 1
  [[ -z "$(find "$source" -mindepth 1 ! -type f -print -quit)" ]] || return 1
  [[ -z "$(find "$copied" -mindepth 1 ! -type f -print -quit)" ]] || return 1
  write_context_file_list "$source" >"$source_list" || return 1
  write_context_file_list "$copied" >"$copied_list" || return 1
  cmp -s "$source_list" "$copied_list" || return 1
  while IFS= read -r -d '' path; do
    [[ -f "$copied/$path" && ! -L "$copied/$path" ]] || return 1
  done <"$source_list"
  write_directory_manifest "$source" "$source_list" "$runtime_dir/evidence-source-sha256.txt" || return 1
  write_directory_manifest "$copied" "$copied_list" "$runtime_dir/evidence-copied-sha256.txt" || return 1
  cmp -s "$runtime_dir/evidence-source-sha256.txt" "$runtime_dir/evidence-copied-sha256.txt"
}

publish_evidence_directory() {
  local directory="$1" staging_name=""
  [[ ! -e "$evidence_dir" && ! -L "$evidence_dir" ]] || return 1
  mkdir -p "$evidence_parent" || return 1
  evidence_publish_lock="$evidence_parent/.session-access-revocation.publish.lock"
  mkdir "$evidence_publish_lock" || {
    evidence_publish_lock=""
    return 1
  }
  if [[ -e "$evidence_dir" || -L "$evidence_dir" ]]; then
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! evidence_publish_tmp="$(mktemp -d "$evidence_parent/.session-access-revocation.publish.XXXXXX")"; then
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
  if ! (cd "$directory" && tar -cf - . 2>>"$runtime_dir/evidence-copy-errors.raw") |
    tar -C "$evidence_publish_tmp" -xf - 2>>"$runtime_dir/evidence-copy-errors.raw"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! validate_evidence_copy "$directory" "$evidence_publish_tmp"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! mv -n -- "$evidence_publish_tmp" "$evidence_dir" 2>>"$runtime_dir/evidence-publish-errors.raw" ||
    [[ -e "$evidence_publish_tmp" ]]; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if [[ -d "$evidence_dir/$staging_name" ]]; then
    rm -r -- "$evidence_dir/$staging_name" || return 1
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  if ! validate_evidence_copy "$directory" "$evidence_dir"; then
    rm -r -- "$evidence_dir"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    return 1
  fi
  evidence_publish_tmp=""
  if ! rmdir "$evidence_publish_lock"; then
    rm -r -- "$evidence_dir"
    evidence_publish_lock=""
    return 1
  fi
  evidence_publish_lock=""
  evidence_published=1
}

publish_sanitization_failure() {
  local fallback="$runtime_dir/sanitization-fallback"
  mkdir "$fallback" || return 1
  chmod 700 "$fallback" || return 1
  printf 'classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n' \
    >"$fallback/acceptance-results.txt" || return 1
  printf 'classification=evidence_scan|result=FAIL|count=1\n' \
    >"$fallback/evidence-leak-scan.txt" || return 1
  write_evidence_hashes "$fallback" || return 1
  chmod 600 "$fallback"/*.txt || return 1
  publish_evidence_directory "$fallback"
}

publish_evidence() {
  local status="$1" mode=failure candidate="$runtime_dir/evidence-candidate"
  [[ "$status" -ne 0 ]] || mode=success
  if ! classification_file_is_safe "$source_results" "$mode" ||
    ! snapshot_file_is_safe "$production_before" ||
    ! snapshot_file_is_safe "$production_after" ||
    ! cmp -s "$production_before" "$production_after"; then
    publish_sanitization_failure || return 1
    return 2
  fi
  mkdir "$candidate" || return 1
  chmod 700 "$candidate" || return 1
  cp "$source_results" "$candidate/acceptance-results.txt" || return 1
  cp "$production_before" "$candidate/production-before.txt" || return 1
  cp "$production_after" "$candidate/production-after.txt" || return 1
  if [[ "$status" -ne 0 ]]; then
    failure_stage_is_safe "$current_stage" || {
      publish_sanitization_failure || return 1
      return 2
    }
    printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' \
      "$current_stage" >"$candidate/failure-status.txt" || return 1
  else
    cp "$runtime_dir/mysql-auto-migrate-false.txt" "$candidate/mysql-auto-migrate-false.txt" || return 1
    cp "$runtime_dir/mysql-auto-migrate-true.txt" "$candidate/mysql-auto-migrate-true.txt" || return 1
    cp "$runtime_dir/backend-tests.txt" "$candidate/backend-tests.txt" || return 1
    printf 'go_vet=pass\n' >"$candidate/go-vet.txt" || return 1
  fi
  if ! scan_evidence_directory "$candidate" "$runtime_dir/evidence-leaks.raw"; then
    rm -r -- "$candidate"
    publish_sanitization_failure || return 1
    return 2
  fi
  printf 'classification=evidence_scan|result=PASS|count=0\n' \
    >"$candidate/evidence-leak-scan.txt" || return 1
  write_evidence_hashes "$candidate" || return 1
  chmod 600 "$candidate"/*.txt || return 1
  publish_evidence_directory "$candidate"
}

on_exit() {
  local status=$? publication_status=0 snapshots_safe=1
  trap - EXIT INT TERM
  set +e
  if [[ "$evidence_eligible" -eq 1 && "$evidence_published" -ne 1 &&
    ! -e "$evidence_dir" && ! -L "$evidence_dir" ]]; then
    if ! snapshot_file_is_safe "$production_before"; then
      snapshot_production "$production_before" >/dev/null 2>&1 || snapshots_safe=0
    fi
    if [[ "$status" -ne 0 ]] || ! snapshot_file_is_safe "$production_after"; then
      snapshot_production "$production_after" >/dev/null 2>&1 || snapshots_safe=0
    fi
    if [[ "$snapshots_safe" -ne 1 ]]; then
      publish_sanitization_failure || publication_status=$?
      [[ "$publication_status" -ne 0 ]] || publication_status=2
    else
      publish_evidence "$status" || publication_status=$?
    fi
    if [[ "$publication_status" -ne 0 ]]; then
      status=1
      success=0
    fi
  fi
  if [[ "$project_may_exist" -eq 1 && "$docker_available" -eq 1 ]] &&
    docker container ls -a \
      --filter "label=com.docker.compose.project=$project_name" -q | grep -q .; then
    if [[ "$status" -ne 0 ]]; then
      "${compose[@]}" ps >"$runtime_dir/isolated-ps.raw" 2>&1 || true
    fi
    "${compose[@]}" stop >"$runtime_dir/isolated-stop.raw" 2>&1 || true
  fi
  if [[ -n "$evidence_publish_tmp" && -d "$evidence_publish_tmp" ]]; then
    rm -r -- "$evidence_publish_tmp"
  fi
  if [[ -n "$evidence_publish_lock" && -d "$evidence_publish_lock" ]]; then
    rmdir "$evidence_publish_lock" || true
  fi
  if [[ -n "$runtime_dir" && -d "$runtime_dir" ]]; then
    rm -r -- "$runtime_dir"
  fi
  if [[ "$success" -eq 1 ]]; then
    echo "resources retained for inspection under Compose project: $project_name"
  fi
  exit "$status"
}
trap on_exit EXIT INT TERM

verify_source_package() {
  local package_dir="${SESSION_REVOCATION_SOURCE_PACKAGE_DIR:-}"
  local authorized_digest="${SESSION_REVOCATION_SOURCE_PACKAGE_MANIFEST_SHA256:-}"
  local actual_digest
  local artifact
  local entry
  local path
  local entry_count=0
  local -a package_artifacts=(
    source-files.z
    source-sha256.txt
    source.tar
    package-sha256.txt
  )

  [[ "$package_dir" == /* && "$package_dir" != "/" &&
    -d "$package_dir" && ! -L "$package_dir" ]] || {
    echo "SESSION_REVOCATION_SOURCE_PACKAGE_DIR must be an absolute regular directory" >&2
    return 1
  }
  [[ "$authorized_digest" =~ ^[0-9a-f]{64}$ ]] || {
    echo "SESSION_REVOCATION_SOURCE_PACKAGE_MANIFEST_SHA256 must be a lowercase SHA-256" >&2
    return 1
  }
  for artifact in "${package_artifacts[@]}"; do
    [[ -f "$package_dir/$artifact" && ! -L "$package_dir/$artifact" ]] || {
      echo "session revocation source package artifact is missing or unsafe: $artifact" >&2
      return 1
    }
  done
  while IFS= read -r -d '' entry; do
    entry="${entry#"$package_dir"/}"
    case "$entry" in
      source-files.z | source-sha256.txt | source.tar | package-sha256.txt) ;;
      *)
        echo "session revocation source package contains unexpected entry: $entry" >&2
        return 1
        ;;
    esac
    entry_count=$((entry_count + 1))
  done < <(find "$package_dir" -mindepth 1 -maxdepth 1 -print0)
  [[ "$entry_count" -eq 4 ]] || {
    echo "session revocation source package must contain exactly four artifacts" >&2
    return 1
  }

  actual_digest="$(sha256sum "$package_dir/package-sha256.txt")"
  actual_digest="${actual_digest%% *}"
  [[ "$actual_digest" == "$authorized_digest" ]] || {
    echo "session revocation source package manifest is not authorized" >&2
    return 1
  }
  (
    cd "$package_dir"
    sha256sum source-files.z source-sha256.txt source.tar
  ) >"$runtime_dir/expected-package-sha256.txt"
  cmp -s "$runtime_dir/expected-package-sha256.txt" \
    "$package_dir/package-sha256.txt" || {
    echo "session revocation source package artifact checksum mismatch" >&2
    return 1
  }

  validate_source_list "$package_dir/source-files.z" \
    "$runtime_dir/sorted-source-files.z" || {
    echo "session revocation source package list is invalid" >&2
    return 1
  }
  : >"$runtime_dir/archive-source-files.z"
  while IFS= read -r path; do
    [[ -n "$path" && "$path" != /* && "$path" != -* &&
      "$path" != ../* && "$path" != */../* && "$path" != */.. &&
      "$path" != ./* && "$path" != */./* && "$path" != *//* &&
      "$path" != *[!A-Za-z0-9._/-]* ]] || {
      echo "session revocation source archive contains an unsafe path" >&2
      return 1
    }
    if [[ "$path" == */ ]]; then
      path="${path%/}"
      source_path_is_forbidden "$path" && return 1
      source_list_contains_child "$package_dir/source-files.z" "$path" || {
        echo "session revocation source archive contains an unexpected directory" >&2
        return 1
      }
    else
      printf '%s\0' "$path" >>"$runtime_dir/archive-source-files.z"
    fi
  done < <(tar -tf "$package_dir/source.tar")
  validate_source_list "$runtime_dir/archive-source-files.z" \
    "$runtime_dir/sorted-archive-source-files.z" || {
    echo "session revocation source archive list is invalid" >&2
    return 1
  }
  cmp -s "$package_dir/source-files.z" "$runtime_dir/archive-source-files.z" || {
    echo "session revocation source archive does not match its source list" >&2
    return 1
  }

  mkdir "$build_context"
  chmod 700 "$build_context"
  tar -C "$build_context" -xf "$package_dir/source.tar"
  validate_received_source_files "$build_context" "$package_dir/source-files.z" || {
    echo "session revocation extracted source contains a missing or unsafe file" >&2
    return 1
  }
  write_context_file_list "$build_context" >"$runtime_dir/extracted-source-files.z"
  cmp -s "$package_dir/source-files.z" "$runtime_dir/extracted-source-files.z" || {
    echo "session revocation extracted source does not match its source list" >&2
    return 1
  }
  write_directory_manifest "$build_context" "$package_dir/source-files.z" \
    "$runtime_dir/extracted-source-sha256.txt"
  cmp -s "$package_dir/source-sha256.txt" \
    "$runtime_dir/extracted-source-sha256.txt" || {
    echo "session revocation extracted source checksum mismatch" >&2
    return 1
  }

  validate_received_source_files "$repo_dir" "$package_dir/source-files.z" || {
    echo "received session revocation source contains a missing or unsafe file" >&2
    return 1
  }
  write_directory_manifest "$repo_dir" "$package_dir/source-files.z" \
    "$runtime_dir/received-source-sha256.txt"
  cmp -s "$package_dir/source-sha256.txt" \
    "$runtime_dir/received-source-sha256.txt" || {
    echo "received session revocation source checksum mismatch" >&2
    return 1
  }

  while IFS= read -r -d '' path; do
    source_count=$((source_count + 1))
  done <"$package_dir/source-files.z"
  source_manifest_sha256="$(sha256sum "$package_dir/source-sha256.txt")"
  source_manifest_sha256="${source_manifest_sha256%% *}"
}

verify_source_package
printf 'classification=source_package|result=PASS|count=%s|sha256=%s\n' \
  "$source_count" "$source_manifest_sha256" >"$source_results"

[[ ! -e "$evidence_dir" && ! -L "$evidence_dir" ]] || {
  echo "refusing to overwrite existing session revocation evidence" >&2
  exit 1
}
[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}
command -v docker >/dev/null || {
  echo "required command is unavailable: docker" >&2
  exit 1
}
docker_available=1

current_stage="resource_collision"
existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q \
  2>>"$runtime_dir/project-collision-errors.raw")"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q \
  2>>"$runtime_dir/project-collision-errors.raw")"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q \
  2>>"$runtime_dir/project-collision-errors.raw")"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}

evidence_eligible=1
current_stage="production_before"
snapshot_production "$production_before" || {
  echo "production-before snapshot failed strict inspection" >&2
  exit 1
}

compose_override="$runtime_dir/session-revocation-compose.yml"
printf 'services:\n  bootstrap-admin:\n    build:\n      context: "%s"\n      dockerfile: backend/Dockerfile\n' \
  "$build_context" >"$compose_override"
compose+=(--file "$compose_override")

mysql_sql() {
  local sql="$1"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="$1"
  ' sh "$sql"
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
    DROP TABLE IF EXISTS file_quota_guards, buyer_intents, buyer_histories,
      buyer_favorites, buyer_device_bindings, buyer_users, idempotency_records,
      auth_sessions, operation_logs, file_records, files, order_events, orders,
      product_images, products, categories, merchant_audit_logs, admin_users,
      merchant_accounts, merchants;
    SET FOREIGN_KEY_CHECKS=1;
  "
}

apply_migration_chain() {
  reset_schema
  mysql_file /acceptance/migrations/0001_init.up.sql
  mysql_file /acceptance/migrations/0002_buyer_domain.up.sql
  mysql_file /acceptance/migrations/0003_buyer_auth_provider.up.sql
  for migration in 0004_merchant_multi_stock 0005_file_records_table \
    0006_file_binding_ownership 0007_license_file_privacy \
    0008_anonymous_upload_governance 0009_buyer_intent_open_uniqueness; do
    mysql_file "/acceptance/migrations/$migration.preflight.sql"
    mysql_file "/acceptance/migrations/$migration.up.sql"
    mysql_file "/acceptance/migrations/$migration.postflight.sql"
  done
}

run_focused_test() {
  local auto_migrate="$1"
  local label="$2"
  local raw="$runtime_dir/$label.raw"
  if ! "${compose[@]}" --profile tools run --rm \
    -e SESSION_REVOCATION_MYSQL_TEST=1 \
    -e AUTO_MIGRATE="$auto_migrate" \
      bootstrap-admin go test ./tests -run '^TestSessionRevocationMySQLAcceptance$' -count=1 -v \
      >"$raw" 2>&1; then
    echo "focused session revocation test failed for AUTO_MIGRATE=$auto_migrate" >&2
    return 1
  fi
  grep -E -- '^(=== RUN|--- PASS:|    --- PASS:|PASS$|ok[[:space:]])|status/code =|EXPLAIN access/key =' "$raw" \
    >"$runtime_dir/$label.txt"
  grep -q -- '--- PASS: TestSessionRevocationMySQLAcceptance' "$runtime_dir/$label.txt" || {
    echo "focused session revocation PASS marker is missing" >&2
    return 1
  }
}

current_stage="mysql_start"
project_may_exist=1
"${compose[@]}" up -d --wait mysql
current_stage="mysql_version"
mysql_version="$(mysql_sql 'SELECT VERSION()')"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated session revocation acceptance requires MySQL 8.4.x" >&2
  exit 1
}
printf 'classification=mysql_version|result=PASS|count=1\n' >>"$source_results"

current_stage="bootstrap_build"
"${compose[@]}" --profile tools build bootstrap-admin

current_stage="migration_chain"
apply_migration_chain
printf 'classification=migration_chain|result=PASS|count=1\n' >>"$source_results"
current_stage="session_auto_migrate_false"
run_focused_test false mysql-auto-migrate-false
printf 'classification=session_auto_migrate_false|result=PASS|count=1\n' >>"$source_results"

current_stage="migration_chain_true_reset"
apply_migration_chain
current_stage="session_auto_migrate_true"
run_focused_test true mysql-auto-migrate-true
printf 'classification=session_auto_migrate_true|result=PASS|count=1\n' >>"$source_results"

current_stage="backend_tests"
backend_raw="$runtime_dir/backend-tests.raw"
if ! "${compose[@]}" --profile tools run --rm \
  -e SESSION_REVOCATION_MYSQL_TEST=0 \
  bootstrap-admin go test ./... -count=1 >"$backend_raw" 2>&1; then
  echo "full backend tests failed" >&2
  exit 1
fi
grep -E -- '^(\?|ok[[:space:]])' "$backend_raw" >"$runtime_dir/backend-tests.txt"
printf 'classification=backend_tests|result=PASS|count=1\n' >>"$source_results"

current_stage="go_vet"
if ! "${compose[@]}" --profile tools run --rm \
  -e SESSION_REVOCATION_MYSQL_TEST=0 \
  bootstrap-admin go vet ./... >"$runtime_dir/go-vet.raw" 2>&1; then
  echo "go vet failed" >&2
  exit 1
fi
printf 'classification=go_vet|result=PASS|count=1\n' >>"$source_results"

current_stage="production_snapshot"
snapshot_production "$production_after" || {
  echo "production-after snapshot failed strict inspection" >&2
  exit 1
}
cmp -s "$production_before" "$production_after" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}
printf 'classification=production_snapshot|result=PASS|count=3\n' >>"$source_results"

current_stage="evidence_publication"
publication_status=0
publish_evidence 0 || publication_status=$?
[[ "$publication_status" -eq 0 ]] || {
  echo "session revocation evidence publication failed closed" >&2
  exit 1
}
success=1
echo "isolated session access revocation acceptance passed"
echo "mysql version: $mysql_version"
