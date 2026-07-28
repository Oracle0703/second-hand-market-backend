#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-upload-governance-acceptance"
retained_evidence_dir="$base_dir/evidence/anonymous-upload-governance"
evidence_parent="${retained_evidence_dir%/*}"
evidence_dir=""
evidence_publish_tmp=""
evidence_publish_lock=""
evidence_parent_identity=""
evidence_parent_cwd_active=0
evidence_parent_return_dir=""
compose=()
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)
runtime_dir=""
evidence_forbidden_pattern='Authorization|Bearer[[:space:]]|access_token|refresh_token|file_token|token["=:]|password["=:]|DB_DSN=|MYSQL_(DATABASE|USER|PASSWORD|ROOT_PASSWORD)=|JWT_(ACCESS|REFRESH)_SECRET=|object_key|source_ip_hash|cleanup_claim_token|/var/lib/second-hand-market/uploads|injected-secret|TestUploadGovernance|000[0-9]_[[:alnum:]_-]+\.(preflight|up|postflight)\.sql|f06-historical-'

required_source_paths=(
  Makefile backend/Dockerfile backend/go.mod backend/go.sum
  backend/internal/app/upload_governance.go
  backend/internal/app/upload_governance_mysql_test.go
  backend/tests/anonymous_upload_governance_acceptance_contract_test.go
  backend/migrations/0008_anonymous_upload_governance.preflight.sql
  backend/migrations/0008_anonymous_upload_governance.up.sql
  backend/migrations/0008_anonymous_upload_governance.postflight.sql
  backend/migrations/anonymous_upload_governance_migration_test.go
  frontend/package.json frontend/package-lock.json frontend/index.html
  frontend/tsconfig.json frontend/vite.config.ts frontend/vitest.config.ts
  frontend/src/utils/upload.ts frontend/src/utils/upload.test.ts
  deploy/acceptance/docker-compose.yml deploy/acceptance/frontend.Dockerfile
  deploy/acceptance/nginx.conf deploy/acceptance/anonymous-upload-governance-smoke.sh
  deploy/acceptance/sql/post-smoke.sql deploy/acceptance/sql/protected-fingerprint.sql
)

source_path_is_forbidden() {
  local path="$1"
  local lower component
  local -a components=()
  lower="$(printf '%s' "$path" | LC_ALL=C tr '[:upper:]' '[:lower:]')"
  case "$lower" in
    *.db|*.db.*|*.sqlite|*.sqlite.*|*.sqlite3|*.sqlite3.*) return 0 ;;
  esac
  IFS=/ read -r -a components <<<"$lower"
  for component in "${components[@]}"; do
    case "$component" in
      .env|.env.*|.git|.tmp|.cache|cache|caches|secret|secrets|database|databases|upload|uploads|evidence|backup|backups|node_modules)
        return 0 ;;
    esac
  done
  return 1
}

source_path_is_allowed() {
  local path="$1"
  case "$path" in
    Makefile|backend/Dockerfile|backend/go.mod|backend/go.sum|backend/*.go|backend/migrations/*.sql|\
    frontend/src/*|frontend/package.json|frontend/package-lock.json|frontend/index.html|\
    frontend/tsconfig.json|frontend/vite.config.ts|frontend/vitest.config.ts|\
    deploy/acceptance/docker-compose.yml|deploy/acceptance/frontend.Dockerfile|\
    deploy/acceptance/nginx.conf|deploy/acceptance/anonymous-upload-governance-smoke.sh|\
    deploy/acceptance/sql/*.sql)
      return 0 ;;
  esac
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
  local commit="${1:-HEAD}"
  (
    cd "$repo_dir"
    git ls-tree -r --name-only -z "$commit" -- Makefile backend frontend deploy/acceptance 2>/dev/null |
      while IFS= read -r -d '' path; do
        source_path_is_portable "$path" || continue
        source_path_is_forbidden "$path" && continue
        source_path_is_allowed "$path" && printf '%s\0' "$path"
      done | LC_ALL=C sort -zu
  )
}

source_list_contains() {
  local source_list="$1" required="$2" path
  while IFS= read -r -d '' path; do
    [[ "$path" == "$required" ]] && return 0
  done <"$source_list"
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
  for required in "${required_source_paths[@]}"; do
    source_list_contains "$source_list" "$required" || return 1
  done
}

write_directory_manifest() {
  local directory="$1" source_list="$2" output="$3"
  (
    cd "$directory"
    xargs -0 sha256sum <"$source_list"
  ) >"$output"
}

write_context_file_list() {
  local directory="$1"
  (
    cd "$directory"
    find . -type f -print0 | while IFS= read -r -d '' path; do
      printf '%s\0' "${path#./}"
    done | LC_ALL=C sort -zu
  )
}

validate_received_source_files() {
  local directory="$1" source_list="$2" path current component
  local -a components=()
  [[ ! -L "$directory" ]] || return 1
  while IFS= read -r -d '' path; do
    current="$directory"
    IFS=/ read -r -a components <<<"$path"
    for component in "${components[@]}"; do
      current="$current/$component"
      [[ ! -L "$current" ]] || return 1
    done
    [[ -f "$current" ]] || return 1
  done <"$source_list"
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

validate_package_checksums() {
  local package_dir="$1" diagnostics="${2:-/dev/null}" expected_name actual_hash line line_count=0
  local -a names=(source-files.z source-sha256.txt source.tar)
  exec 3<"$package_dir/package-sha256.txt"
  while [[ "$line_count" -lt 3 ]]; do
    expected_name="${names[$line_count]}"
    IFS= read -r line <&3 || { exec 3<&-; return 1; }
    actual_hash="$(sha256sum "$package_dir/$expected_name" 2>>"$diagnostics" | cut -d ' ' -f1)"
    [[ "$line" == "$actual_hash  $expected_name" ]] || { exec 3<&-; return 1; }
    line_count=$((line_count + 1))
  done
  if IFS= read -r line <&3; then
    exec 3<&-
    return 1
  fi
  exec 3<&-
}

validate_archive_list() {
  local package_dir="$1" source_list="$2" runtime="$3" line path parent type_line expected_type
  local unsorted_expected="$runtime/expected-archive-paths.unsorted"
  local expected_paths="$runtime/expected-archive-paths.raw"
  : >"$runtime/archive-source-files.z"
  : >"$unsorted_expected"
  while IFS= read -r -d '' path; do
    printf '%s\n' "$path" >>"$unsorted_expected"
    parent="${path%/*}"
    while [[ "$parent" != "$path" && -n "$parent" ]]; do
      printf '%s/\n' "$parent" >>"$unsorted_expected"
      path="$parent"
      parent="${path%/*}"
    done
  done <"$source_list"
  LC_ALL=C sort -u "$unsorted_expected" >"$expected_paths"
  tar -tvf "$package_dir/source.tar" >"$runtime/archive-types.raw" 2>"$runtime/archive-types-errors.raw" || return 1
  tar -tf "$package_dir/source.tar" >"$runtime/archive-paths.raw" 2>"$runtime/archive-paths-errors.raw" || return 1
  exec 3<"$runtime/archive-types.raw"
  while IFS= read -r path; do
    IFS= read -r type_line <&3 || { exec 3<&-; return 1; }
    source_path_is_portable "${path%/}" || return 1
    if [[ "$path" == */ ]]; then
      expected_type=d
    else
      expected_type=-
      printf '%s\0' "$path" >>"$runtime/archive-source-files.z"
    fi
    [[ "${type_line:0:1}" == "$expected_type" ]] || { exec 3<&-; return 1; }
  done <"$runtime/archive-paths.raw"
  if IFS= read -r line <&3; then
    exec 3<&-
    return 1
  fi
  exec 3<&-
  cmp -s "$expected_paths" "$runtime/archive-paths.raw" || return 1
  validate_source_list "$runtime/archive-source-files.z" "$runtime/sorted-archive-source-files.z" || return 1
  cmp -s "$source_list" "$runtime/archive-source-files.z"
}

