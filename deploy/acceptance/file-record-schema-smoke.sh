#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_name="secondhand-file-schema-acceptance"
evidence_dir="$base_dir/evidence/file-record-schema"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")

[[ "${FILE_SCHEMA_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA" ]] || {
  echo "isolated file schema confirmation is missing" >&2
  exit 1
}
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || {
  echo "ACCEPTANCE_DB_ENGINE must be mysql8.4" >&2
  exit 1
}
[[ -f "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}

existing_containers="$("${compose[@]}" ps -aq 2>/dev/null || true)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}

mkdir -p "$evidence_dir"

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

expect_gate_failure() {
  local path="$1"
  if mysql_file "$path"; then
    echo "expected migration gate failure for $path" >&2
    exit 1
  fi
}

reset_file_tables() {
  mysql_sql 'DROP TABLE IF EXISTS file_records; DROP TABLE IF EXISTS files;'
}

apply_0001() {
  mysql_file /acceptance/migrations/0001_init.up.sql
}

run_0005() {
  mysql_file /acceptance/migrations/0005_file_records_table.preflight.sql
  mysql_file /acceptance/migrations/0005_file_records_table.up.sql
  mysql_file /acceptance/migrations/0005_file_records_table.postflight.sql
}

"${compose[@]}" up -d --wait mysql
"${compose[@]}" ps mysql
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" | tee "$evidence_dir/mysql-version.txt"

# Legacy files-only state: preserve the exact sentinel row across rename.
reset_file_tables
apply_0001
mysql_sql "INSERT INTO files (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status) VALUES (900001,'MERCHANT_LICENSE','f09/legacy','/uploads/f09-legacy.jpg','image/jpeg',22,'PUBLIC',NULL,'PENDING')"
legacy_before="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,COALESCE(uploader_id,'NULL'),scan_status,created_at),256) FROM files WHERE id=900001")"
run_0005 | tee "$evidence_dir/legacy.txt"
legacy_after="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,COALESCE(uploader_id,'NULL'),scan_status,created_at),256) FROM file_records WHERE id=900001")"
[[ "$legacy_before" == "$legacy_after" ]] || { echo "legacy sentinel changed" >&2; exit 1; }
grep -q file_records_migration_renamed "$evidence_dir/legacy.txt"
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='files'")" == "0" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='file_records'")" == "1" ]]

# Canonical file_records-only state: 0005 must be a no-op with unchanged row count.
reset_file_tables
apply_0001
mysql_sql 'RENAME TABLE files TO file_records'
mysql_sql "INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status) VALUES (900002,'PRODUCT_IMAGE','f09/canonical','/uploads/f09-canonical.jpg','image/jpeg',22,'MERCHANT',7,'PASS')"
canonical_before="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status,created_at),256) FROM file_records WHERE id=900002")"
canonical_count_before="$(mysql_sql 'SELECT COUNT(*) FROM file_records')"
run_0005 | tee "$evidence_dir/canonical.txt"
canonical_after="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status,created_at),256) FROM file_records WHERE id=900002")"
canonical_count_after="$(mysql_sql 'SELECT COUNT(*) FROM file_records')"
[[ "$canonical_before" == "$canonical_after" ]] || { echo "canonical sentinel changed" >&2; exit 1; }
[[ "$canonical_count_before" == "$canonical_count_after" ]] || { echo "canonical row count changed" >&2; exit 1; }
grep -q file_records_migration_noop "$evidence_dir/canonical.txt"

# Ambiguous state: preflight must fail and preserve both tables.
reset_file_tables
apply_0001
mysql_sql 'CREATE TABLE file_records LIKE files'
mysql_sql "INSERT INTO files (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status) VALUES (900003,'OTHER','f09/files','/uploads/f09-files.jpg','image/jpeg',22,'PUBLIC','PENDING')"
mysql_sql "INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status) VALUES (900004,'OTHER','f09/file-records','/uploads/f09-file-records.jpg','image/jpeg',22,'PUBLIC','PENDING')"
expect_gate_failure /acceptance/migrations/0005_file_records_table.preflight.sql
[[ "$(mysql_sql "SELECT COUNT(*) FROM files WHERE id=900003")" == "1" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE id=900004")" == "1" ]]

# Missing state: preflight must fail without creating either table.
reset_file_tables
expect_gate_failure /acceptance/migrations/0005_file_records_table.preflight.sql
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('files','file_records')")" == "0" ]]

