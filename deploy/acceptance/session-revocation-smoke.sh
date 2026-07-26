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
[[ -f "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}
for command in docker sha256sum sort find xargs mktemp grep cmp tar; do
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
  echo "refusing to overwrite existing session revocation evidence" >&2
  exit 1
}

mkdir -p "$evidence_dir"
chmod 700 "$evidence_dir"
runtime_dir="$(mktemp -d)"

on_exit() {
  local status=$?
  trap - EXIT INT TERM
  if docker container ls -a --filter "label=com.docker.compose.project=$project_name" -q | grep -q .; then
    if [[ "$status" -ne 0 ]]; then
      echo "session revocation acceptance failed; retained service state follows" >&2
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

write_source_file_list() {
  (
    cd "$repo_dir"
    {
      printf '%s\0' Makefile backend/Dockerfile backend/go.mod backend/go.sum
      find backend -type f \( -name '*.go' -o -path 'backend/migrations/*.sql' \) \
        ! -path '*/.cache/*' ! -path '*/uploads/*' ! -name 'app.db' -print0
      find deploy/acceptance -maxdepth 2 -type f \
        \( -name '*.sh' -o -name '*.yml' -o -name '*.conf' -o -name '*.md' -o -name '*.Dockerfile' -o -path 'deploy/acceptance/sql/*.sql' \) \
        ! -name '.env' ! -path '*/secrets/*' ! -path '*/backups/*' ! -path '*/evidence/*' -print0
    } | LC_ALL=C sort -zu
  )
}

snapshot_production "$evidence_dir/production-before.txt"
source_files="$runtime_dir/source-files.z"
write_source_file_list >"$source_files"
(
  cd "$repo_dir"
  xargs -0 sha256sum <"$source_files"
) >"$evidence_dir/source-sha256.txt"

build_context="$runtime_dir/build-context"
mkdir -p "$build_context"
(
  cd "$repo_dir"
  tar --null --files-from="$source_files" -cf -
) | tar -C "$build_context" -xf -
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
    0008_anonymous_upload_governance; do
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
    exit 1
  fi
  grep -E -- '^(=== RUN|--- PASS:|    --- PASS:|PASS$|ok[[:space:]])|status/code =|EXPLAIN access/key =' "$raw" \
    >"$evidence_dir/$label.txt"
  grep -q -- '--- PASS: TestSessionRevocationMySQLAcceptance' "$evidence_dir/$label.txt" || {
    echo "focused session revocation PASS marker is missing" >&2
    exit 1
  }
}

"${compose[@]}" up -d --wait mysql
mysql_version="$(mysql_sql 'SELECT VERSION()')"
printf '%s\n' "$mysql_version" >"$evidence_dir/mysql-version.txt"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated session revocation acceptance requires MySQL 8.4.x" >&2
  exit 1
}

"${compose[@]}" --profile tools build bootstrap-admin

apply_migration_chain
run_focused_test false mysql-auto-migrate-false

apply_migration_chain
run_focused_test true mysql-auto-migrate-true

backend_raw="$runtime_dir/backend-tests.raw"
if ! "${compose[@]}" --profile tools run --rm \
  -e SESSION_REVOCATION_MYSQL_TEST=0 \
  bootstrap-admin go test ./... -count=1 >"$backend_raw" 2>&1; then
  echo "full backend tests failed" >&2
  exit 1
fi
grep -E -- '^(\?|ok[[:space:]])' "$backend_raw" >"$evidence_dir/backend-tests.txt"

if ! "${compose[@]}" --profile tools run --rm \
  -e SESSION_REVOCATION_MYSQL_TEST=0 \
  bootstrap-admin go vet ./... >"$runtime_dir/go-vet.raw" 2>&1; then
  echo "go vet failed" >&2
  exit 1
fi
printf 'go_vet=pass\n' >"$evidence_dir/go-vet.txt"

snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}

if grep -ERn --binary-files=without-match \
  'Authorization|access_token|refresh_token|DB_DSN=|MYSQL_PASSWORD=|MYSQL_ROOT_PASSWORD=|JWT_ACCESS_SECRET=|JWT_REFRESH_SECRET=|FILE_UPLOAD_IP_HASH_SECRET=|eyJ[A-Za-z0-9_-]+\.|openid["=:]|session_id["=:]|user_id["=:]|actor_id["=:]' \
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
echo "isolated session access revocation acceptance passed"
echo "mysql version: $mysql_version"