evidence_parent_is_safe() {
  [[ -d "$evidence_parent" && ! -L "$evidence_parent" ]]
}

prepare_evidence_parent() {
  if [[ -e "$evidence_parent" || -L "$evidence_parent" ]]; then
    evidence_parent_is_safe || return 1
  else
    mkdir "$evidence_parent" 2>>"$runtime_dir/evidence-parent.raw" || return 1
    evidence_parent_is_safe || return 1
  fi
  evidence_parent_identity="$(directory_identity "$evidence_parent")"
}

directory_identity() {
  local path="$1" identity=""
  if identity="$(stat -f '%d:%i' "$path" 2>/dev/null)" &&
    [[ "$identity" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf '%s\n' "$identity"
    return 0
  fi
  if identity="$(stat -c '%d:%i' "$path" 2>/dev/null)" &&
    [[ "$identity" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf '%s\n' "$identity"
    return 0
  fi
  return 1
}

evidence_parent_is_current() {
  local identity=""
  evidence_parent_is_safe || return 1
  [[ -n "$evidence_parent_identity" ]] || return 1
  identity="$(directory_identity "$evidence_parent")" || return 1
  [[ "$identity" == "$evidence_parent_identity" ]]
}

enter_evidence_parent_capability() {
  evidence_parent_is_current || return 1
  evidence_parent_return_dir="$PWD"
  cd "$evidence_parent" || return 1
  if [[ "$(directory_identity .)" != "$evidence_parent_identity" ]]; then
    cd "$evidence_parent_return_dir" || true
    evidence_parent_return_dir=""
    return 1
  fi
  evidence_parent_cwd_active=1
}

leave_evidence_parent_capability() {
  local status="${1:-0}"
  if [[ "$evidence_parent_cwd_active" -eq 1 ]]; then
    cd "$evidence_parent_return_dir" || status=1
  fi
  evidence_parent_cwd_active=0
  evidence_parent_return_dir=""
  return "$status"
}

export_head_source() (
  set -euo pipefail
  local export_dir="$1" export_runtime="" extracted="" completed=0 export_dir_owned=0 export_dir_identity="" path head_oid
  local -a archive_paths=()

  export_dir_is_current() {
    local identity=""
    [[ "$export_dir_owned" -eq 1 && -d "$export_dir" && ! -L "$export_dir" ]] || return 1
    identity="$(directory_identity "$export_dir")" || return 1
    [[ -n "$export_dir_identity" && "$identity" == "$export_dir_identity" ]]
  }

  cleanup_source_export() {
    local status="${1:-$?}"
    trap - EXIT INT TERM
    if [[ -n "$export_runtime" && -d "$export_runtime" ]]; then
      rm -r -- "$export_runtime"
    fi
    if [[ "$completed" -ne 1 ]] && export_dir_is_current; then
      rm -r -- "$export_dir"
    fi
    exit "$status"
  }
  trap cleanup_source_export EXIT
  trap 'cleanup_source_export 130' INT
  trap 'cleanup_source_export 143' TERM

  [[ "$export_dir" == /* && "$export_dir" != / && ! -e "$export_dir" && ! -L "$export_dir" ]] || {
    echo "ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2
    return 1
  }
  for command in git sha256sum sort xargs mktemp tar chmod mkdir rm tr cmp find stat; do
    command -v "$command" >/dev/null || {
      echo "required source export command is unavailable: $command" >&2
      return 1
    }
  done
  export_runtime="$(mktemp -d)"
  extracted="$export_runtime/extracted"
  chmod 700 "$export_runtime"
  head_oid="$(git -C "$repo_dir" rev-parse --verify 'HEAD^{commit}' 2>"$export_runtime/head-resolve.raw")" || {
    echo "cannot resolve committed HEAD" >&2
    return 1
  }
  mkdir "$export_dir" 2>"$export_runtime/export-destination.raw" || {
    echo "source export destination was concurrently acquired" >&2
    return 1
  }
  export_dir_identity="$(directory_identity "$export_dir")" || {
    echo "cannot bind source export destination identity" >&2
    return 1
  }
  export_dir_owned=1
  mkdir "$extracted"
  chmod 700 "$extracted"
  if ! (
    cd "$export_dir" || exit 1
    [[ "$(directory_identity .)" == "$export_dir_identity" ]] || exit 1
    chmod 700 . || exit 1
    write_source_file_list "$head_oid" >source-files.z || exit 1
    validate_source_list source-files.z "$export_runtime/sorted-source-files.z" || exit 1
    while IFS= read -r -d '' path; do archive_paths+=("$path"); done <source-files.z
    [[ "${#archive_paths[@]}" -gt 0 ]] || exit 1
    ( cd "$repo_dir"; git archive --format=tar "$head_oid" -- "${archive_paths[@]}" ) \
      >source.tar 2>"$export_runtime/git-archive.raw" || exit 1
    validate_archive_list . source-files.z "$export_runtime" || exit 1
    tar -C "$extracted" -xf source.tar >"$export_runtime/archive-extract.raw" 2>&1 || exit 1
    validate_received_source_files "$extracted" source-files.z || exit 1
    write_context_file_list "$extracted" >"$export_runtime/archive-files.z" || exit 1
    cmp -s source-files.z "$export_runtime/archive-files.z" || exit 1
    exec 4<source-files.z || exit 1
    ( cd "$extracted"; xargs -0 sha256sum <&4 ) >source-sha256.txt || exit 1
    exec 4<&-
    sha256sum source-files.z source-sha256.txt source.tar >package-sha256.txt || exit 1
    chmod 600 source-files.z source-sha256.txt source.tar package-sha256.txt || exit 1
    validate_package_artifact_list . || exit 1
    validate_package_checksums . "$export_runtime/package-checksums.raw"
  ); then
    echo "committed HEAD source export failed" >&2
    return 1
  fi
  export_dir_is_current || { echo "source export destination identity changed" >&2; return 1; }
  rm -r -- "$export_runtime"
  export_runtime=""
  completed=1
)

preflight_on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  [[ -z "$runtime_dir" || ! -d "$runtime_dir" ]] || rm -r -- "$runtime_dir"
  exit "$status"
}

if [[ "${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_LIST_ONLY:-0}" == 1 &&
  -n "${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR:-}" ]]; then
  echo "choose one anonymous upload governance source mode" >&2
  exit 1
fi
if [[ "${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_LIST_ONLY:-0}" == 1 ]]; then
  write_source_file_list
  exit 0
fi
if [[ -n "${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR:-}" ]]; then
  export_head_source "$ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR"
  exit $?
fi

[[ "${ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA" ]] || {
  echo "isolated anonymous upload governance confirmation is missing" >&2
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
[[ "$project_name" == "secondhand-upload-governance-acceptance" ]] || {
  echo "unexpected upload governance Compose project" >&2
  exit 1
}
for command in sha256sum sort xargs mktemp grep cmp tar chmod mkdir rm find wc tr cut cp mv stat; do
  command -v "$command" >/dev/null || {
    echo "required provenance command is unavailable: $command" >&2
    exit 1
  }
done
source_package_dir="${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_DIR:-}"
[[ "$source_package_dir" == /* && -d "$source_package_dir" && ! -L "$source_package_dir" ]] || {
  echo "ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_DIR must identify the transferred source package" >&2
  exit 1
}
runtime_dir="$(mktemp -d 2>/dev/null)" || { echo "cannot create private upload governance runtime" >&2; exit 1; }
trap preflight_on_exit EXIT
trap 'preflight_on_exit 130' INT
trap 'preflight_on_exit 143' TERM
chmod 700 "$runtime_dir" 2>"$runtime_dir/runtime-mode.raw" || { echo "cannot secure private upload governance runtime" >&2; exit 1; }
source_package_snapshot="$runtime_dir/source-package"
build_context="$runtime_dir/build-context"
mkdir "$source_package_snapshot" "$build_context" 2>"$runtime_dir/runtime-directories.raw" || {
  echo "cannot create private upload governance runtime state" >&2
  exit 1
}
chmod 700 "$source_package_snapshot" "$build_context" 2>>"$runtime_dir/runtime-mode.raw" || {
  echo "cannot secure private upload governance runtime state" >&2
  exit 1
}
validate_package_artifact_list "$source_package_dir" 2>"$runtime_dir/caller-package-list.raw" || {
  echo "transferred anonymous upload governance source package must contain exactly four artifacts" >&2
  exit 1
}
for artifact in source-files.z source-sha256.txt source.tar package-sha256.txt; do
  [[ -f "$source_package_dir/$artifact" && ! -L "$source_package_dir/$artifact" ]] || {
    echo "transferred anonymous upload governance source package is incomplete" >&2
    exit 1
  }
done
cp -- "$source_package_dir/source-files.z" "$source_package_dir/source-sha256.txt" \
  "$source_package_dir/source.tar" "$source_package_dir/package-sha256.txt" \
  "$source_package_snapshot/" >"$runtime_dir/package-snapshot.raw" 2>&1 || {
  echo "cannot snapshot transferred anonymous upload governance source package" >&2
  exit 1
}
chmod 600 "$source_package_snapshot/source-files.z" "$source_package_snapshot/source-sha256.txt" \
  "$source_package_snapshot/source.tar" "$source_package_snapshot/package-sha256.txt" \
  2>>"$runtime_dir/runtime-mode.raw" || { echo "cannot secure upload governance source package snapshot" >&2; exit 1; }
validate_package_artifact_list "$source_package_snapshot" 2>"$runtime_dir/package-snapshot-list.raw" || {
  echo "private anonymous upload governance source package snapshot is invalid" >&2
  exit 1
}
for artifact in source-files.z source-sha256.txt source.tar package-sha256.txt; do
  [[ -f "$source_package_snapshot/$artifact" && ! -L "$source_package_snapshot/$artifact" ]] || {
    echo "private anonymous upload governance source package snapshot is incomplete" >&2
    exit 1
  }
done
authorized_package_manifest_sha256="${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_MANIFEST_SHA256:-}"
actual_package_manifest_sha256="$(sha256sum "$source_package_snapshot/package-sha256.txt" 2>>"$runtime_dir/package-manifest.raw" | cut -d ' ' -f1)"
[[ "${#authorized_package_manifest_sha256}" -eq 64 &&
  "$authorized_package_manifest_sha256" != *[!0-9a-f]* &&
  "$actual_package_manifest_sha256" == "$authorized_package_manifest_sha256" ]] || {
  echo "source package manifest digest does not match authorization" >&2
  exit 1
}
validate_package_checksums "$source_package_snapshot" "$runtime_dir/package-checksums.raw" || {
  echo "transferred anonymous upload governance source package checksum failed" >&2
  exit 1
}
source_files="$source_package_snapshot/source-files.z"
source_manifest="$source_package_snapshot/source-sha256.txt"
validate_source_list "$source_files" "$runtime_dir/sorted-source-files.z" || {
  echo "transferred anonymous upload governance source list is invalid" >&2
  exit 1
}
validate_archive_list "$source_package_snapshot" "$source_files" "$runtime_dir" || {
  echo "source archive list or entry type is unsafe" >&2
  exit 1
}
if ! tar -C "$build_context" -xf "$source_package_snapshot/source.tar" >"$runtime_dir/source-archive-extract.raw" 2>&1; then
  echo "source archive extraction failed" >&2
  exit 1
fi
validate_received_source_files "$build_context" "$source_files" 2>>"$runtime_dir/build-context-validation.raw" || {
  echo "temporary upload governance source contains a missing or unsafe file" >&2
  exit 1
}
write_context_file_list "$build_context" >"$runtime_dir/build-context-files.z"
cmp -s "$source_files" "$runtime_dir/build-context-files.z" || {
  echo "source archive contents do not match the committed source list" >&2
  exit 1
}
write_directory_manifest "$build_context" "$source_files" "$runtime_dir/build-context-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/build-context-sha256.txt" || {
  echo "temporary build context does not match the committed source manifest" >&2
  exit 1
}
validate_received_source_files "$repo_dir" "$source_files" 2>>"$runtime_dir/received-source-validation.raw" || {
  echo "received anonymous upload governance source contains a missing or unsafe file" >&2
  exit 1
}
write_directory_manifest "$repo_dir" "$source_files" "$runtime_dir/received-source-sha256.txt" 2>>"$runtime_dir/received-source-validation.raw"
cmp -s "$source_manifest" "$runtime_dir/received-source-sha256.txt" || {
  echo "received anonymous upload governance source does not match package manifest" >&2
  exit 1
}
source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
source_manifest_sha256="$(sha256sum "$source_manifest" 2>>"$runtime_dir/source-manifest.raw" | cut -d ' ' -f1)"
prepare_evidence_parent || { echo "anonymous upload governance evidence parent is unsafe" >&2; exit 1; }
[[ ! -e "$retained_evidence_dir" && ! -L "$retained_evidence_dir" &&
  ! -e "${retained_evidence_dir}.publish.lock" && ! -L "${retained_evidence_dir}.publish.lock" ]] || {
  echo "refusing to overwrite existing anonymous upload governance evidence" >&2
  exit 1
}
[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}
compose_env_source_identity="$(directory_identity "$base_dir/.env")" || {
  echo "cannot bind acceptance env identity" >&2
  exit 1
}
[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" &&
  "$(directory_identity "$base_dir/.env")" == "$compose_env_source_identity" ]] || {
  echo "acceptance env changed during validation" >&2
  exit 1
}
compose_env_snapshot="$runtime_dir/compose.env"
cp -P -- "$base_dir/.env" "$compose_env_snapshot" 2>"$runtime_dir/compose-env-copy.raw" || {
  echo "cannot snapshot acceptance env" >&2
  exit 1
}
[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" &&
  "$(directory_identity "$base_dir/.env")" == "$compose_env_source_identity" &&
  -f "$compose_env_snapshot" && ! -L "$compose_env_snapshot" ]] || {
  echo "acceptance env changed while being snapshotted" >&2
  exit 1
}
chmod 600 "$compose_env_snapshot" 2>"$runtime_dir/compose-env-mode.raw" || {
  echo "cannot secure acceptance env snapshot" >&2
  exit 1
}
[[ -f "$compose_env_snapshot" && ! -L "$compose_env_snapshot" ]] || {
  echo "acceptance env snapshot is unsafe" >&2
  exit 1
}
compose=(docker compose --project-name "$project_name" --env-file "$compose_env_snapshot" --file "$base_dir/docker-compose.yml")
for command in docker curl jq openssl sha256sum truncate base64 sort xargs mktemp tar chmod mkdir rm tr cmp find wc cut cat; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done
existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q 2>>"$runtime_dir/project-collision.raw")" || { echo "cannot inspect isolated project containers" >&2; exit 1; }
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q 2>>"$runtime_dir/project-collision.raw")" || { echo "cannot inspect isolated project volumes" >&2; exit 1; }
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q 2>>"$runtime_dir/project-collision.raw")" || { echo "cannot inspect isolated project networks" >&2; exit 1; }
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}
if command -v ss >/dev/null && ss -ltnH | awk '{print $4}' | grep -Eq '(^|:)(18081|18082)$'; then
  echo "acceptance loopback port 18081 or 18082 is already in use" >&2
  exit 1
fi

raw_evidence_dir="$runtime_dir/raw-evidence"
mkdir -p "$raw_evidence_dir"
chmod 700 "$raw_evidence_dir"
evidence_dir="$raw_evidence_dir"
success=0
evidence_eligible=0
project_touched=0
current_stage="source_package"
sanitization_failed=0
: >"$evidence_dir/acceptance-results.txt"

hash_evidence_directory() {
  local directory="$1"
  (
    cd "$directory"
    find . -type f ! -name evidence-sha256.txt -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
  ) >"$directory/evidence-sha256.txt"
}

checkpoint_pass_line_is_safe() {
  local index="$1" line="$2"
  case "$index" in
    0) [[ "$line" =~ ^classification=source_package\|result=PASS\|count=[1-9][0-9]*\|sha256=[0-9a-f]{64}$ ]] ;;
    1) [[ "$line" == 'classification=mysql_version|result=PASS|count=1' ]] ;;
    2) [[ "$line" == 'classification=skipped_0007_preflight|result=PASS|count=1' ]] ;;
    3) [[ "$line" == 'classification=dirty_0008_preflights|result=PASS|count=4' ]] ;;
    4) [[ "$line" == 'classification=clean_migration|result=PASS|count=1' ]] ;;
    5) [[ "$line" == 'classification=mysql_auto_migrate_false|result=PASS|count=1' ]] ;;
    6) [[ "$line" == 'classification=mysql_auto_migrate_true|result=PASS|count=1' ]] ;;
    7) [[ "$line" == 'classification=backend_tests|result=PASS|count=1' ]] ;;
    8) [[ "$line" == 'classification=frontend_tests_build|result=PASS|count=1' ]] ;;
    9) [[ "$line" == 'classification=upload_boundaries|result=PASS|count=7' ]] ;;
    10) [[ "$line" == 'classification=historical_rows_files|result=PASS|count=2' ]] ;;
    11) [[ "$line" == 'classification=production_snapshot|result=PASS|count=3' ]] ;;
    *) return 1 ;;
  esac
}

failure_stage_is_safe() {
  case "$1" in
    production_before|mysql_start|mysql_version|build_test_images|skipped_0007_preflight|dirty_0008_preflights|clean_migration|mysql_auto_migrate_false|mysql_auto_migrate_true|backend_tests|frontend_tests_build|upload_boundaries|historical_rows_files|production_after|evidence_scan|evidence_publish) return 0 ;;
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
    [[ "$pass_count" -eq 12 && "$failure_count" -eq 0 ]]
  else
    [[ "$pass_count" -ge 1 && "$pass_count" -lt 12 && "$failure_count" -eq 1 ]]
  fi
}

snapshot_file_is_safe() {
  local file="$1" name id state rest extra count=0
  local -a expected=(/secondhand-market-api /secondhand-market-web /secondhand-market-mysql)
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS='|' read -r name id state rest extra; do
    [[ "$count" -lt 3 && "$name" == "${expected[$count]}" && -z "$extra" ]] || return 1
    if [[ "$id" == absent ]]; then
      [[ "$state" == absent && "$rest" == absent ]] || return 1
    else
      [[ "${#id}" -eq 64 && "$id" != *[!0-9a-f]* && "$state" =~ ^[a-z]+$ && "$rest" =~ ^[0-9]+$ ]] || return 1
    fi
    count=$((count + 1))
  done <"$file"
  [[ "$count" -eq 3 ]]
}

publish_evidence_directory() {
  local directory="$1" staging_name=""
  local evidence_name="./${retained_evidence_dir##*/}"
  local lock_name="${evidence_name}.publish.lock"
  enter_evidence_parent_capability || return 1
  [[ ! -e "$evidence_name" && ! -L "$evidence_name" ]] || {
    leave_evidence_parent_capability 1
    return 1
  }
  evidence_publish_lock="$lock_name"
  mkdir "$evidence_publish_lock" || {
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  }
  if ! evidence_parent_is_current || [[ -e "$evidence_name" || -L "$evidence_name" ]]; then
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if ! evidence_publish_tmp="$(mktemp -d "${evidence_name}.publish.XXXXXX")"; then
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  staging_name="${evidence_publish_tmp##*/}"
  if ! evidence_parent_is_current || ! chmod 700 "$evidence_publish_tmp"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if ! (cd "$directory" && tar -cf - . 2>>"$runtime_dir/evidence-copy-errors.raw") |
    tar -C "$evidence_publish_tmp" -xf - 2>>"$runtime_dir/evidence-copy-errors.raw"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if ! evidence_parent_is_current || ! validate_evidence_staging_copy "$directory" "$evidence_publish_tmp"; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if ! evidence_parent_is_current ||
    ! mv -n -- "$evidence_publish_tmp" "$evidence_name" 2>>"$runtime_dir/evidence-publish-errors.raw" ||
    [[ -e "$evidence_publish_tmp" ]]; then
    rm -r -- "$evidence_publish_tmp"
    evidence_publish_tmp=""
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if ! evidence_parent_is_current; then
    evidence_publish_tmp=""
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if [[ -d "$evidence_name/$staging_name" ]]; then
    evidence_publish_tmp=""
    if ! rm -r -- "$evidence_name/$staging_name"; then
      evidence_publish_lock=""
      leave_evidence_parent_capability 1
      return 1
    fi
    rmdir "$evidence_publish_lock" || true
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  if ! validate_evidence_staging_copy "$directory" "$evidence_name"; then
    evidence_publish_tmp=""
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  evidence_publish_tmp=""
  if ! evidence_parent_is_current || ! rmdir "$evidence_publish_lock"; then
    evidence_publish_lock=""
    leave_evidence_parent_capability 1
    return 1
  fi
  evidence_publish_lock=""
  leave_evidence_parent_capability 0
}

validate_evidence_staging_copy() {
  local source="$1" staged="$2" path
  local source_list="$runtime_dir/evidence-publish-source-files.z"
  local staged_list="$runtime_dir/evidence-publish-staged-files.z"
  [[ -d "$source" && ! -L "$source" && -d "$staged" && ! -L "$staged" ]] || return 1
  [[ -z "$(find "$source" -mindepth 1 ! -type f -print -quit)" ]] || return 1
  [[ -z "$(find "$staged" -mindepth 1 ! -type f -print -quit)" ]] || return 1
  write_context_file_list "$source" >"$source_list"
  write_context_file_list "$staged" >"$staged_list"
  cmp -s "$source_list" "$staged_list" || return 1
  while IFS= read -r -d '' path; do
    [[ -f "$staged/$path" && ! -L "$staged/$path" ]] || return 1
  done <"$source_list"
  write_directory_manifest "$source" "$source_list" "$runtime_dir/evidence-publish-source-sha256.txt"
  write_directory_manifest "$staged" "$staged_list" "$runtime_dir/evidence-publish-staged-sha256.txt"
  cmp -s "$runtime_dir/evidence-publish-source-sha256.txt" "$runtime_dir/evidence-publish-staged-sha256.txt"
}

scan_evidence_directory() {
  local directory="$1" scan_output="$2" scan_status=0
  grep -ERn --binary-files=text "$evidence_forbidden_pattern" "$directory" >"$scan_output" || scan_status=$?
  [[ "$scan_status" -eq 1 ]]
}

publish_sanitization_failure() {
  local fallback_dir="$runtime_dir/safe-sanitization-failure"
  [[ ! -e "$fallback_dir" ]] || rm -r -- "$fallback_dir"
  mkdir -p "$fallback_dir"
  chmod 700 "$fallback_dir"
  printf 'classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n' >"$fallback_dir/acceptance-results.txt"
  printf 'classification=evidence_scan|result=FAIL|count=1\n' >"$fallback_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$fallback_dir"
  publish_evidence_directory "$fallback_dir"
}

retain_failure_evidence() {
  local safe_dir="$runtime_dir/safe-failure-evidence" snapshot
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  [[ "$sanitization_failed" -eq 0 ]] || { publish_sanitization_failure; return; }
  failure_stage_is_safe "$current_stage" || { publish_sanitization_failure; return; }
  cp "$evidence_dir/acceptance-results.txt" "$safe_dir/acceptance-results.txt"
  printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' "$current_stage" >>"$safe_dir/acceptance-results.txt"
  checkpoint_file_is_safe "$safe_dir/acceptance-results.txt" failure || { publish_sanitization_failure; return; }
  for snapshot in production-before.txt production-after.txt; do
    if [[ -e "$evidence_dir/$snapshot" ]]; then
      snapshot_file_is_safe "$evidence_dir/$snapshot" || { publish_sanitization_failure; return; }
      cp "$evidence_dir/$snapshot" "$safe_dir/$snapshot"
    fi
  done
  scan_evidence_directory "$safe_dir" "$runtime_dir/failure-evidence-leaks.raw" || {
    sanitization_failed=1
    publish_sanitization_failure
    return
  }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

publish_success_evidence() {
  local safe_dir="$runtime_dir/safe-success-evidence" snapshot
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  checkpoint_file_is_safe "$evidence_dir/acceptance-results.txt" success || {
    publish_sanitization_failure
    return 1
  }
  cp "$evidence_dir/acceptance-results.txt" "$safe_dir/acceptance-results.txt"
  for snapshot in production-before.txt production-after.txt; do
    snapshot_file_is_safe "$evidence_dir/$snapshot" || { publish_sanitization_failure; return 1; }
    cp "$evidence_dir/$snapshot" "$safe_dir/$snapshot"
  done
  scan_evidence_directory "$safe_dir" "$runtime_dir/success-evidence-leaks.raw" || {
    sanitization_failed=1
    publish_sanitization_failure
    return 1
  }
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

record_pass() {
  local classification="$1" count="$2" sha="${3:-}"
  if [[ "$classification" == source_package ]]; then
    [[ "$count" =~ ^[1-9][0-9]*$ && "$sha" =~ ^[0-9a-f]{64}$ ]] || {
      sanitization_failed=1
      echo "refusing unsafe source evidence classification" >&2
      exit 1
    }
  else
    [[ -z "$sha" ]] || { sanitization_failed=1; echo "refusing unexpected evidence digest" >&2; exit 1; }
    case "$classification|$count" in
      mysql_version\|1|skipped_0007_preflight\|1|dirty_0008_preflights\|4|clean_migration\|1|mysql_auto_migrate_false\|1|mysql_auto_migrate_true\|1|backend_tests\|1|frontend_tests_build\|1|upload_boundaries\|7|historical_rows_files\|2|production_snapshot\|3) ;;
      *) sanitization_failed=1; echo "refusing unsafe evidence classification" >&2; exit 1 ;;
    esac
  fi
  if [[ -n "$sha" ]]; then
    printf 'classification=%s|result=PASS|count=%s|sha256=%s\n' "$classification" "$count" "$sha" >>"$evidence_dir/acceptance-results.txt"
  else
    printf 'classification=%s|result=PASS|count=%s\n' "$classification" "$count" >>"$evidence_dir/acceptance-results.txt"
  fi
}

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 && ! -e "$retained_evidence_dir" ]]; then
    if [[ "$project_touched" -eq 1 && ! -e "$evidence_dir/production-after.txt" ]]; then
      snapshot_production "$evidence_dir/production-after.txt" >/dev/null 2>&1 || true
    fi
  fi
  if [[ "$project_touched" -eq 1 ]]; then
    "${compose[@]}" stop >"$runtime_dir/isolated-stop.raw" 2>&1 || true
  fi
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 && ! -e "$retained_evidence_dir" ]]; then
    retain_failure_evidence || true
  fi
  if [[ "$evidence_parent_cwd_active" -eq 1 &&
    "$(directory_identity . 2>/dev/null)" == "$evidence_parent_identity" ]]; then
    if [[ -n "$evidence_publish_tmp" && -d "$evidence_publish_tmp" ]]; then
      rm -r -- "$evidence_publish_tmp"
    fi
    if [[ -n "$evidence_publish_lock" && -d "$evidence_publish_lock" ]]; then
      rmdir "$evidence_publish_lock" || true
    fi
  fi
  if [[ -n "$runtime_dir" && -d "$runtime_dir" ]]; then
    rm -r -- "$runtime_dir"
  fi
  exit "$status"
}
trap on_exit EXIT
trap 'on_exit 130' INT
trap 'on_exit 143' TERM

