#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-buyer-intent-acceptance"
evidence_dir="$base_dir/evidence/buyer-intent-open-uniqueness"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(
  secondhand-market-api
  secondhand-market-web
  secondhand-market-mysql
)
runtime_dir=""
success=0

write_source_file_list() {
  (
    cd "$repo_dir"
    {
      printf '%s\0' Makefile backend/Dockerfile backend/go.mod backend/go.sum
      find backend -type f -name '*.go' \
        ! -path '*/.cache/*' ! -path '*/uploads/*' ! -name 'app.db' -print0
      find backend/migrations -maxdepth 1 -type f -name '*.sql' -print0
      find deploy/acceptance -maxdepth 2 -type f \
        \( -name '*.sh' -o -name '*.yml' -o -name '*.yaml' \
          -o -name '*.conf' -o -name '*.md' -o -name '*.Dockerfile' \
          -o -path 'deploy/acceptance/sql/*.sql' \) \
        ! -name '.env' ! -name '.env.*' ! -path '*/secrets/*' \
        ! -path '*/backups/*' ! -path '*/evidence/*' -print0
    } | LC_ALL=C sort -zu
  )
}

write_source_manifest() {
  local output="$1"
  if [[ "$output" == "/dev/stdout" ]]; then
    write_source_file_list | (
      cd "$repo_dir"
      xargs -0 sha256sum
    )
    return
  fi
  write_source_file_list | (
    cd "$repo_dir"
    xargs -0 sha256sum
  ) >"$output"
}

if [[ "${BUYER_INTENT_SOURCE_LIST_ONLY:-0}" == "1" &&
      "${BUYER_INTENT_SOURCE_MANIFEST_ONLY:-0}" == "1" ]]; then
  echo "choose one read-only source mode" >&2
  exit 1
