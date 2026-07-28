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

write_source_file_list() {
  (
    cd "$repo_dir"
    git ls-files -z -- Makefile backend deploy/acceptance |
      while IFS= read -r -d '' path; do
        source_path_is_forbidden "$path" && continue
        case "$path" in
          Makefile | backend/Dockerfile | backend/go.mod | backend/go.sum | \
            backend/*.go | backend/migrations/*.sql | \
            deploy/acceptance/*.sh | deploy/acceptance/*.yml | \
            deploy/acceptance/*.yaml | deploy/acceptance/*.conf | \
            deploy/acceptance/*.md | deploy/acceptance/*.Dockerfile | \
            deploy/acceptance/sql/*.sql)
            printf '%s\0' "$path"
            ;;
        esac
      done | LC_ALL=C sort -zu
  )
}

write_source_manifest() {
  local output="$1"
  write_source_file_list | (
    cd "$repo_dir"
    xargs -0 sha256sum
  ) >"$output"
}

if [[ "${IDEMPOTENCY_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
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
for command in docker git sha256sum sort xargs mktemp grep cmp tar chmod \
  mkdir rm wc tr cut cat find; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done
if ! write_source_file_list | (
  cd "$repo_dir"
  xargs -0 git diff --quiet HEAD --
); then
  echo "committed idempotency source must match HEAD" >&2
  exit 1
fi
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

runtime_dir="$(mktemp -d)"
evidence_dir="$runtime_dir/evidence"
mkdir -p "$evidence_dir"
chmod 700 "$runtime_dir" "$evidence_dir"

on_exit() {
  local status="${1:-$?}"
  trap - EXIT INT TERM
  set +e
  if [[ "$project_touched" -eq 1 ]]; then
    "${compose[@]}" stop >/dev/null 2>&1 || true
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
trap on_exit EXIT
trap 'on_exit 130' INT
trap 'on_exit 143' TERM

record_pass() {
  local classification="$1"
  local count="$2"
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

source_files="$runtime_dir/source-files.z"
source_manifest="$runtime_dir/source-sha256.txt"
write_source_file_list >"$source_files"
write_source_manifest "$source_manifest"
source_count="$(tr -cd '\0' <"$source_files" | wc -c | tr -d ' ')"
[[ "$source_count" -gt 0 ]] || {
  echo "committed source whitelist is empty" >&2
  exit 1
}

build_context="$runtime_dir/build-context"
mkdir -p "$build_context"
(
  cd "$repo_dir"
  tar --null --files-from="$source_files" -cf -
) | tar -C "$build_context" -xf -
(
  cd "$build_context"
  xargs -0 sha256sum <"$source_files"
) >"$runtime_dir/build-context-sha256.txt"
cmp -s "$source_manifest" "$runtime_dir/build-context-sha256.txt" || {
  echo "temporary build context does not match the committed source manifest" >&2
  exit 1
}
manifest_sha256="$(sha256sum "$source_manifest" | cut -d ' ' -f1)"
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
      - "$build_context:/workspace:ro"
    networks:
      - acceptance
    depends_on:
      mysql:
        condition: service_healthy
EOF
compose+=(--file "$compose_override")

snapshot_production "$evidence_dir/production-before.txt"

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

project_touched=1
"${compose[@]}" up -d --wait mysql
mysql_version="$("${compose[@]}" exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="SELECT VERSION()"
')"
[[ "$mysql_version" == 8.4.* ]] || {
  echo "isolated idempotency acceptance requires MySQL 8.4.x" >&2
  exit 1
}
record_pass mysql_8_4 1

"${compose[@]}" --profile tools build idempotency-test >/dev/null
git -C "$build_context" init -q
xargs -0 git -C "$build_context" add -- <"$source_files"

disabled_mysql_opts=(
  -e IDEMPOTENCY_MYSQL_TEST=0
  -e BUYER_INTENT_MYSQL_TEST=0
  -e FILE_SCHEMA_MYSQL_TEST=0
  -e SESSION_REVOCATION_MYSQL_TEST=0
  -e UPLOAD_GOVERNANCE_MYSQL_TEST=0
)

apply_migration_chain
run_focused_test false mysql_auto_migrate_false
record_pass migrations_0001_through_0009_false 9

apply_migration_chain
run_focused_test true mysql_auto_migrate_true
record_pass migrations_0001_through_0009_true 9

backend_raw="$runtime_dir/backend-tests.raw"
if ! "${compose[@]}" --profile tools run --rm --no-deps \
  "${disabled_mysql_opts[@]}" \
  idempotency-test go test ./... -count=1 >"$backend_raw" 2>&1; then
  echo "go test ./... failed" >&2
  exit 1
fi
record_pass backend_tests "$(grep -Ec '^(ok|\?)[[:space:]]' "$backend_raw")"

race_raw="$runtime_dir/backend-race.raw"
if ! "${compose[@]}" --profile tools run --rm --no-deps \
  "${disabled_mysql_opts[@]}" \
  idempotency-test go test -race ./internal/app ./tests -count=1 >"$race_raw" 2>&1; then
  echo "go test -race ./internal/app ./tests failed" >&2
  exit 1
fi
record_pass backend_race "$(grep -Ec '^ok[[:space:]]' "$race_raw")"

if ! "${compose[@]}" --profile tools run --rm --no-deps \
  "${disabled_mysql_opts[@]}" \
  idempotency-test go vet ./... >"$runtime_dir/go-vet.raw" 2>&1; then
  echo "go vet ./... failed" >&2
  exit 1
fi
record_pass go_vet 1

snapshot_production "$evidence_dir/production-after.txt"
cmp -s "$evidence_dir/production-before.txt" "$evidence_dir/production-after.txt" || {
  echo "production container identity, state, or restart count changed" >&2
  exit 1
}
record_pass production_snapshot_unchanged 3

"${compose[@]}" stop >/dev/null
project_touched=0
record_pass isolated_containers_stopped 1

evidence_scan_status=0
grep -ERn --binary-files=text \
  'Authorization|Bearer[[:space:]]|access_token|refresh_token|token["=:]|password["=:]|DB_DSN=|MYSQL_(DATABASE|USER|PASSWORD|ROOT_PASSWORD)=|JWT_(ACCESS|REFRESH)_SECRET=|Idempotency-Key|idem_key["=:]|request_hash["=:]|response_raw["=:]|openid["=:]|buyer_id["=:]|merchant_id["=:]|operator_id["=:]|actor_id["=:]|session_id["=:]|intent_no["=:]|contact_(phone|wechat|name)["=:]|TestIdempotency|f15-[[:alnum:]_-]+' \
  "$evidence_dir" >"$runtime_dir/evidence-leaks.txt" || evidence_scan_status=$?
case "$evidence_scan_status" in
  0)
    echo "sanitized evidence check found a forbidden secret, identifier, or payload field" >&2
    exit 1
    ;;
  1)
    ;;
  *)
    echo "sanitized evidence scan failed" >&2
    exit 1
    ;;
esac
printf 'classification=evidence_scan|result=PASS|count=0\n' \
  >"$evidence_dir/evidence-leak-scan.txt"

(
  cd "$evidence_dir"
  find . -type f ! -name 'evidence-sha256.txt' -print0 |
    LC_ALL=C sort -z | xargs -0 sha256sum
) >"$evidence_dir/evidence-sha256.txt"

mkdir -p "$retained_evidence_dir"
chmod 700 "$retained_evidence_dir"
(
  cd "$evidence_dir"
  tar -cf - .
) | tar -C "$retained_evidence_dir" -xf -

success=1
echo "isolated idempotency atomicity acceptance passed"