snapshot_production() {
  local output="$1" line matches
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

record_pass source_package "$source_count" "$source_manifest_sha256"
evidence_eligible=1
current_stage="production_before"
snapshot_production "$evidence_dir/production-before.txt"
snapshot_file_is_safe "$evidence_dir/production-before.txt" || {
  echo "production-before snapshot failed strict validation" >&2
  exit 1
}

compose_override="$runtime_dir/anonymous-upload-governance-compose.yml"
cat >"$compose_override" <<EOF
services:
  mysql:
    volumes:
      - mysql-data:/var/lib/mysql
      - "$build_context/backend/migrations:/acceptance/migrations:ro"
      - "$build_context/deploy/acceptance/sql:/acceptance/sql:ro"
  api:
    build:
      context: "$build_context"
      dockerfile: backend/Dockerfile
  web:
    build:
      context: "$build_context"
      dockerfile: deploy/acceptance/frontend.Dockerfile
  bootstrap-admin:
    build:
      context: "$build_context"
      dockerfile: backend/Dockerfile
      target: build
  frontend-test:
    build:
      context: "$build_context"
      dockerfile: deploy/acceptance/frontend.Dockerfile
      target: build
EOF
compose+=(--file "$compose_override")

mysql_sql() {
  local sql="$1"
  local diagnostics="${2:-$runtime_dir/mysql-sql.raw}"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="$1"
  ' sh "$sql" 2>>"$diagnostics"
}

mysql_file() {
  local container_path="$1"
  local diagnostics="${2:-$runtime_dir/mysql-file.raw}"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" < "$1"
  ' sh "$container_path" 2>>"$diagnostics"
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

apply_chain_0001_0007() {
  apply_chain_0001_0006
  mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql
  mysql_file /acceptance/migrations/0007_license_file_privacy.up.sql
  mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql
}

expect_skipped_0007_failure() {
  apply_chain_0001_0006
  mysql_sql "
    INSERT INTO file_records
      (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status,created_at)
    VALUES
      (860003,'MERCHANT_LICENSE','merchant_license/f06-skipped-0007.png',
       '/uploads/merchant_license/f06-skipped-0007.png','image/png',25,
       'PUBLIC','PENDING',CURRENT_TIMESTAMP(3) - INTERVAL 2 DAY);
  "
  mysql_sql "
    SELECT CONCAT('id=',id,'|digest=',SHA2(CONCAT_WS('#',id,biz_type,object_key,url,
      mime_type,size_bytes,uploader_type,scan_status,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%f')),256))
    FROM file_records WHERE id=860003;
  " >"$evidence_dir/skipped-0007-before.txt"
  if mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql \
    "$evidence_dir/skipped-0007-error.txt" >"$runtime_dir/skipped-0007-preflight-stdout.raw"; then
    echo "expected 0008 preflight to reject a skipped 0007 migration" >&2
    exit 1
  fi
  grep -Eq -- 'ERROR 1644 \(45000\)' "$evidence_dir/skipped-0007-error.txt" || {
    echo "skipped 0007 did not fail 0008 preflight with SQLSTATE 45000" >&2
    exit 1
  }
  grep -Fq -- 'upload governance preflight: 0007 merchant license URL remains public' \
    "$evidence_dir/skipped-0007-error.txt" || {
    echo "skipped 0007 failed 0008 preflight for an unexpected reason" >&2
    exit 1
  }
  mysql_sql "
    SELECT CONCAT('id=',id,'|digest=',SHA2(CONCAT_WS('#',id,biz_type,object_key,url,
      mime_type,size_bytes,uploader_type,scan_status,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%f')),256))
    FROM file_records WHERE id=860003;
  " >"$evidence_dir/skipped-0007-after.txt"
  cmp -s "$evidence_dir/skipped-0007-before.txt" "$evidence_dir/skipped-0007-after.txt" || {
    echo "skipped-0007 preflight changed its historical fixture" >&2
    exit 1
  }
}

run_0008() {
  mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql
  mysql_file /acceptance/migrations/0008_anonymous_upload_governance.up.sql
  mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql
}

seed_historical_rows() {
  mysql_sql "
    INSERT INTO file_records
      (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status,owner_merchant_id,created_at)
    VALUES
      (860001,'MERCHANT_LICENSE','merchant_license/f06-historical-license.png','',
       'image/png',23,'PUBLIC',NULL,'PENDING',NULL,CURRENT_TIMESTAMP(3) - INTERVAL 2 DAY),
      (860002,'PRODUCT_IMAGE','product_image/f06-historical-product.png',
       '/uploads/product_image/f06-historical-product.png','image/png',24,
       'MERCHANT',NULL,'PASS',NULL,CURRENT_TIMESTAMP(3) - INTERVAL 2 DAY);
  "
}

write_historical_files() {
  "${compose[@]}" --profile tools run --rm --no-deps --entrypoint /bin/sh bootstrap-admin -ec '
    install -d -m 700 /var/lib/second-hand-market/uploads/merchant_license \
      /var/lib/second-hand-market/uploads/product_image
    printf %s isolated-historical-license > /var/lib/second-hand-market/uploads/merchant_license/f06-historical-license.png
    printf %s isolated-historical-product > /var/lib/second-hand-market/uploads/product_image/f06-historical-product.png
  ' >/dev/null 2>>"$runtime_dir/historical-files-compose.raw"
}

capture_historical_rows() {
  mysql_sql "
    SELECT CONCAT(
      'ids=', GROUP_CONCAT(id ORDER BY id),
      '|rows=', COUNT(*),
      '|digest=', SHA2(GROUP_CONCAT(CONCAT_WS('#',id,biz_type,object_key,url,mime_type,
        size_bytes,uploader_type,COALESCE(uploader_id,'NULL'),scan_status,
        COALESCE(owner_merchant_id,'NULL'),DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%f'))
        ORDER BY id SEPARATOR '|'),256))
    FROM file_records WHERE id IN (860001,860002);
  "
}

capture_historical_files() {
  "${compose[@]}" --profile tools run --rm --no-deps --entrypoint /bin/sh bootstrap-admin -ec '
    printf "license=%s\n" "$(sha256sum /var/lib/second-hand-market/uploads/merchant_license/f06-historical-license.png | cut -d " " -f 1)"
    printf "product=%s\n" "$(sha256sum /var/lib/second-hand-market/uploads/product_image/f06-historical-product.png | cut -d " " -f 1)"
  ' 2>>"$runtime_dir/historical-files-compose.raw"
}

setup_partial_0008() {
  mysql_sql 'ALTER TABLE file_records ADD COLUMN source_ip_hash CHAR(64) NULL;'
}

setup_drifted_0008() {
  run_0008 >/dev/null
  mysql_sql 'ALTER TABLE file_records MODIFY COLUMN source_ip_hash VARCHAR(64) NULL;'
}

setup_missing_guard_0008() {
  run_0008 >/dev/null
  mysql_sql 'DELETE FROM file_quota_guards WHERE id=1;'
}

setup_drifted_guard_engine() {
  run_0008 >/dev/null
  mysql_sql 'ALTER TABLE file_quota_guards ENGINE=MyISAM;'
}

expect_0008_preflight_failure() {
  local name="$1"
  local setup_function="$2"
  local expected_message="$3"
  apply_chain_0001_0007
  seed_historical_rows
  write_historical_files
  "$setup_function"
  capture_historical_rows >"$evidence_dir/$name-historical-before.txt"
  capture_historical_files >"$evidence_dir/$name-files-before.txt"
  if mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql \
    "$evidence_dir/$name-error.txt" >"$runtime_dir/$name-error-stdout.raw"; then
    echo "expected upload governance preflight failure for $name" >&2
    exit 1
  fi
  grep -Eq -- 'ERROR 1644 \(45000\)' "$evidence_dir/$name-error.txt" || {
    echo "upload governance preflight $name did not fail with SQLSTATE 45000" >&2
    exit 1
  }
  grep -Fq -- "$expected_message" "$evidence_dir/$name-error.txt" || {
    echo "upload governance preflight $name failed for an unexpected reason" >&2
    exit 1
  }
  capture_historical_rows >"$evidence_dir/$name-historical-after.txt"
  capture_historical_files >"$evidence_dir/$name-files-after.txt"
  cmp -s "$evidence_dir/$name-historical-before.txt" "$evidence_dir/$name-historical-after.txt" || {
    echo "dirty-state preflight $name changed historical rows" >&2
    exit 1
  }
  cmp -s "$evidence_dir/$name-files-before.txt" "$evidence_dir/$name-files-after.txt" || {
    echo "dirty-state preflight $name changed historical files" >&2
    exit 1
  }
}

current_stage="mysql_start"
project_touched=1
"${compose[@]}" up -d --wait mysql >"$runtime_dir/mysql-start.raw" 2>&1
current_stage="mysql_version"
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" >"$runtime_dir/mysql-version.raw"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated anonymous upload governance acceptance requires MySQL 8.4.x" >&2
  exit 1
}
record_pass mysql_version 1

current_stage="build_test_images"
"${compose[@]}" --profile tools build bootstrap-admin frontend-test >"$runtime_dir/build-test-images.raw" 2>&1

current_stage="skipped_0007_preflight"
expect_skipped_0007_failure >"$runtime_dir/skipped-0007-preflight.raw" 2>&1
record_pass skipped_0007_preflight 1
current_stage="dirty_0008_preflights"
expect_0008_preflight_failure partial-column setup_partial_0008 \
  'upload governance preflight: partial 0008 schema exists' >"$runtime_dir/dirty-0008-partial-column.raw" 2>&1
expect_0008_preflight_failure drifted-column setup_drifted_0008 \
  'upload governance preflight: 0008 columns are drifted' >"$runtime_dir/dirty-0008-drifted-column.raw" 2>&1
expect_0008_preflight_failure missing-guard-row setup_missing_guard_0008 \
  'upload governance preflight: fixed quota guard row is missing or drifted' >"$runtime_dir/dirty-0008-missing-guard-row.raw" 2>&1
expect_0008_preflight_failure drifted-guard-engine setup_drifted_guard_engine \
  'upload governance preflight: quota guard table must use InnoDB' >"$runtime_dir/dirty-0008-drifted-guard-engine.raw" 2>&1
record_pass dirty_0008_preflights 4

current_stage="clean_migration"
apply_chain_0001_0007 >"$runtime_dir/clean-chain-setup.raw" 2>&1
seed_historical_rows >"$runtime_dir/seed-historical-rows.raw" 2>&1
write_historical_files >"$runtime_dir/write-historical-files.raw" 2>&1
capture_historical_rows >"$evidence_dir/historical-before.txt"
capture_historical_files >"$evidence_dir/historical-files-before.txt"
run_0008 >"$runtime_dir/clean-migration.raw" 2>&1
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql >"$runtime_dir/0009-preflight.raw" 2>&1
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql >"$runtime_dir/0009-up.raw" 2>&1
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql >"$runtime_dir/0009-postflight.raw" 2>&1
capture_historical_rows >"$evidence_dir/historical-after.txt"
capture_historical_files >"$evidence_dir/historical-files-after.txt"
cmp -s "$evidence_dir/historical-before.txt" "$evidence_dir/historical-after.txt" || {
  echo "clean 0008 migration changed historical rows" >&2
  exit 1
}
cmp -s "$evidence_dir/historical-files-before.txt" "$evidence_dir/historical-files-after.txt" || {
  echo "clean 0008 migration changed historical files" >&2
  exit 1
}
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE id IN (860001,860002) AND (source_ip_hash IS NOT NULL OR cleanup_after IS NOT NULL OR cleanup_claimed_at IS NOT NULL OR cleanup_claim_token IS NOT NULL)" "$runtime_dir/historical-row-enrollment.raw")" == "0" ]] || {
  echo "clean 0008 migration enrolled historical rows" >&2
  exit 1
}
grep -q anonymous_upload_governance_preflight_passed "$runtime_dir/clean-migration.raw"
grep -q anonymous_upload_governance_migration_applied "$runtime_dir/clean-migration.raw"
grep -q anonymous_upload_governance_postflight_passed "$runtime_dir/clean-migration.raw"
record_pass clean_migration 1

current_stage="mysql_auto_migrate_false"
"${compose[@]}" --profile tools run --rm \
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=false \
  bootstrap-admin go test ./internal/app -run '^TestUploadGovernanceMySQLConcurrencyAndCleanup$' -count=1 -v \
  >"$runtime_dir/mysql-auto-migrate-false.raw" 2>&1
grep -q -- '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' "$runtime_dir/mysql-auto-migrate-false.raw"
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql \
  >"$runtime_dir/post-mysql-auto-migrate-false.raw" 2>&1
record_pass mysql_auto_migrate_false 1

current_stage="mysql_auto_migrate_true"
"${compose[@]}" --profile tools run --rm \
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=true \
  bootstrap-admin go test ./internal/app -run '^TestUploadGovernanceMySQLConcurrencyAndCleanup$' -count=1 -v \
  >"$runtime_dir/mysql-auto-migrate-true.raw" 2>&1
grep -q -- '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' "$runtime_dir/mysql-auto-migrate-true.raw"
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql \
  >"$runtime_dir/post-mysql-auto-migrate-true.raw" 2>&1
record_pass mysql_auto_migrate_true 1

current_stage="backend_tests"
"${compose[@]}" --profile tools run --rm \
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=0 \
  bootstrap-admin go test ./... -count=1 \
  >"$runtime_dir/backend-tests.raw" 2>&1
record_pass backend_tests 1
current_stage="frontend_tests_build"
"${compose[@]}" --profile tools run --rm frontend-test \
  >"$runtime_dir/frontend-tests-build.raw" 2>&1
record_pass frontend_tests_build 1

"${compose[@]}" --profile tools run --rm bootstrap-admin \
  >"$runtime_dir/bootstrap-admin.raw" 2>&1
"${compose[@]}" up -d --wait api web >"$runtime_dir/api-web-start.raw" 2>&1

current_stage="upload_boundaries"
png_file="$runtime_dir/exact-10-mib.png"
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=' \
  | base64 -d >"$png_file"
truncate -s 10485760 "$png_file"

presign_exact="$runtime_dir/presign-exact.json"
presign_exact_status="$(curl --silent --show-error --output "$presign_exact" --write-out '%{http_code}' \
  --header 'Content-Type: application/json' \
  --data '{"biz_type":"MERCHANT_LICENSE","file_name":"exact-10-mib.png","file_size":10485760,"mime_type":"image/png"}' \
  http://127.0.0.1:18082/api/v1/files/presign 2>"$runtime_dir/presign-exact-errors.raw")"
[[ "$presign_exact_status" == "200" ]] && jq -e '.code == 0' "$presign_exact" >/dev/null || {
  echo "exact 10 MiB presign was not accepted" >&2
  exit 1
}
file_id="$(jq -er '.data.file_id' "$presign_exact")"
object_key="$(jq -er '.data.object_key' "$presign_exact")"
file_token="$(jq -er '.data.file_token' "$presign_exact")"
upload_exact="$runtime_dir/upload-exact.json"
upload_exact_status="$(curl --silent --show-error --output "$upload_exact" --write-out '%{http_code}' \
  --form "file_id=$file_id" --form "object_key=$object_key" --form "file_token=$file_token" \
  --form "file=@$png_file;type=image/png" \
  http://127.0.0.1:18082/api/v1/files/upload 2>"$runtime_dir/upload-exact-errors.raw")"
[[ "$upload_exact_status" == "200" ]] && jq -e '.code == 0' "$upload_exact" >/dev/null || {
  echo "exact 10 MiB upload was not accepted" >&2
  exit 1
}
unset file_id object_key file_token

presign_over="$runtime_dir/presign-over.json"
presign_over_status="$(curl --silent --show-error --output "$presign_over" --write-out '%{http_code}' \
  --header 'Content-Type: application/json' \
  --data '{"biz_type":"MERCHANT_LICENSE","file_name":"over-10-mib.png","file_size":10485761,"mime_type":"image/png"}' \
  http://127.0.0.1:18082/api/v1/files/presign 2>"$runtime_dir/presign-over-errors.raw")"
[[ "$presign_over_status" == "400" ]] && jq -e '.code == 10008' "$presign_over" >/dev/null || {
  echo "10 MiB plus one byte presign did not fail with code 10008" >&2
  exit 1
}

multipart_boundary='f06-upload-governance-boundary'
make_multipart_body() {
  local target_bytes="$1"
  local output="$2"
  printf -- '--%s\r\nContent-Disposition: form-data; name="padding"\r\n\r\n' "$multipart_boundary" >"$output"
  local footer_size current_size payload_end
  footer_size="$(printf '\r\n--%s--\r\n' "$multipart_boundary" | wc -c | tr -d ' ')"
  current_size="$(wc -c <"$output" | tr -d ' ')"
  payload_end=$((target_bytes - footer_size))
  [[ "$payload_end" -gt "$current_size" ]] || return 1
  truncate -s "$payload_end" "$output"
  printf '\r\n--%s--\r\n' "$multipart_boundary" >>"$output"
  [[ "$(wc -c <"$output" | tr -d ' ')" == "$target_bytes" ]]
}

body_exact="$runtime_dir/multipart-exact-11-mib.bin"
body_over="$runtime_dir/multipart-over-11-mib.bin"
make_multipart_body 11534336 "$body_exact"
make_multipart_body 11534337 "$body_over"

exercise_request_boundary() {
  local name="$1"
  local url="$2"
  local body="$3"
  local expected_status="$4"
  local expected_code="$5"
  local response="$runtime_dir/$name.json"
  local status
  status="$(curl --silent --show-error --output "$response" --write-out '%{http_code}' \
    --header "Content-Type: multipart/form-data; boundary=$multipart_boundary" \
    --data-binary "@$body" "$url" 2>"$runtime_dir/$name-errors.raw")"
  [[ "$status" == "$expected_status" ]] && jq -e ".code == $expected_code and (.request_id | length > 0)" "$response" >/dev/null || {
    echo "$name request boundary failed" >&2
    exit 1
  }
  printf '%s_status=%s code=%s request_id=present\n' "$name" "$status" "$expected_code"
}

{
  printf 'presign_exact_bytes=10485760 status=%s code=0\n' "$presign_exact_status"
  printf 'upload_exact_bytes=10485760 status=%s code=0\n' "$upload_exact_status"
  printf 'presign_over_bytes=10485761 status=%s code=10008\n' "$presign_over_status"
  exercise_request_boundary direct_exact_11_mib http://127.0.0.1:18082/api/v1/files/upload "$body_exact" 400 10001
  exercise_request_boundary direct_over_11_mib http://127.0.0.1:18082/api/v1/files/upload "$body_over" 413 10008
  exercise_request_boundary proxy_exact_11_mib http://127.0.0.1:18081/api/v1/files/upload "$body_exact" 400 10001
  exercise_request_boundary proxy_over_11_mib http://127.0.0.1:18081/api/v1/files/upload "$body_over" 413 10008
} >"$evidence_dir/upload-boundaries.txt"
record_pass upload_boundaries 7

current_stage="historical_rows_files"
capture_historical_rows >"$evidence_dir/historical-final.txt"
capture_historical_files >"$evidence_dir/historical-files-final.txt"
cmp -s "$evidence_dir/historical-before.txt" "$evidence_dir/historical-final.txt" || {
  echo "API and concurrency acceptance changed historical rows" >&2
  exit 1
}
cmp -s "$evidence_dir/historical-files-before.txt" "$evidence_dir/historical-files-final.txt" || {
  echo "API and concurrency acceptance changed historical files" >&2
  exit 1
}
record_pass historical_rows_files 2

current_stage="production_after"
snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}
snapshot_file_is_safe "$evidence_dir/production-after.txt" || {
  echo "production-after snapshot failed strict validation" >&2
  exit 1
}
record_pass production_snapshot 3

current_stage="evidence_publish"
publish_success_evidence

success=1
echo "isolated anonymous upload governance acceptance passed"