fi
if [[ "${BUYER_INTENT_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
  exit 0
fi
if [[ "${BUYER_INTENT_SOURCE_MANIFEST_ONLY:-0}" == "1" ]]; then
  write_source_manifest /dev/stdout
  exit 0
fi

[[ "${BUYER_INTENT_ACCEPTANCE_CONFIRM:-}" == \
  "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA" ]] || {
  echo "isolated buyer intent confirmation is missing" >&2
  exit 1
}
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || {
  echo "ACCEPTANCE_DB_ENGINE=mysql8.4 is required" >&2
  exit 1
}
[[ -z "${COMPOSE_PROJECT_NAME:-}" || "$COMPOSE_PROJECT_NAME" == "$project_name" ]] || {
  echo "COMPOSE_PROJECT_NAME must be $project_name when set" >&2
  exit 1
}
[[ "$project_name" == "secondhand-buyer-intent-acceptance" ]] || {
  echo "unexpected buyer intent Compose project" >&2
  exit 1
}
[[ -f "$base_dir/.env" && ! -L "$base_dir/.env" ]] || {
  echo "generate the remote acceptance .env with deploy/acceptance/prepare.sh" >&2
  exit 1
}
[[ -f "$base_dir/secrets/control-admin-password" &&
   ! -L "$base_dir/secrets/control-admin-password" ]] || {
  echo "generate the remote acceptance secret with deploy/acceptance/prepare.sh" >&2
  exit 1
}
for command in docker sha256sum sort find xargs mktemp grep cmp tar jq chmod \
  mkdir rm wc tail; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done
grep -qx 'MYSQL_DATABASE=second_hand_market_acceptance' "$base_dir/.env" || {
  echo "remote-generated .env must select second_hand_market_acceptance" >&2
  exit 1
}

existing_containers="$(docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}
[[ ! -e "$evidence_dir" ]] || {
  echo "refusing to overwrite existing buyer intent evidence" >&2
  exit 1
}

mkdir -p "$evidence_dir"
chmod 700 "$evidence_dir"
runtime_dir="$(mktemp -d)"

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  if docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q | grep -q .; then
    "${compose[@]}" stop >/dev/null 2>&1 || true
  fi
  if [[ -n "$runtime_dir" && -d "$runtime_dir" ]]; then
    rm -r -- "$runtime_dir"
  fi
  if [[ "$success" -eq 1 ]]; then
    echo "resources retained for inspection under Compose project: $project_name"
  else
    echo "buyer intent acceptance stopped; project resources and evidence were retained" >&2
  fi
  exit "$status"
}
trap on_exit EXIT
trap 'on_exit 130' INT
trap 'on_exit 143' TERM

snapshot_production() {
  local output="$1"
  : >"$output"
  for container in "${production_containers[@]}"; do
    if docker inspect --type container \
      --format '{{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}}' \
      "$container" >>"$output" 2>/dev/null; then
      :
    else
      printf '/%s|absent|absent|absent\n' "$container" >>"$output"
    fi
  done
}

source_files="$runtime_dir/source-files.z"
write_source_file_list >"$source_files"
write_source_manifest "$evidence_dir/source-sha256.txt"

build_context="$runtime_dir/build-context"
mkdir -p "$build_context"
(
  cd "$repo_dir"
  tar --null --files-from="$source_files" -cf -
) | tar -C "$build_context" -xf -

compose_override="$runtime_dir/buyer-intent-compose.yml"
printf 'services:\n  bootstrap-admin:\n    build:\n      context: "%s"\n      dockerfile: backend/Dockerfile\n    working_dir: /workspace/backend\n    volumes:\n      - "%s:/workspace:ro"\n' \
  "$build_context" "$build_context" >"$compose_override"
compose+=(--file "$compose_override")

compose_structure="$runtime_dir/compose-structure.json"
"${compose[@]}" config --no-interpolate --format json >"$compose_structure"
jq -e --arg source "$build_context" \
  --arg migrations "$repo_dir/backend/migrations" \
  --arg secret "$base_dir/secrets/control-admin-password" '
  .services["bootstrap-admin"].working_dir == "/workspace/backend" and
  .services["bootstrap-admin"].build.context == $source and
  any(.services["bootstrap-admin"].volumes[]?;
    .target == "/workspace" and .source == $source and .read_only == true) and
  any(.services["bootstrap-admin"].volumes[]?;
    .target == "/run/secrets/admin-password" and .read_only == true and
    (.source == $secret or (.source | contains("secrets/control-admin-password")))) and
  any(.services.mysql.volumes[]?;
    .target == "/acceptance/migrations" and .source == $migrations and
    .read_only == true) and
  (.services.mysql.environment.MYSQL_DATABASE | startswith("${MYSQL_DATABASE")) and
  (.services.mysql.environment.MYSQL_USER | startswith("${MYSQL_USER")) and
  (.services.mysql.environment.MYSQL_PASSWORD | startswith("${MYSQL_PASSWORD")) and
  (.services.mysql.environment.MYSQL_ROOT_PASSWORD | startswith("${MYSQL_ROOT_PASSWORD")) and
  (.services.api.environment.DB_DSN | contains("${MYSQL_PASSWORD")) and
  (.services.api.environment.DB_DSN | contains("${MYSQL_USER")) and
  (.services.api.environment.JWT_ACCESS_SECRET | startswith("${JWT_ACCESS_SECRET")) and
  (.services.api.environment.JWT_REFRESH_SECRET | startswith("${JWT_REFRESH_SECRET")) and
  (.services.api.environment.FILE_UPLOAD_IP_HASH_SECRET | startswith("${FILE_UPLOAD_IP_HASH_SECRET")) and
  (.services["bootstrap-admin"].environment.DB_DSN | contains("${MYSQL_PASSWORD")) and
  (.services["bootstrap-admin"].environment.DB_DSN | contains("${MYSQL_USER"))
' "$compose_structure" >/dev/null || {
  echo "Compose structure did not preserve the isolated read-only source mount or unresolved credentials" >&2
  exit 1
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
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names < "$1"
  ' sh "$container_path"
}

reset_schema() {
  mysql_sql '
    SET FOREIGN_KEY_CHECKS=0;
    DROP TABLE IF EXISTS file_quota_guards, buyer_intents, buyer_histories,
      buyer_favorites, buyer_device_bindings, buyer_users, idempotency_records,
      auth_sessions, operation_logs, file_records, files, order_events, orders,
      product_images, products, categories, merchant_audit_logs, admin_users,
      merchant_accounts, merchants;
    SET FOREIGN_KEY_CHECKS=1;
  '
}

apply_chain_0001_0008() {
  mysql_file /acceptance/migrations/0001_init.up.sql
  mysql_file /acceptance/migrations/0002_buyer_domain.up.sql
  mysql_file /acceptance/migrations/0003_buyer_auth_provider.up.sql
  local migration
  for migration in 0004_merchant_multi_stock 0005_file_records_table \
    0006_file_binding_ownership 0007_license_file_privacy \
    0008_anonymous_upload_governance; do
    mysql_file "/acceptance/migrations/$migration.preflight.sql"
    mysql_file "/acceptance/migrations/$migration.up.sql"
    mysql_file "/acceptance/migrations/$migration.postflight.sql"
  done
}

run_0009() {
  mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
  mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql
  mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
}

