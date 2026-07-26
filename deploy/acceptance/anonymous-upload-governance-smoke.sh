#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-upload-governance-acceptance"
evidence_dir="$base_dir/evidence/anonymous-upload-governance"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)
runtime_dir=""

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
[[ -f "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}
for command in docker curl jq openssl sha256sum truncate base64; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done

existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}
[[ ! -e "$evidence_dir" ]] || {
  echo "refusing to overwrite existing anonymous upload governance evidence" >&2
  exit 1
}
if command -v ss >/dev/null && ss -ltnH | awk '{print $4}' | grep -Eq '(^|:)(18081|18082)$'; then
  echo "acceptance loopback port 18081 or 18082 is already in use" >&2
  exit 1
fi

mkdir -p "$evidence_dir"
chmod 700 "$evidence_dir"
runtime_dir="$(mktemp -d)"
success=0

on_exit() {
  local status=$?
  trap - EXIT INT TERM
  if docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q | grep -q .; then
    if [[ "$status" -ne 0 ]]; then
      echo "anonymous upload governance acceptance failed; retained service state follows" >&2
      "${compose[@]}" ps >&2 || true
    fi
    "${compose[@]}" stop >/dev/null 2>&1 || true
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

write_source_manifest() {
  (
    cd "$repo_dir"
    {
      printf '%s\0' Makefile
      find backend -type f \( -name '*.go' -o -name '*.sql' -o -name 'go.mod' -o -name 'go.sum' -o -name 'Dockerfile' \) \
        ! -path '*/.cache/*' ! -path '*/uploads/*' ! -name 'app.db' -print0
      find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.css' \) -print0
      printf '%s\0' frontend/package.json frontend/package-lock.json
      find deploy/acceptance -maxdepth 2 -type f \
        ! -name '.env' ! -path '*/secrets/*' ! -path '*/backups/*' ! -path '*/evidence/*' -print0
    } | LC_ALL=C sort -z | xargs -0 sha256sum
  ) >"$evidence_dir/source-sha256.txt"
}

snapshot_production "$evidence_dir/production-before.txt"
write_source_manifest

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

apply_chain_0001_0007() {
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
  mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql
  mysql_file /acceptance/migrations/0007_license_file_privacy.up.sql
  mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql
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

"${compose[@]}" up -d --wait mysql
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" >"$evidence_dir/mysql-version.txt"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated anonymous upload governance acceptance requires MySQL 8.4.x" >&2
  exit 1
}

"${compose[@]}" --profile tools build bootstrap-admin frontend-test

expect_0008_preflight_failure partial-column setup_partial_0008 \
  'upload governance preflight: partial 0008 schema exists'
expect_0008_preflight_failure drifted-column setup_drifted_0008 \
  'upload governance preflight: 0008 columns are drifted'
expect_0008_preflight_failure missing-guard-row setup_missing_guard_0008 \
  'upload governance preflight: fixed quota guard row is missing or drifted'

apply_chain_0001_0007
seed_historical_rows
write_historical_files
capture_historical_rows >"$evidence_dir/historical-before.txt"
capture_historical_files >"$evidence_dir/historical-files-before.txt"
run_0008 | tee "$evidence_dir/clean-migration.txt"
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

snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}

if grep -ERn --binary-files=without-match \
  'DB_DSN=|MYSQL_PASSWORD=|MYSQL_ROOT_PASSWORD=|JWT_ACCESS_SECRET=|JWT_REFRESH_SECRET=|FILE_UPLOAD_IP_HASH_SECRET=|file_token["=:]|object_key["=:]|/var/lib/second-hand-market/uploads|192\.0\.2\.' \
  "$evidence_dir" >"$runtime_dir/evidence-leaks.txt"; then
  echo "sanitized evidence check found a forbidden secret or identifier" >&2
  exit 1
fi

(
  cd "$evidence_dir"
  find . -maxdepth 1 -type f -name '*.txt' ! -name 'evidence-sha256.txt' -print0 \
    | LC_ALL=C sort -z | xargs -0 sha256sum
) >"$evidence_dir/evidence-sha256.txt"

success=1
echo "isolated anonymous upload governance acceptance passed"
echo "mysql version: $mysql_version"