# Preflight-to-up drift on legacy files: drop a required column after preflight.
# Up must fail before rename and leave the table named files.
reset_file_tables
apply_0001
mysql_sql "INSERT INTO files (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status) VALUES (900005,'OTHER','f09/drift-col','/uploads/f09-drift-col.jpg','image/jpeg',22,'PUBLIC','PENDING')"
mysql_file /acceptance/migrations/0005_file_records_table.preflight.sql
mysql_sql 'ALTER TABLE files DROP COLUMN mime_type'
if mysql_file /acceptance/migrations/0005_file_records_table.up.sql >"$evidence_dir/drift-legacy-column.txt" 2>&1; then
  echo "expected up failure after legacy column drift" >&2
  exit 1
fi
if grep -Eq 'file_records_migration_(renamed|noop)' "$evidence_dir/drift-legacy-column.txt"; then
  echo "up emitted success marker after legacy column drift" >&2
  exit 1
fi
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='files'")" == "1" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='file_records'")" == "0" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM files WHERE id=900005")" == "1" ]]

# Preflight-to-up drift on canonical file_records: drop required index after preflight.
# Up must fail before noop marker and leave the table named file_records.
reset_file_tables
apply_0001
mysql_sql 'RENAME TABLE files TO file_records'
mysql_sql "INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status) VALUES (900006,'OTHER','f09/drift-idx','/uploads/f09-drift-idx.jpg','image/jpeg',22,'PUBLIC','PENDING')"
mysql_file /acceptance/migrations/0005_file_records_table.preflight.sql
# Drop the composite (biz_type, created_at) index; name from 0001 is idx_biz_type_created.
idx_name="$(mysql_sql "SELECT index_name FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='file_records' AND column_name='biz_type' AND seq_in_index=1 LIMIT 1")"
[[ -n "$idx_name" ]] || { echo "could not locate biz_type leading index" >&2; exit 1; }
mysql_sql "ALTER TABLE file_records DROP INDEX \`$idx_name\`"
if mysql_file /acceptance/migrations/0005_file_records_table.up.sql >"$evidence_dir/drift-canonical-index.txt" 2>&1; then
  echo "expected up failure after canonical index drift" >&2
  exit 1
fi
if grep -Eq 'file_records_migration_(renamed|noop)' "$evidence_dir/drift-canonical-index.txt"; then
  echo "up emitted success marker after canonical index drift" >&2
  exit 1
fi
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='files'")" == "0" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='file_records'")" == "1" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE id=900006")" == "1" ]]

# Clean full migration chain, migration-only API flow, then AutoMigrate compatibility.
# Historical 0001 down only drops `files`, not `file_records`. Clear both file
# tables after the drift assertions so the clean chain never starts with a
# leftover canonical table beside a freshly created `files` table.
reset_file_tables
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('files','file_records')")" == "0" ]]
mysql_file /acceptance/migrations/0001_init.down.sql
apply_0001
mysql_file /acceptance/migrations/0002_buyer_domain.up.sql
mysql_file /acceptance/migrations/0003_buyer_auth_provider.up.sql
mysql_file /acceptance/migrations/0004_merchant_multi_stock.preflight.sql
mysql_file /acceptance/migrations/0004_merchant_multi_stock.up.sql
mysql_file /acceptance/migrations/0004_merchant_multi_stock.postflight.sql
run_0005 | tee "$evidence_dir/full-chain.txt"
mysql_file /acceptance/migrations/0006_file_binding_ownership.preflight.sql
mysql_file /acceptance/migrations/0006_file_binding_ownership.up.sql
mysql_file /acceptance/migrations/0006_file_binding_ownership.postflight.sql
mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql
mysql_file /acceptance/migrations/0007_license_file_privacy.up.sql
mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.up.sql
mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
"${compose[@]}" --profile tools build bootstrap-admin
"${compose[@]}" --profile tools run --rm \
  -e FILE_SCHEMA_MYSQL_TEST=1 \
  bootstrap-admin go test ./tests -run '^TestFileFlowWithMigrationOnlyMySQL$' -count=1 -v \
  | tee "$evidence_dir/file-flow.txt"

grep -q file_records_preflight_passed "$evidence_dir/full-chain.txt"
grep -Eq 'file_records_migration_(renamed|noop)' "$evidence_dir/full-chain.txt"
grep -q file_records_postflight_passed "$evidence_dir/full-chain.txt"
grep -q -- '--- PASS: TestFileFlowWithMigrationOnlyMySQL' "$evidence_dir/file-flow.txt"

echo "isolated file schema acceptance passed"
echo "mysql version: $mysql_version"
echo "resources retained for inspection under Compose project: $project_name"