capture_intent_summary() {
  local output="$1"
  mysql_sql "
    SET SESSION group_concat_max_len = 1048576;
    SELECT CONCAT('row_count=', COUNT(*)) FROM buyer_intents;
    SELECT SHA2(COALESCE(GROUP_CONCAT(row_hash ORDER BY id SEPARATOR ''), ''), 256)
    FROM (
      SELECT id, SHA2(CAST(JSON_ARRAY(
        id, intent_no, buyer_id, source_device_id, product_id, merchant_id,
        status, is_open, contact_name, contact_phone, contact_wechat, message,
        handled_by, handled_at, closed_at, close_reason, merchant_note,
        created_at, updated_at
      ) AS CHAR), 256) AS row_hash
      FROM buyer_intents
    ) AS ordered_row_hashes;
  " >"$output"
  [[ "$(wc -l <"$output")" -eq 2 ]] &&
    grep -Eq '^row_count=[0-9]+$' "$output" &&
    tail -n 1 "$output" | grep -Eq '^[0-9a-f]{64}$' || {
    echo "buyer intent row summary is empty or malformed" >&2
    exit 1
  }
}

require_45000_unchanged() {
  local label="$1"
  local gate="$2"
  local before="$runtime_dir/$label-before-summary.txt"
  local after="$runtime_dir/$label-after-summary.txt"
  local failure="$runtime_dir/$label-error.txt"
  capture_intent_summary "$before"
  if mysql_file "$gate" >"$runtime_dir/$label-output.txt" 2>"$failure"; then
    echo "$label unexpectedly passed its rejection gate" >&2
    exit 1
  fi
  grep -Eq 'ERROR 1644 \(45000\)' "$failure" || {
    echo "$label did not fail with ERROR 1644 (45000)" >&2
    exit 1
  }
  capture_intent_summary "$after"
  cmp -s "$before" "$after" || {
    echo "$label changed buyer intent rows during rejection" >&2
    exit 1
  }
  {
    printf 'case=%s\n' "$label"
    printf 'expected_error=ERROR 1644 (45000)\n'
    printf 'before/after row-summary comparisons=identical\n'
    cat "$before"
  } >"$evidence_dir/$label.txt"
}

prepare_legacy_schema() {
  reset_schema
  apply_chain_0001_0008
}

add_exact_marker() {
  mysql_sql '
    ALTER TABLE buyer_intents
      ADD COLUMN open_marker TINYINT
        GENERATED ALWAYS AS (
          CASE WHEN is_open = 1 THEN 1 ELSE NULL END
        ) STORED AFTER is_open;
  '
}

compare_successful_0009() {
  local label="$1"
  local before="$runtime_dir/$label-before-summary.txt"
  local after="$runtime_dir/$label-after-summary.txt"
  local gates="$runtime_dir/$label-gates.txt"
  capture_intent_summary "$before"
  run_0009 >"$gates"
  capture_intent_summary "$after"
  cmp -s "$before" "$after" || {
    echo "$label changed buyer intent rows" >&2
    exit 1
  }
  grep -qx 'buyer_intent_open_uniqueness_preflight_passed' "$gates"
  grep -qx 'buyer_intent_open_uniqueness_migration_applied' "$gates"
  grep -qx 'buyer_intent_open_uniqueness_postflight_passed' "$gates"
  {
    printf 'case=%s\n' "$label"
    printf '0009 preflight/up/postflight=pass\n'
    printf 'before/after row-summary comparisons=identical\n'
    cat "$before"
  } >"$evidence_dir/$label.txt"
}

capture_schema_summary() {
  local output="$1"
  mysql_sql "
    SELECT CONCAT('column=', column_name, '|type=', column_type,
      '|nullable=', is_nullable, '|extra=', extra)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND column_name = 'open_marker';
    SELECT CONCAT('index=', index_name, '|columns=',
      GROUP_CONCAT(column_name ORDER BY seq_in_index))
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND non_unique = 0
      AND index_name IN ('uk_buyer_product_open', 'uk_buyer_intent_open')
    GROUP BY index_name
    ORDER BY index_name;
  " >"$output"
  grep -Fx 'column=open_marker|type=tinyint|nullable=YES|extra=STORED GENERATED' "$output" >/dev/null
  grep -Fx 'index=uk_buyer_intent_open|columns=buyer_id,product_id,open_marker' "$output" >/dev/null
  [[ "$(wc -l <"$output")" -eq 2 ]] || {
    echo "final buyer intent schema summary is incomplete or drifted" >&2
    exit 1
  }
}

"${compose[@]}" up -d --wait mysql

