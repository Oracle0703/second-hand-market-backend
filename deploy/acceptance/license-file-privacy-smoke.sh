#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_name="secondhand-license-privacy-acceptance"
evidence_dir="$base_dir/evidence/license-file-privacy"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)

[[ "${LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA" ]] || {
  echo "isolated license file privacy confirmation is missing" >&2
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
[[ "$project_name" == "secondhand-license-privacy-acceptance" ]] || {
  echo "unexpected license privacy Compose project" >&2
  exit 1
}
[[ -f "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}

existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}

mkdir -p "$evidence_dir"
chmod 700 "$evidence_dir"

snapshot_production() {
  local output="$1"
  : >"$output"
  for container in "${production_containers[@]}"; do
    docker inspect --type container --format '{{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}}' "$container" >>"$output"
  done
}

snapshot_production "$evidence_dir/production-before.txt"

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
  apply_chain_0001_0006
  mysql_sql "$valid_fixture_sql $fixture_sql"
  capture_file_state >"$evidence_dir/$name-before.txt"
  if mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql >"$evidence_dir/$name.txt" 2>&1; then
    echo "expected license privacy preflight failure for $name" >&2
    exit 1
  fi
  grep -Eq -- 'ERROR 1644 \(45000\)' "$evidence_dir/$name.txt" || {
    echo "license privacy preflight $name did not fail with SQLSTATE 45000" >&2
    exit 1
  }
  grep -Fq -- "$expected_message" "$evidence_dir/$name.txt" || {
    echo "license privacy preflight $name failed for an unexpected reason" >&2
    exit 1
  }
  grep -Fq -- 'license_file_privacy_preflight_passed' "$evidence_dir/$name.txt" && {
    echo "license privacy preflight $name emitted a success marker" >&2
    exit 1
  }
  capture_file_state >"$evidence_dir/$name-after.txt"
  cmp -s "$evidence_dir/$name-before.txt" "$evidence_dir/$name-after.txt" || {
    echo "license privacy preflight $name changed file rows or license URLs" >&2
    exit 1
  }
}

"${compose[@]}" up -d --wait mysql
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" | tee "$evidence_dir/mysql-version.txt"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated license file privacy acceptance requires MySQL 8.4.x, got $mysql_version" >&2
  exit 1
}

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

apply_chain_0001_0006
mysql_sql "$valid_fixture_sql"
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
} | tee "$evidence_dir/clean-migration.txt"

[[ "$(mysql_sql "SELECT url FROM file_records WHERE id=302")" == "$product_url_before" ]]
[[ -z "$(mysql_sql "SELECT url FROM file_records WHERE id=301")" ]]
[[ "$(mysql_sql 'SELECT COUNT(*) FROM file_records')" == "$file_count_before" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE biz_type='MERCHANT_LICENSE'")" == "$license_count_before" ]]
grep -q license_file_privacy_preflight_passed "$evidence_dir/clean-migration.txt"
grep -q license_file_privacy_postflight_passed "$evidence_dir/clean-migration.txt"

"${compose[@]}" --profile tools build bootstrap-admin
"${compose[@]}" --profile tools run --rm \
  -e FILE_SCHEMA_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=false \
  -e FILE_UPLOAD_LOCAL_DIR=/tmp/license-file-privacy-uploads \
  bootstrap-admin go test ./tests -run '^TestLicenseFilePrivacyWithMigrationOnlyMySQL$' -count=1 -v \
  | tee "$evidence_dir/api-auto-migrate-false.txt"
grep -q -- '--- PASS: TestLicenseFilePrivacyWithMigrationOnlyMySQL' "$evidence_dir/api-auto-migrate-false.txt"
mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql \
  | tee "$evidence_dir/post-api-auto-migrate-false.txt"

"${compose[@]}" --profile tools run --rm \
  -e FILE_SCHEMA_MYSQL_TEST=1 \
  -e AUTO_MIGRATE=true \
  -e FILE_UPLOAD_LOCAL_DIR=/tmp/license-file-privacy-uploads \
  bootstrap-admin go test ./tests -run '^TestLicenseFilePrivacyWithMigrationOnlyMySQL$' -count=1 -v \
  | tee "$evidence_dir/api-auto-migrate-true.txt"
grep -q -- '--- PASS: TestLicenseFilePrivacyWithMigrationOnlyMySQL' "$evidence_dir/api-auto-migrate-true.txt"
mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql \
  | tee "$evidence_dir/post-api-auto-migrate-true.txt"

snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}

find "$evidence_dir" -maxdepth 1 -type f -name '*.txt' ! -name 'sha256.txt' -print0 \
  | sort -z | xargs -0 sha256sum >"$evidence_dir/sha256.txt"

echo "isolated license file privacy acceptance passed"
echo "mysql version: $mysql_version"
echo "resources retained for inspection under Compose project: $project_name"
