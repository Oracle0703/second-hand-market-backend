#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-idempotency-acceptance"
retained_evidence_dir="$base_dir/evidence/idempotency-atomicity"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(
  secondhand-market-api
  secondhand-market-web
  secondhand-market-mysql
)
runtime_dir=""
evidence_dir=""
project_touched=0
success=0
evidence_eligible=0
sanitization_failed=0
current_stage="preflight"
evidence_forbidden_pattern='Authorization|Bearer[[:space:]]|access_token|refresh_token|token["=:]|password["=:]|DB_DSN=|MYSQL_(DATABASE|USER|PASSWORD|ROOT_PASSWORD)=|JWT_(ACCESS|REFRESH)_SECRET=|Idempotency-Key|idem_key["=:]|request_hash["=:]|response_raw["=:]|openid["=:]|buyer_id["=:]|merchant_id["=:]|operator_id["=:]|actor_id["=:]|session_id["=:]|intent_no["=:]|contact_(phone|wechat|name)["=:]|TestIdempotency|f15-[[:alnum:]_-]+'

required_source_paths=(
  Makefile
  backend/Dockerfile
  backend/go.mod
  backend/go.sum
  backend/internal/app/idempotency.go
  backend/tests/idempotency_acceptance_contract_test.go
  backend/tests/idempotency_mysql_test.go
  backend/migrations/0001_init.up.sql
  backend/migrations/0009_buyer_intent_open_uniqueness.up.sql
  deploy/acceptance/README.md
  deploy/acceptance/docker-compose.yml
  deploy/acceptance/idempotency-atomicity-smoke.sh
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

write_directory_manifest() {
  local directory="$1"
  local source_list="$2"
  local output="$3"
  (
    cd "$directory"
    xargs -0 sha256sum <"$source_list"
  ) >"$output"
}

export_head_source() {
  local export_dir="$1"
  local export_runtime
  local extracted
  local path
  local -a archive_paths=()

  [[ "$export_dir" == /* && "$export_dir" != "/" && ! -e "$export_dir" ]] || {
    echo "IDEMPOTENCY_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2
    return 1
  }
  for command in git sha256sum sort mktemp tar chmod mkdir rm tr cmp; do
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
    echo "committed HEAD source list is invalid" >&2
    rm -r -- "$export_runtime"
    return 1
  }
  while IFS= read -r -d '' path; do
    archive_paths+=("$path")
  done <"$export_dir/source-files.z"
  [[ "${#archive_paths[@]}" -gt 0 ]] || {
    echo "committed HEAD source whitelist is empty" >&2
    rm -r -- "$export_runtime"
    return 1
  }
  (
    cd "$repo_dir"
    git archive --format=tar --output="$export_dir/source.tar" HEAD -- "${archive_paths[@]}"
  )
  tar -C "$extracted" -xf "$export_dir/source.tar"
  validate_received_source_files "$extracted" "$export_dir/source-files.z" || {
    echo "committed HEAD archive contains a missing or unsafe file" >&2
    rm -r -- "$export_runtime"
    return 1
  }
  write_context_file_list "$extracted" >"$export_runtime/archive-source-files.z"
  cmp -s "$export_dir/source-files.z" "$export_runtime/archive-source-files.z" || {
    echo "committed HEAD archive does not match its source list" >&2
    rm -r -- "$export_runtime"
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
  rm -r -- "$export_runtime"
}

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

  LC_ALL=C sort -zu "$source_list" >"$sorted_list"
  cmp -s "$source_list" "$sorted_list" || return 1
  while IFS= read -r -d '' path; do
    [[ -n "$path" && "$path" != /* && "$path" != ../* && "$path" != */../* ]] || return 1
    source_path_is_forbidden "$path" && return 1
    source_path_is_allowed "$path" || return 1
    count=$((count + 1))
  done <"$source_list"
  [[ "$count" -gt 0 ]] || return 1
  for required in "${required_source_paths[@]}"; do
    source_list_contains "$source_list" "$required" || return 1
  done
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

preflight_on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ -n "$runtime_dir" && -d "$runtime_dir" ]]; then
    rm -r -- "$runtime_dir"
  fi
  exit "$status"
}

if [[ "${IDEMPOTENCY_SOURCE_LIST_ONLY:-0}" == "1" &&
  -n "${IDEMPOTENCY_SOURCE_EXPORT_DIR:-}" ]]; then
  echo "choose one idempotency source mode" >&2
  exit 1
fi
if [[ "${IDEMPOTENCY_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
  exit 0
fi
if [[ -n "${IDEMPOTENCY_SOURCE_EXPORT_DIR:-}" ]]; then
  export_head_source "$IDEMPOTENCY_SOURCE_EXPORT_DIR"
  exit 0
fi

[[ "${IDEMPOTENCY_ACCEPTANCE_CONFIRM:-}" == \
  "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA" ]] || {
  echo "set IDEMPOTENCY_ACCEPTANCE_CONFIRM for isolated idempotency tests" >&2
  exit 1
}
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || {
  echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2
  exit 1
}
[[ "${COMPOSE_PROJECT_NAME:-}" == "$project_name" ]] || {
  echo "COMPOSE_PROJECT_NAME must be $project_name" >&2
  exit 1
}
[[ "$project_name" == "secondhand-idempotency-acceptance" ]] || {
  echo "unexpected idempotency Compose project" >&2
  exit 1
}
for command in docker sha256sum sort xargs mktemp grep cmp tar chmod \
  mkdir rm wc tr cut cat find cp id; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done
host_uid="$(id -u)"
host_gid="$(id -g)"
[[ -n "$host_uid" && "$host_uid" != *[!0-9]* &&
  -n "$host_gid" && "$host_gid" != *[!0-9]* ]] || {
  echo "failed to determine host uid and gid for metadata initialization" >&2
  exit 1
}

source_package_dir="${IDEMPOTENCY_SOURCE_PACKAGE_DIR:-$repo_dir/.idempotency-source}"
[[ "$source_package_dir" == /* && -d "$source_package_dir" && ! -L "$source_package_dir" ]] || {
  echo "IDEMPOTENCY_SOURCE_PACKAGE_DIR must identify the transferred source package" >&2
  exit 1
}
for artifact in source-files.z source-sha256.txt source.tar package-sha256.txt; do
  [[ -f "$source_package_dir/$artifact" && ! -L "$source_package_dir/$artifact" ]] || {
    echo "transferred idempotency source package is incomplete" >&2
    exit 1
  }
done
authorized_package_manifest_sha256="${IDEMPOTENCY_SOURCE_PACKAGE_MANIFEST_SHA256:-}"
actual_package_manifest_sha256="$(sha256sum "$source_package_dir/package-sha256.txt" | cut -d ' ' -f1)"
[[ "${#authorized_package_manifest_sha256}" -eq 64 &&
  "$authorized_package_manifest_sha256" != *[!0-9a-f]* &&
  "$actual_package_manifest_sha256" == "$authorized_package_manifest_sha256" ]] || {
  echo "source package manifest digest does not match authorization" >&2
  exit 1
}
validate_package_checksums "$source_package_dir" || {
  echo "transferred idempotency source package checksum failed" >&2
  exit 1
}

runtime_dir="$(mktemp -d)"
trap preflight_on_exit EXIT
trap 'preflight_on_exit 130' INT
trap 'preflight_on_exit 143' TERM
evidence_dir="$runtime_dir/evidence"
build_context="$runtime_dir/build-context"
mkdir -p "$evidence_dir" "$build_context"
chmod 700 "$runtime_dir" "$evidence_dir" "$build_context"
source_files="$source_package_dir/source-files.z"
source_manifest="$source_package_dir/source-sha256.txt"
validate_source_list "$source_files" "$runtime_dir/sorted-source-files.z" || {
  echo "transferred idempotency source list is invalid" >&2
  exit 1
}
validate_received_source_files "$repo_dir" "$source_files" || {
  echo "received idempotency source contains a missing or unsafe file" >&2
  exit 1
}
write_directory_manifest "$repo_dir" "$source_files" "$runtime_dir/received-source-sha256.txt" || {
  echo "received idempotency source does not match package manifest" >&2
  exit 1
}
cmp -s "$source_manifest" "$runtime_dir/received-source-sha256.txt" || {
  echo "received idempotency source does not match package manifest" >&2
  exit 1
}
tar -C "$build_context" -xf "$source_package_dir/source.tar"
write_context_file_list "$build_context" >"$runtime_dir/build-context-files.z"
cmp -s "$source_files" "$runtime_dir/build-context-files.z" || {
  echo "source archive contents do not match the committed source list" >&2
  exit 1
}
write_directory_manifest "$build_context" "$source_files" \
  "$runtime_dir/build-context-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/build-context-sha256.txt" || {
  echo "temporary build context does not match the committed source manifest" >&2
  exit 1
}

source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
manifest_sha256="$(sha256sum "$source_manifest" | cut -d ' ' -f1)"

[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" ]] || {
  echo "generate the isolated acceptance .env with deploy/acceptance/prepare.sh" >&2
  exit 1
}
grep -qx 'MYSQL_DATABASE=second_hand_market_acceptance' "$base_dir/.env" || {
  echo "isolated acceptance .env must select second_hand_market_acceptance" >&2
  exit 1
}
[[ ! -e "$retained_evidence_dir" ]] || {
  echo "refusing to overwrite existing idempotency evidence" >&2
  exit 1
}

existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}

checkpoint_file_is_safe() {
  local file="$1"
  local line
  local count=0
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS= read -r line; do
    printf '%s\n' "$line" |
      grep -Eq '^classification=[a-z0-9_]+\|result=PASS\|count=[0-9]+(\|sha256=[0-9a-f]{64})?$' || return 1
    count=$((count + 1))
  done <"$file"
  [[ "$count" -gt 0 ]]
}

snapshot_file_is_safe() {
  local file="$1"
  local name
  local id
  local state
  local restart_count
  local extra
  local count=0
  local -a expected_names=(
    /secondhand-market-api
    /secondhand-market-web
    /secondhand-market-mysql
  )
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS='|' read -r name id state restart_count extra; do
    [[ "$count" -lt "${#expected_names[@]}" &&
      "$name" == "${expected_names[$count]}" && -z "$extra" ]] || return 1
    if [[ "$id" == "absent" ]]; then
      [[ "$state" == "absent" && "$restart_count" == "absent" ]] || return 1
    else
      [[ "${#id}" -eq 64 && "$id" != *[!0-9a-f]* &&
        -n "$state" && "$state" != *[!a-z]* &&
        -n "$restart_count" && "$restart_count" != *[!0-9]* ]] || return 1
    fi
    count=$((count + 1))
  done <"$file"
  [[ "$count" -eq "${#expected_names[@]}" ]]
}

scan_evidence_directory() {
  local directory="$1"
  local scan_output="$2"
  local scan_status=0
  grep -ERn --binary-files=text "$evidence_forbidden_pattern" \
    "$directory" >"$scan_output" || scan_status=$?
  [[ "$scan_status" -eq 1 ]]
}

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
  mkdir -p "$retained_evidence_dir"
  chmod 700 "$retained_evidence_dir"
  (
    cd "$directory"
    tar -cf - .
  ) | tar -C "$retained_evidence_dir" -xf -
}

publish_sanitization_failure() {
  local fallback_dir="$runtime_dir/safe-sanitization-failure"
  mkdir -p "$fallback_dir"
  chmod 700 "$fallback_dir"
  printf 'classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n' \
    >"$fallback_dir/failure-status.txt"
  printf 'classification=evidence_scan|result=FAIL|count=1\n' \
    >"$fallback_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$fallback_dir"
  publish_evidence_directory "$fallback_dir"
}

retain_failure_evidence() {
  local safe_dir="$runtime_dir/safe-failure-evidence"
  local snapshot
  [[ "$sanitization_failed" -eq 0 ]] || {
    publish_sanitization_failure
    return
  }
  [[ "$current_stage" != *[!a-z0-9_]* ]] || {
    publish_sanitization_failure
    return
  }
  mkdir -p "$safe_dir"
  chmod 700 "$safe_dir"
  printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' \
    "$current_stage" >"$safe_dir/failure-status.txt"
  checkpoint_file_is_safe "$evidence_dir/acceptance-results.txt" || {
    publish_sanitization_failure
    return
  }
  cp "$evidence_dir/acceptance-results.txt" "$safe_dir/acceptance-results.txt"
  for snapshot in production-before.txt production-after.txt; do
    if [[ -e "$evidence_dir/$snapshot" ]]; then
      snapshot_file_is_safe "$evidence_dir/$snapshot" || {
        publish_sanitization_failure
        return
      }
      cp "$evidence_dir/$snapshot" "$safe_dir/$snapshot"
    fi
  done
  scan_evidence_directory "$safe_dir" "$runtime_dir/safe-failure-leaks.txt" || {
    publish_sanitization_failure
    return
  }
  printf 'classification=evidence_scan|result=PASS|count=0\n' \
    >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  publish_evidence_directory "$safe_dir"
}

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$project_touched" -eq 1 ]]; then
    "${compose[@]}" stop >/dev/null 2>&1 || true
  fi
  if [[ "$success" -ne 1 && "$evidence_eligible" -eq 1 &&
    ! -e "$retained_evidence_dir" ]]; then
    retain_failure_evidence || true
  fi
  if [[ -n "$runtime_dir" && -d "$runtime_dir" ]]; then
    rm -r -- "$runtime_dir"
  fi
  if [[ "$success" -eq 1 ]]; then
    echo "resources retained for inspection under Compose project: $project_name"
  else
    echo "idempotency acceptance stopped; isolated project resources were retained" >&2
  fi
  exit "$status"
}
trap - EXIT INT TERM
trap on_exit EXIT
trap 'on_exit 130' INT
trap 'on_exit 143' TERM

record_pass() {
  local classification="$1"
  local count="$2"
  [[ -n "$classification" && "$classification" != *[!a-z0-9_]* &&
    -n "$count" && "$count" != *[!0-9]* ]] || {
    sanitization_failed=1
    echo "refusing unsafe evidence classification" >&2
    exit 1
  }
  printf 'classification=%s|result=PASS|count=%s\n' "$classification" "$count" \
    >>"$evidence_dir/acceptance-results.txt"
}

snapshot_production() {
  local output="$1"
  local container
  local matches
  : >"$output"
  for container in "${production_containers[@]}"; do
    if ! matches="$(docker container ls -a --filter "name=^/$container$" --format '{{.Names}}')"; then
      echo "failed to prove production container presence: $container" >&2
      return 1
    fi
    if [[ -z "$matches" ]]; then
      printf '/%s|absent|absent|absent\n' "$container" >>"$output"
      continue
    fi
    [[ "$matches" == "$container" ]] || {
      echo "production container name lookup was ambiguous: $container" >&2
      return 1
    }
    docker inspect --type container \
      --format '{{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}}' \
      "$container" >>"$output" || {
      echo "failed to inspect production container $container" >&2
      return 1
    }
  done
}

printf 'classification=source_manifest|result=PASS|count=%s|sha256=%s\n' \
  "$source_count" "$manifest_sha256" >"$evidence_dir/acceptance-results.txt"

compose_override="$runtime_dir/idempotency-compose.yml"
cat >"$compose_override" <<EOF
services:
  mysql:
    volumes:
      - mysql-data:/var/lib/mysql
      - "$build_context/backend/migrations:/acceptance/migrations:ro"
  idempotency-test:
    profiles:
      - tools
    build:
      context: "$build_context"
      dockerfile: backend/Dockerfile
      target: build
    working_dir: /workspace/backend
    command:
      - go
      - test
      - ./...
    environment:
      DB_DSN: "\${MYSQL_USER:?set MYSQL_USER in .env}:\${MYSQL_PASSWORD:?set MYSQL_PASSWORD in .env}@tcp(mysql:3306)/\${MYSQL_DATABASE:?set MYSQL_DATABASE in .env}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
      TZ: Asia/Shanghai
    volumes:
      - "$build_context:/workspace"
    networks:
      - acceptance
    depends_on:
      mysql:
        condition: service_healthy
EOF
compose+=(--file "$compose_override")

current_stage="production_before"
snapshot_production "$evidence_dir/production-before.txt"
evidence_eligible=1

mysql_file() {
  local container_path="$1"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names < "$1"
  ' sh "$container_path" >/dev/null
}

reset_schema() {
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="
        SET FOREIGN_KEY_CHECKS=0;
        DROP TABLE IF EXISTS file_quota_guards, buyer_intents, buyer_histories,
          buyer_favorites, buyer_device_bindings, buyer_users, idempotency_records,
          auth_sessions, operation_logs, file_records, files, order_events, orders,
          product_images, products, categories, merchant_audit_logs, admin_users,
          merchant_accounts, merchants;
        SET FOREIGN_KEY_CHECKS=1;
      "
  ' >/dev/null
}

apply_migration_chain() {
  local migration
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
  local pass_count
  if ! "${compose[@]}" --profile tools run --rm --no-deps \
    -e IDEMPOTENCY_MYSQL_TEST=1 -e AUTO_MIGRATE="$auto_migrate" \
    idempotency-test go test ./tests \
      -run '^TestIdempotencyMySQLAcceptance$' -count=1 -v >"$raw" 2>&1; then
    echo "idempotency MySQL test failed for AUTO_MIGRATE=$auto_migrate" >&2
    exit 1
  fi
  grep -q -- '--- PASS: TestIdempotencyMySQLAcceptance' "$raw" || {
    echo "focused idempotency PASS marker is missing" >&2
    exit 1
  }
  pass_count="$(grep -Ec '^(--- PASS:|    --- PASS:)' "$raw")"
  record_pass "$label" "$pass_count"
}

current_stage="mysql_start"
project_touched=1
"${compose[@]}" up -d --wait mysql
current_stage="mysql_version"
mysql_version="$("${compose[@]}" exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="SELECT VERSION()"
')"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated idempotency acceptance requires MySQL 8.4.x" >&2
  exit 1
}
record_pass mysql_8_4 1

current_stage="build_test_image"
"${compose[@]}" --profile tools build idempotency-test >/dev/null
current_stage="test_metadata"
if ! "${compose[@]}" --profile tools run --rm --no-deps \
  --user "$host_uid:$host_gid" -e HOME=/tmp \
  idempotency-test sh -ec '
    git init -q /workspace
    git -C /workspace add -- .
    git -C /workspace -c user.name="Acceptance Contract" \
      -c user.email="acceptance-contract@example.invalid" \
      commit -q -m source-snapshot
  ' >"$runtime_dir/test-git-init.raw" 2>&1; then
  echo "initialize temporary test-only Git metadata failed" >&2
  exit 1
fi

disabled_mysql_opts=(
  -e IDEMPOTENCY_MYSQL_TEST=0
  -e BUYER_INTENT_MYSQL_TEST=0
  -e FILE_SCHEMA_MYSQL_TEST=0
  -e SESSION_REVOCATION_MYSQL_TEST=0
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=0
)

current_stage="mysql_auto_migrate_false"
apply_migration_chain
run_focused_test false mysql_auto_migrate_false
record_pass migrations_0001_through_0009_false 9

current_stage="mysql_auto_migrate_true"
apply_migration_chain
run_focused_test true mysql_auto_migrate_true
record_pass migrations_0001_through_0009_true 9

current_stage="backend_tests"
backend_raw="$runtime_dir/backend-tests.raw"
if ! "${compose[@]}" --profile tools run --rm --no-deps \
  "${disabled_mysql_opts[@]}" \
  idempotency-test go test ./... -count=1 >"$backend_raw" 2>&1; then
  echo "go test ./... failed" >&2
  exit 1
fi
record_pass backend_tests "$(grep -Ec '^(ok|\?)[[:space:]]' "$backend_raw")"

current_stage="backend_race"
race_raw="$runtime_dir/backend-race.raw"
if ! "${compose[@]}" --profile tools run --rm --no-deps \
  "${disabled_mysql_opts[@]}" \
  idempotency-test go test -race ./internal/app ./tests -count=1 >"$race_raw" 2>&1; then
  echo "go test -race ./internal/app ./tests failed" >&2
  exit 1
fi
record_pass backend_race "$(grep -Ec '^ok[[:space:]]' "$race_raw")"

current_stage="go_vet"
if ! "${compose[@]}" --profile tools run --rm --no-deps \
  "${disabled_mysql_opts[@]}" \
  idempotency-test go vet ./... >"$runtime_dir/go-vet.raw" 2>&1; then
  echo "go vet ./... failed" >&2
  exit 1
fi
record_pass go_vet 1

current_stage="production_after"
snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}
record_pass production_snapshot_unchanged 3

current_stage="isolated_stop"
"${compose[@]}" stop >/dev/null
project_touched=0
record_pass isolated_containers_stopped 1

current_stage="evidence_scan"
if ! scan_evidence_directory "$evidence_dir" "$runtime_dir/evidence-leaks.txt"; then
  sanitization_failed=1
  echo "sanitized evidence scan failed or found forbidden content" >&2
  exit 1
fi
printf 'classification=evidence_scan|result=PASS|count=0\n' \
  >"$evidence_dir/evidence-leak-scan.txt"

hash_evidence_directory "$evidence_dir"
current_stage="evidence_publish"
publish_evidence_directory "$evidence_dir"

success=1
echo "isolated idempotency atomicity acceptance passed"