# MySQL 8.4 version check
mysql_version="$(mysql_sql 'SELECT VERSION()')"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated buyer intent acceptance requires MySQL 8.4.x" >&2
  exit 1
}
printf 'mysql_version=%s\n' "$mysql_version" >"$evidence_dir/mysql-version.txt"

# legacy
prepare_legacy_schema
mysql_sql "
  INSERT INTO buyer_intents
    (intent_no, buyer_id, product_id, merchant_id, status, is_open)
  VALUES
    ('F11LEGACYOPEN', 1101, 2101, 3101, 'NEW', 1),
    ('F11LEGACYCLOSED', 1102, 2102, 3102, 'CLOSED', 0);
"
compare_successful_0009 legacy

# marker-only
prepare_legacy_schema
add_exact_marker
compare_successful_0009 marker-only

# both-keys
prepare_legacy_schema
add_exact_marker
mysql_sql '
  ALTER TABLE buyer_intents
    ADD UNIQUE KEY uk_buyer_intent_open (buyer_id, product_id, open_marker);
'
compare_successful_0009 both-keys

# final-rerun
prepare_legacy_schema
final_before="$runtime_dir/final-rerun-before-summary.txt"
final_after="$runtime_dir/final-rerun-after-summary.txt"
capture_intent_summary "$final_before"
run_0009 >"$runtime_dir/final-rerun-first-gates.txt"
run_0009 >"$runtime_dir/final-rerun-second-gates.txt"
capture_intent_summary "$final_after"
cmp -s "$final_before" "$final_after"
for rerun_gates in "$runtime_dir/final-rerun-first-gates.txt" \
  "$runtime_dir/final-rerun-second-gates.txt"; do
  [[ "$(grep -Ec '^buyer_intent_open_uniqueness_.*passed$|^buyer_intent_open_uniqueness_migration_applied$' \
    "$rerun_gates")" -eq 3 ]] || {
    echo "final-rerun did not pass all 0009 gates twice" >&2
    exit 1
  }
done
{
  printf 'case=final-rerun\n'
  printf '0009_first_run=pass\n0009_second_run=pass\n'
  printf 'before/after row-summary comparisons=identical\n'
  cat "$final_before"
} >"$evidence_dir/final-rerun.txt"
capture_schema_summary "$evidence_dir/final-schema.txt"

# invalid-state cases are independent: NEW/false, CONTACTED/false,
# CLOSED/true, BOGUS/false, and BOGUS/true.
invalid_states=(
  'new-false|NEW|0'
  'contacted-false|CONTACTED|0'
  'closed-true|CLOSED|1'
  'bogus-false|BOGUS|0'
  'bogus-true|BOGUS|1'
)
for fixture in "${invalid_states[@]}"; do
  IFS='|' read -r fixture_label fixture_status fixture_open <<<"$fixture"
  prepare_legacy_schema
  mysql_sql "
    INSERT INTO buyer_intents
      (intent_no, buyer_id, product_id, merchant_id, status, is_open)
    VALUES
      ('F11INVALID', 1201, 2201, 3201, '$fixture_status', $fixture_open);
  "
  require_45000_unchanged "invalid-state-$fixture_label" \
    /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
done

# duplicate-open
prepare_legacy_schema
mysql_sql "
  ALTER TABLE buyer_intents DROP INDEX uk_buyer_product_open;
  INSERT INTO buyer_intents
    (intent_no, buyer_id, product_id, merchant_id, status, is_open)
  VALUES
    ('F11DUPLICATEA', 1301, 2301, 3301, 'NEW', 1),
    ('F11DUPLICATEB', 1301, 2301, 3301, 'CONTACTED', 1);
"
require_45000_unchanged duplicate-open \
  /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql

# drifted-marker
prepare_legacy_schema
mysql_sql 'ALTER TABLE buyer_intents ADD COLUMN open_marker TINYINT NULL;'
require_45000_unchanged drifted-marker \
  /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql

# drifted-key
prepare_legacy_schema
add_exact_marker
mysql_sql '
  ALTER TABLE buyer_intents
    ADD UNIQUE KEY uk_buyer_intent_open (product_id, buyer_id, open_marker);
'
require_45000_unchanged drifted-key \
  /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql

# unknown-partial
prepare_legacy_schema
add_exact_marker
mysql_sql 'ALTER TABLE buyer_intents DROP INDEX uk_buyer_product_open;'
require_45000_unchanged unknown-partial \
  /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql

apply_full_chain() {
  prepare_legacy_schema
  run_0009
}

apply_full_chain >"$runtime_dir/clean-chain-gates.txt"
capture_schema_summary "$evidence_dir/api-matrix-schema.txt"
"${compose[@]}" --profile tools build bootstrap-admin

