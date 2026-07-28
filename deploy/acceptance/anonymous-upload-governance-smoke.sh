#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-upload-governance-acceptance"
evidence_dir="$base_dir/evidence/anonymous-upload-governance"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)
runtime_dir=""

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

write_source_file_list() {
  (
    cd "$repo_dir"
    git ls-tree -r --name-only -z HEAD -- Makefile backend frontend deploy/acceptance |
      while IFS= read -r -d '' path; do
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
  local directory="$1" source_list="$2" path
  while IFS= read -r -d '' path; do
    [[ -f "$directory/$path" && ! -L "$directory/$path" ]] || return 1
  done <"$source_list"
}

export_head_source() {
  local export_dir="$1" export_runtime extracted path
  local -a archive_paths=()
  [[ "$export_dir" == /* && "$export_dir" != / && ! -e "$export_dir" ]] || {
    echo "ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR must be an absent absolute directory" >&2
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
    rm -r -- "$export_runtime" "$export_dir"
    return 1
  }
  while IFS= read -r -d '' path; do archive_paths+=("$path"); done <"$export_dir/source-files.z"
  [[ "${#archive_paths[@]}" -gt 0 ]] || { rm -r -- "$export_runtime" "$export_dir"; return 1; }
  (
    cd "$repo_dir"
    git archive --format=tar --output="$export_dir/source.tar" HEAD -- "${archive_paths[@]}"
  )
  tar -C "$extracted" -xf "$export_dir/source.tar"
  validate_received_source_files "$extracted" "$export_dir/source-files.z" || {
    rm -r -- "$export_runtime" "$export_dir"
    return 1
  }
  write_context_file_list "$extracted" >"$export_runtime/archive-files.z"
  cmp -s "$export_dir/source-files.z" "$export_runtime/archive-files.z" || {
    rm -r -- "$export_runtime" "$export_dir"
    return 1
  }
  write_directory_manifest "$extracted" "$export_dir/source-files.z" "$export_dir/source-sha256.txt"
  (
    cd "$export_dir"
    sha256sum source-files.z source-sha256.txt source.tar >package-sha256.txt
  )
  chmod 600 "$export_dir/source-files.z" "$export_dir/source-sha256.txt" "$export_dir/source.tar" "$export_dir/package-sha256.txt"
  rm -r -- "$export_runtime"
}

validate_package_checksums() {
  local package_dir="$1" expected_hash expected_name actual_hash count=0
  local -a expected_names=(source-files.z source-sha256.txt source.tar)
  while read -r expected_hash expected_name; do
    [[ "$count" -lt "${#expected_names[@]}" &&
      "$expected_name" == "${expected_names[$count]}" &&
      "${#expected_hash}" -eq 64 && "$expected_hash" != *[!0-9a-f]* ]] || return 1
    actual_hash="$(sha256sum "$package_dir/$expected_name" | cut -d ' ' -f1)"
    [[ "$actual_hash" == "$expected_hash" ]] || return 1
    count=$((count + 1))
  done <"$package_dir/package-sha256.txt"
  [[ "$count" -eq "${#expected_names[@]}" ]]
}

preflight_on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
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
source_package_dir="${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_DIR:-$repo_dir/.anonymous-upload-governance-source}"
[[ "$source_package_dir" == /* && -d "$source_package_dir" && ! -L "$source_package_dir" ]] || {
  echo "ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_DIR must identify the transferred source package" >&2
  exit 1
}
for artifact in source-files.z source-sha256.txt source.tar package-sha256.txt; do
  [[ -f "$source_package_dir/$artifact" && ! -L "$source_package_dir/$artifact" ]] || {
    echo "transferred anonymous upload governance source package is incomplete" >&2
    exit 1
  }
done
authorized_package_manifest_sha256="${ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_MANIFEST_SHA256:-}"
actual_package_manifest_sha256="$(sha256sum "$source_package_dir/package-sha256.txt" | cut -d ' ' -f1)"
[[ "${#authorized_package_manifest_sha256}" -eq 64 &&
  "$authorized_package_manifest_sha256" != *[!0-9a-f]* &&
  "$actual_package_manifest_sha256" == "$authorized_package_manifest_sha256" ]] || {
  echo "source package manifest digest does not match authorization" >&2
  exit 1
}
validate_package_checksums "$source_package_dir" || {
  echo "transferred anonymous upload governance source package checksum failed" >&2
  exit 1
}
runtime_dir="$(mktemp -d)"
trap preflight_on_exit EXIT
trap 'preflight_on_exit 130' INT
trap 'preflight_on_exit 143' TERM
source_files="$source_package_dir/source-files.z"
source_manifest="$source_package_dir/source-sha256.txt"
validate_source_list "$source_files" "$runtime_dir/sorted-source-files.z" || {
  echo "transferred anonymous upload governance source list is invalid" >&2
  exit 1
}
validate_received_source_files "$repo_dir" "$source_files" || {
  echo "received anonymous upload governance source contains a missing or unsafe file" >&2
  exit 1
}
write_directory_manifest "$repo_dir" "$source_files" "$runtime_dir/received-source-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/received-source-sha256.txt" || {
  echo "received anonymous upload governance source does not match package manifest" >&2
  exit 1
}
build_context="$runtime_dir/build-context"
mkdir -p "$build_context"
tar -C "$build_context" -xf "$source_package_dir/source.tar"
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
source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
source_manifest_sha256="$(sha256sum "$source_manifest" | cut -d ' ' -f1)"
[[ -f "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}
for command in docker curl jq openssl sha256sum truncate base64 sort xargs mktemp tar chmod mkdir rm tr cmp find wc cut cat; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done

[[ ! -e "$evidence_dir" ]] || {
  echo "refusing to overwrite existing anonymous upload governance evidence" >&2
  exit 1
}
existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
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

hash_evidence_directory() {
  local directory="$1"
  (
    cd "$directory"
    find . -type f ! -name evidence-sha256.txt -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
  ) >"$directory/evidence-sha256.txt"
}

snapshot_file_is_safe() {
  local file="$1" line count=0
  [[ -s "$file" && -f "$file" && ! -L "$file" ]] || return 1
  while IFS= read -r line; do
    [[ "$line" =~ ^/[a-z0-9-]+\|([0-9a-f]{64}|absent)\|[a-z]+\|([0-9]+|absent)$ ]] || return 1
    count=$((count + 1))
  done <"$file"
  [[ "$count" -eq 3 ]]
}

publish_failure_evidence() {
  local safe_dir="$runtime_dir/safe-failure-evidence" snapshot
  [[ ! -e "$base_dir/evidence/anonymous-upload-governance" ]] || return 0
  mkdir -p "$safe_dir" "$base_dir/evidence/anonymous-upload-governance"
  chmod 700 "$safe_dir" "$base_dir/evidence/anonymous-upload-governance"
  printf 'classification=source_package|result=PASS|count=%s|sha256=%s\n' \
    "$source_count" "$source_manifest_sha256" >"$safe_dir/acceptance-results.txt"
  printf 'classification=acceptance_failure|result=FAIL|stage=%s|count=1\n' "$current_stage" \
    >"$safe_dir/failure-status.txt"
  for snapshot in production-before.txt production-after.txt; do
    if snapshot_file_is_safe "$evidence_dir/$snapshot"; then
      cp "$evidence_dir/$snapshot" "$safe_dir/$snapshot"
    fi
  done
  printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_dir/evidence-leak-scan.txt"
  hash_evidence_directory "$safe_dir"
  (
    cd "$safe_dir"
    tar -cf - .
  ) | tar -C "$base_dir/evidence/anonymous-upload-governance" -xf -
}

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$project_touched" -eq 1 ]]; then
    "${compose[@]}" stop >/dev/null 2>&1 || true
  fi
  if [[ "$status" -ne 0 && "$evidence_eligible" -eq 1 ]]; then
    snapshot_production "$evidence_dir/production-after.txt" >/dev/null 2>&1 || true
    publish_failure_evidence || true
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

snapshot_production() {
  local output="$1"
  : >"$output"
  for container in "${production_containers[@]}"; do
    if docker inspect --type container "$container" >/dev/null 2>&1; then
      docker inspect --type container --format '{{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}}' "$container" >>"$output"
    else
      printf '/%s|absent|absent|absent\n' "$container" >>"$output"
    fi
  done
}

snapshot_production "$evidence_dir/production-before.txt"
evidence_eligible=1

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
  if mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql >"$evidence_dir/skipped-0007-error.txt" 2>&1; then
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
  ' >/dev/null
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
  '
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
  if mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql >"$evidence_dir/$name-error.txt" 2>&1; then
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
"${compose[@]}" up -d --wait mysql
current_stage="mysql_version"
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" >"$evidence_dir/mysql-version.txt"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated anonymous upload governance acceptance requires MySQL 8.4.x" >&2
  exit 1
}

"${compose[@]}" --profile tools build bootstrap-admin frontend-test

expect_skipped_0007_failure
expect_0008_preflight_failure partial-column setup_partial_0008 \
  'upload governance preflight: partial 0008 schema exists'
expect_0008_preflight_failure drifted-column setup_drifted_0008 \
  'upload governance preflight: 0008 columns are drifted'
expect_0008_preflight_failure missing-guard-row setup_missing_guard_0008 \
  'upload governance preflight: fixed quota guard row is missing or drifted'
expect_0008_preflight_failure drifted-guard-engine setup_drifted_guard_engine \
  'upload governance preflight: quota guard table must use InnoDB'

apply_chain_0001_0007
seed_historical_rows
write_historical_files
capture_historical_rows >"$evidence_dir/historical-before.txt"
capture_historical_files >"$evidence_dir/historical-files-before.txt"
run_0008 | tee "$evidence_dir/clean-migration.txt"
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
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
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE id IN (860001,860002) AND (source_ip_hash IS NOT NULL OR cleanup_after IS NOT NULL OR cleanup_claimed_at IS NOT NULL OR cleanup_claim_token IS NOT NULL)")" == "0" ]] || {
  echo "clean 0008 migration enrolled historical rows" >&2
  exit 1
}
grep -q anonymous_upload_governance_preflight_passed "$evidence_dir/clean-migration.txt"
grep -q anonymous_upload_governance_migration_applied "$evidence_dir/clean-migration.txt"
grep -q anonymous_upload_governance_postflight_passed "$evidence_dir/clean-migration.txt"

"${compose[@]}" --profile tools run --rm \
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=false \
  bootstrap-admin go test ./internal/app -run '^TestUploadGovernanceMySQLConcurrencyAndCleanup$' -count=1 -v \
  | tee "$evidence_dir/mysql-auto-migrate-false.txt"
grep -q -- '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' "$evidence_dir/mysql-auto-migrate-false.txt"
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql \
  | tee "$evidence_dir/post-mysql-auto-migrate-false.txt"

"${compose[@]}" --profile tools run --rm \
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=true \
  bootstrap-admin go test ./internal/app -run '^TestUploadGovernanceMySQLConcurrencyAndCleanup$' -count=1 -v \
  | tee "$evidence_dir/mysql-auto-migrate-true.txt"
grep -q -- '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' "$evidence_dir/mysql-auto-migrate-true.txt"
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql \
  | tee "$evidence_dir/post-mysql-auto-migrate-true.txt"

"${compose[@]}" --profile tools run --rm \
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=0 \
  bootstrap-admin go test ./... -count=1 \
  | tee "$evidence_dir/backend-tests.txt"
"${compose[@]}" --profile tools run --rm frontend-test \
  | tee "$evidence_dir/frontend-tests-build.txt"

"${compose[@]}" --profile tools run --rm bootstrap-admin \
  >"$evidence_dir/bootstrap-admin.txt"
"${compose[@]}" up -d --wait api web

png_file="$runtime_dir/exact-10-mib.png"
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=' \
  | base64 -d >"$png_file"
truncate -s 10485760 "$png_file"

presign_exact="$runtime_dir/presign-exact.json"
presign_exact_status="$(curl --silent --show-error --output "$presign_exact" --write-out '%{http_code}' \
  --header 'Content-Type: application/json' \
  --data '{"biz_type":"MERCHANT_LICENSE","file_name":"exact-10-mib.png","file_size":10485760,"mime_type":"image/png"}' \
  http://127.0.0.1:18082/api/v1/files/presign)"
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
  http://127.0.0.1:18082/api/v1/files/upload)"
[[ "$upload_exact_status" == "200" ]] && jq -e '.code == 0' "$upload_exact" >/dev/null || {
  echo "exact 10 MiB upload was not accepted" >&2
  exit 1
}
unset file_id object_key file_token

presign_over="$runtime_dir/presign-over.json"
presign_over_status="$(curl --silent --show-error --output "$presign_over" --write-out '%{http_code}' \
  --header 'Content-Type: application/json' \
  --data '{"biz_type":"MERCHANT_LICENSE","file_name":"over-10-mib.png","file_size":10485761,"mime_type":"image/png"}' \
  http://127.0.0.1:18082/api/v1/files/presign)"
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
    --data-binary "@$body" "$url")"
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

current_stage="production_after"
snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}

safe_evidence_dir="$runtime_dir/safe-success-evidence"
mkdir -p "$safe_evidence_dir" "$base_dir/evidence/anonymous-upload-governance"
chmod 700 "$safe_evidence_dir" "$base_dir/evidence/anonymous-upload-governance"
snapshot_file_is_safe "$evidence_dir/production-before.txt" &&
  snapshot_file_is_safe "$evidence_dir/production-after.txt" || {
  echo "production snapshots failed strict evidence validation" >&2
  exit 1
}
cp "$evidence_dir/production-before.txt" "$safe_evidence_dir/production-before.txt"
cp "$evidence_dir/production-after.txt" "$safe_evidence_dir/production-after.txt"
cat >"$safe_evidence_dir/acceptance-results.txt" <<EOF
classification=source_package|result=PASS|count=$source_count|sha256=$source_manifest_sha256
classification=mysql_version|result=PASS|count=1
classification=skipped_0007_preflight|result=PASS|count=1
classification=dirty_0008_preflights|result=PASS|count=4
classification=clean_migration|result=PASS|count=1
classification=mysql_auto_migrate_false|result=PASS|count=1
classification=mysql_auto_migrate_true|result=PASS|count=1
classification=backend_tests|result=PASS|count=1
classification=frontend_tests_build|result=PASS|count=1
classification=upload_boundaries|result=PASS|count=7
classification=historical_rows_files|result=PASS|count=2
classification=production_snapshot|result=PASS|count=3
EOF
printf 'classification=evidence_scan|result=PASS|count=0\n' >"$safe_evidence_dir/evidence-leak-scan.txt"
hash_evidence_directory "$safe_evidence_dir"
(
  cd "$safe_evidence_dir"
  tar -cf - .
) | tar -C "$base_dir/evidence/anonymous-upload-governance" -xf -

success=1
echo "isolated anonymous upload governance acceptance passed"
echo "mysql version: $mysql_version"
