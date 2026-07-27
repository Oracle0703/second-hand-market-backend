#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_name="secondhand-file-binding-acceptance"
evidence_dir="$base_dir/evidence/file-binding-authorization"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")

[[ "${FILE_BINDING_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA" ]] || {
  echo "isolated file binding confirmation is missing" >&2
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

existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q)"
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

apply_base_chain() {
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
}

run_0006() {
  mysql_file /acceptance/migrations/0006_file_binding_ownership.preflight.sql
  mysql_file /acceptance/migrations/0006_file_binding_ownership.up.sql
  mysql_file /acceptance/migrations/0006_file_binding_ownership.postflight.sql
}

expect_preflight_failure() {
  local name="$1"
  local expected_message="$2"
  local fixture_sql="$3"
  apply_base_chain
  mysql_sql "$fixture_sql"
  if mysql_file /acceptance/migrations/0006_file_binding_ownership.preflight.sql >"$evidence_dir/$name.txt" 2>&1; then
    echo "expected file binding preflight failure for $name" >&2
    exit 1
  fi
  grep -Eq -- 'ERROR 1644 \(45000\)' "$evidence_dir/$name.txt" || {
    echo "file binding preflight $name did not fail with SQLSTATE 45000" >&2
    exit 1
  }
  grep -Fq -- "$expected_message" "$evidence_dir/$name.txt" || {
    echo "file binding preflight $name failed for an unexpected reason" >&2
    exit 1
  }
  [[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='file_records' AND column_name='owner_merchant_id'")" == "0" ]] || {
    echo "preflight failure $name mutated the schema" >&2
    exit 1
  }
}

merchant_one="
  INSERT INTO merchants (id,merchant_no,merchant_name,contact_name,contact_phone,review_status)
  VALUES (1,'M-F02-1','Merchant One','Owner One','13800138001','APPROVED');
  INSERT INTO merchant_accounts (id,merchant_id,username,password_hash,role,status)
  VALUES (11,1,'f02_owner_1','test-hash','OWNER','ACTIVE');
  INSERT INTO categories (id,parent_id,level,name,status,sort)
  VALUES (101,NULL,1,'F02 Root','ENABLED',1),(102,101,2,'F02 Leaf','ENABLED',1);
  INSERT INTO products (id,product_no,merchant_id,title,description,category_id,price_cent,original_price_cent,condition_level,stock,status,created_by,updated_by,version)
  VALUES (201,'P-F02-1',1,'Fixture Product','fixture',102,100,120,'GOOD',1,'DRAFT',11,11,1);
"

"${compose[@]}" up -d --wait mysql
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" | tee "$evidence_dir/mysql-version.txt"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated file binding acceptance requires MySQL 8.4.x, got $mysql_version" >&2
  exit 1
}

# Clean references and unbound files: verify exact owner backfill behavior.
apply_base_chain
mysql_sql "$merchant_one
  INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status)
  VALUES
    (301,'PRODUCT_IMAGE','f02/product','/uploads/f02-product.jpg','image/jpeg',22,'MERCHANT',11,'PASS'),
    (302,'MERCHANT_LICENSE','f02/license','/uploads/f02-license.jpg','image/jpeg',22,'PUBLIC',NULL,'PASS'),
    (303,'PRODUCT_IMAGE','f02/unbound-merchant','/uploads/f02-unbound-merchant.jpg','image/jpeg',22,'MERCHANT',11,'PASS'),
    (304,'MERCHANT_LICENSE','f02/unbound-public','/uploads/f02-unbound-public.jpg','image/jpeg',22,'PUBLIC',NULL,'PASS');
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,301,1);
  UPDATE merchants SET license_file_id=302 WHERE id=1;