apply_full_chain >"$runtime_dir/auto-migrate-false-chain.txt"
auto_false_raw="$runtime_dir/auto-migrate-false.raw"
if ! "${compose[@]}" --profile tools run --rm \
  -e BUYER_INTENT_MYSQL_TEST=1 -e AUTO_MIGRATE=false \
  bootstrap-admin go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v \
  >"$auto_false_raw" 2>&1; then
  echo "buyer intent MySQL test failed for AUTO_MIGRATE=false" >&2
  exit 1
fi
grep -E -- '^(--- PASS: TestBuyerIntentMySQLAcceptance|    --- PASS:|PASS$|ok[[:space:]])|status/codes =|history/open counts =' \
  "$auto_false_raw" >"$evidence_dir/mysql-auto-migrate-false.txt"
grep -q -- '--- PASS: TestBuyerIntentMySQLAcceptance' \
  "$evidence_dir/mysql-auto-migrate-false.txt"

apply_full_chain >"$runtime_dir/auto-migrate-true-chain.txt"
auto_true_raw="$runtime_dir/auto-migrate-true.raw"
if ! "${compose[@]}" --profile tools run --rm \
  -e BUYER_INTENT_MYSQL_TEST=1 -e AUTO_MIGRATE=true \
  bootstrap-admin go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v \
  >"$auto_true_raw" 2>&1; then
  echo "buyer intent MySQL test failed for AUTO_MIGRATE=true" >&2
  exit 1
fi
grep -E -- '^(--- PASS: TestBuyerIntentMySQLAcceptance|    --- PASS:|PASS$|ok[[:space:]])|status/codes =|history/open counts =' \
  "$auto_true_raw" >"$evidence_dir/mysql-auto-migrate-true.txt"
grep -q -- '--- PASS: TestBuyerIntentMySQLAcceptance' \
  "$evidence_dir/mysql-auto-migrate-true.txt"

backend_raw="$runtime_dir/backend-tests.raw"
if ! "${compose[@]}" --profile tools run --rm \
  -e BUYER_INTENT_MYSQL_TEST=0 \
  bootstrap-admin go test ./... -count=1 >"$backend_raw" 2>&1; then
  echo "go test ./... failed" >&2
  exit 1
fi
grep -E -- '^(\?|ok[[:space:]])' "$backend_raw" >"$evidence_dir/backend-tests.txt"

race_raw="$runtime_dir/backend-race.raw"
if ! "${compose[@]}" --profile tools run --rm \
  -e BUYER_INTENT_MYSQL_TEST=0 \
  bootstrap-admin go test -race ./... -count=1 >"$race_raw" 2>&1; then
  echo "go test -race ./... failed" >&2
  exit 1
fi
grep -E -- '^(\?|ok[[:space:]])' "$race_raw" >"$evidence_dir/backend-race.txt"

if ! "${compose[@]}" --profile tools run --rm \
  -e BUYER_INTENT_MYSQL_TEST=0 \
  bootstrap-admin go vet ./... >"$runtime_dir/go-vet.raw" 2>&1; then
  echo "go vet ./... failed" >&2
  exit 1
fi
printf 'go_vet=pass\n' >"$evidence_dir/go-vet.txt"

snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}

printf 'resource retention marker: project=%s; resources=retained\n' \
  "$project_name" >"$evidence_dir/resource-retention.txt"

if grep -ERn --binary-files=without-match 'Authorization|access_token|refresh_token|DB_DSN=|MYSQL_PASSWORD=|MYSQL_ROOT_PASSWORD=|JWT_ACCESS_SECRET=|JWT_REFRESH_SECRET=|openid["=:]|buyer_id["=:][[:space:]]*[0-9]|merchant_id["=:][[:space:]]*[0-9]|actor_id["=:][[:space:]]*[0-9]|session_id["=:][[:space:]]*[0-9]|intent_no["=:]|contact_(phone|wechat|name)["=:]|eyJ[A-Za-z0-9_-]+\.' \
  "$evidence_dir" >"$runtime_dir/evidence-leaks.txt"; then
  echo "sanitized evidence check found a forbidden secret or identifier" >&2
  exit 1
fi
printf 'forbidden_matches=0\n' >"$evidence_dir/evidence-leak-scan.txt"

(
  cd "$evidence_dir"
  find . -maxdepth 1 -type f -name '*.txt' ! -name 'evidence-sha256.txt' -print0 \
    | LC_ALL=C sort -z | xargs -0 sha256sum
) >"$evidence_dir/evidence-sha256.txt"

success=1
echo "isolated buyer intent open uniqueness acceptance passed"
echo "mysql version: $mysql_version"