"
run_0006 | tee "$evidence_dir/clean-backfill.txt"
grep -q file_binding_ownership_preflight_passed "$evidence_dir/clean-backfill.txt"
grep -q file_binding_ownership_postflight_passed "$evidence_dir/clean-backfill.txt"
[[ "$(mysql_sql 'SELECT owner_merchant_id FROM file_records WHERE id=301')" == "1" ]]
[[ "$(mysql_sql 'SELECT owner_merchant_id FROM file_records WHERE id=302')" == "1" ]]
[[ "$(mysql_sql 'SELECT owner_merchant_id FROM file_records WHERE id=303')" == "1" ]]
[[ "$(mysql_sql "SELECT IF(owner_merchant_id IS NULL,'NULL',owner_merchant_id) FROM file_records WHERE id=304")" == "NULL" ]]

expect_preflight_failure orphan "file binding preflight: invalid product image references" "$merchant_one
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,999999,1);
"

expect_preflight_failure wrong-biz "file binding preflight: invalid product image references" "$merchant_one
  INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status)
  VALUES (311,'MERCHANT_LICENSE','f02/wrong-biz','/uploads/wrong-biz.jpg','image/jpeg',22,'PUBLIC','PASS');
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,311,1);
"

expect_preflight_failure non-pass "file binding preflight: invalid product image references" "$merchant_one
  INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status)
  VALUES (312,'PRODUCT_IMAGE','f02/non-pass','/uploads/non-pass.jpg','image/jpeg',22,'PUBLIC','PENDING');
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,312,1);
"

expect_preflight_failure empty-url "file binding preflight: invalid product image references" "$merchant_one
  INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status)
  VALUES (313,'PRODUCT_IMAGE','f02/empty-url','','image/jpeg',22,'PUBLIC','PASS');
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,313,1);
"

expect_preflight_failure cross-merchant "file binding preflight: file is referenced by multiple merchants" "$merchant_one
  INSERT INTO merchants (id,merchant_no,merchant_name,contact_name,contact_phone,review_status)
  VALUES (2,'M-F02-2','Merchant Two','Owner Two','13800138002','APPROVED');
  INSERT INTO products (id,product_no,merchant_id,title,description,category_id,price_cent,original_price_cent,condition_level,stock,status,created_by,updated_by,version)
  VALUES (202,'P-F02-2',2,'Fixture Product Two','fixture',102,100,120,'GOOD',1,'DRAFT',22,22,1);
  INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status)
  VALUES (314,'PRODUCT_IMAGE','f02/shared','/uploads/shared.jpg','image/jpeg',22,'PUBLIC','PASS');
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,314,1),(202,314,1);
"

expect_preflight_failure uploader-mismatch "file binding preflight: merchant uploader ownership mismatch" "$merchant_one
  INSERT INTO merchants (id,merchant_no,merchant_name,contact_name,contact_phone,review_status)
  VALUES (2,'M-F02-2','Merchant Two','Owner Two','13800138002','APPROVED');
  INSERT INTO merchant_accounts (id,merchant_id,username,password_hash,role,status)
  VALUES (22,2,'f02_owner_2','test-hash','OWNER','ACTIVE');
  INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status)
  VALUES (315,'PRODUCT_IMAGE','f02/mismatch','/uploads/mismatch.jpg','image/jpeg',22,'MERCHANT',22,'PASS');
  INSERT INTO product_images (product_id,file_id,sort_order) VALUES (201,315,1);
"

# Full chain plus real API registration, product binding, concurrent claim, and AutoMigrate compatibility.
apply_base_chain
run_0006 | tee "$evidence_dir/full-chain.txt"
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

grep -q file_binding_ownership_preflight_passed "$evidence_dir/full-chain.txt"
grep -q file_binding_ownership_postflight_passed "$evidence_dir/full-chain.txt"
grep -q -- '--- PASS: TestFileFlowWithMigrationOnlyMySQL' "$evidence_dir/file-flow.txt"

echo "isolated file binding acceptance passed"
echo "mysql version: $mysql_version"
echo "resources retained for inspection under Compose project: $project_name"
