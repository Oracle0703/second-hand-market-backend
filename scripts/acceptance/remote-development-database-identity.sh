#!/usr/bin/env bash
set -Eeuo pipefail
set +x
umask 077

script_relative_path='scripts/acceptance/remote-development-database-identity.sh'
runtime_dir=''
build_dir=''
evidence_dir=''
evidence_is_temporary=0
summary=''
temp_root=''
resource_nonce=''
container_name=''
container_label_key='second-hand-market.issue9.acceptance'
container_id=''
cleanup_complete=0
container_engine=()
engine_timeout='30s'
focused_passed=0
focused_failed=0
focused_skipped=0
mysql_passed=0
mysql_failed=0
mysql_skipped=0

fail() {
  if [[ -n "$summary" && -f "$summary" ]] &&
    ! grep -q '^status=' "$summary" 2>/dev/null; then
    printf 'status=FAIL\n' >>"$summary" || true
  fi
  printf 'remote-development-database-identity acceptance: FAIL: %s\n' "$*" >&2
  exit 1
}

run_engine() {
  timeout --foreground "$engine_timeout" "${container_engine[@]}" "$@"
}

run_engine_quiet() {
  run_engine "$@" >/dev/null 2>&1
}

container_matches_run() {
  local candidate_id="$1"
  local observed_label
  local observed_name
  [[ "$candidate_id" =~ ^[0-9a-f]{12,64}$ ]] || return 1
  observed_label="$(
    run_engine inspect \
      --format "{{ index .Config.Labels \"$container_label_key\" }}" \
      "$candidate_id" 2>/dev/null
  )" || return 1
  observed_name="$(
    run_engine inspect --format '{{.Name}}' "$candidate_id" 2>/dev/null
  )" || return 1
  observed_name="${observed_name#/}"
  [[ "$observed_label" == "$resource_nonce" && "$observed_name" == "$container_name" ]]
}

cleanup_container() {
  local listed_ids
  local known_ids=''
  local candidate_id
  local known_id
  local candidate_was_listed
  local -a candidate_ids=()

  [[ "${#container_engine[@]}" -gt 0 ]] || return 0
  [[ -n "$resource_nonce" && -n "$container_name" ]] || return 0
  run_engine_quiet info || return 1

  listed_ids="$(
    run_engine ps -aq --no-trunc \
      --filter "label=$container_label_key=$resource_nonce" 2>/dev/null
  )" || return 1
  while IFS= read -r candidate_id; do
    [[ -n "$candidate_id" ]] || continue
    [[ "$candidate_id" =~ ^[0-9a-f]{12,64}$ ]] || return 1
    candidate_ids+=("$candidate_id")
  done <<<"$listed_ids"

  if [[ -n "$container_id" ]]; then
    [[ "$container_id" =~ ^[0-9a-f]{12,64}$ ]] || return 1
    known_ids="$(
      run_engine ps -aq --no-trunc \
        --filter "id=$container_id" 2>/dev/null
    )" || return 1
    while IFS= read -r known_id; do
      [[ -n "$known_id" ]] || continue
      [[ "$known_id" =~ ^[0-9a-f]{12,64}$ ]] || return 1
      [[ "$known_id" == "$container_id" ]] || return 1
    done <<<"$known_ids"
    if [[ -n "$known_ids" ]]; then
      candidate_was_listed=0
      for candidate_id in "${candidate_ids[@]}"; do
        if [[ "$candidate_id" == "$container_id" ]]; then
          candidate_was_listed=1
          break
        fi
      done
      ((candidate_was_listed == 1)) || return 1
    elif ((${#candidate_ids[@]} > 0)); then
      return 1
    fi
    for candidate_id in "${candidate_ids[@]}"; do
      [[ "$candidate_id" == "$container_id" ]] || return 1
    done
  fi

  for candidate_id in "${candidate_ids[@]}"; do
    container_matches_run "$candidate_id" || return 1
    run_engine_quiet rm -f -v "$candidate_id" || return 1
    known_ids="$(
      run_engine ps -aq --no-trunc \
        --filter "id=$candidate_id" 2>/dev/null
    )" || return 1
    [[ -z "$known_ids" ]] || return 1
  done

  listed_ids="$(
    run_engine ps -aq --no-trunc \
      --filter "label=$container_label_key=$resource_nonce" 2>/dev/null
  )" || return 1
  [[ -z "$listed_ids" ]] || return 1
  if [[ -n "$container_id" ]]; then
    known_ids="$(
      run_engine ps -aq --no-trunc \
        --filter "id=$container_id" 2>/dev/null
    )" || return 1
    [[ -z "$known_ids" ]] || return 1
  fi
  return 0
}

cleanup_directory() {
  directory="$1"
  [[ -n "$directory" ]] || return 0
  [[ -n "$temp_root" ]] || return 1
  case "$directory" in
    "$temp_root"/issue9-*) ;;
    *) return 1 ;;
  esac
  rm -rf -- "$directory" || return 1
  [[ ! -e "$directory" ]]
}

cleanup_resources() {
  cleanup_failed=0
  cleanup_container || cleanup_failed=1
  if cleanup_directory "$runtime_dir"; then
    runtime_dir=''
  else
    cleanup_failed=1
  fi
  if cleanup_directory "$build_dir"; then
    build_dir=''
  else
    cleanup_failed=1
  fi
  [[ "$cleanup_failed" -eq 0 ]]
}

on_exit() {
  status=$?
  trap - EXIT
  if [[ "$cleanup_complete" -ne 1 ]]; then
    if ! cleanup_resources; then
      printf '%s\n' \
        'remote-development-database-identity acceptance: FAIL: isolated resource cleanup failed' \
        >&2
      status=1
    fi
  fi
  if [[ "$evidence_is_temporary" -eq 1 && -n "$evidence_dir" ]]; then
    case "$evidence_dir" in
      "$temp_root"/issue9-evidence-*)
        rm -rf -- "$evidence_dir" >/dev/null 2>&1 || status=1
        ;;
      *) status=1 ;;
    esac
  elif [[ "$status" -ne 0 && -n "$evidence_dir" ]]; then
    printf 'evidence_dir=%s\n' "$evidence_dir" >&2
  fi
  exit "$status"
}
trap on_exit EXIT

for command_name in \
  git go timeout openssl awk grep realpath mktemp tr chmod rm sleep dirname mkdir; do
  command -v "$command_name" >/dev/null 2>&1 ||
    fail "$command_name is required"
done

script_path="$(realpath "${BASH_SOURCE[0]}")" ||
  fail "could not resolve the acceptance script path"
script_dir="$(dirname "$script_path")"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel 2>/dev/null)" ||
  fail "the acceptance script is not inside a Git checkout"
repo_root="$(realpath "$repo_root")"
[[ "$script_path" == "$repo_root/$script_relative_path" ]] ||
  fail "the acceptance script path does not belong to the tested checkout"
git -C "$repo_root" ls-files --error-unmatch "$script_relative_path" >/dev/null 2>&1 ||
  fail "the acceptance script must be tracked by the tested commit"

expected_sha="${EXPECTED_COMMIT_SHA:-}"
[[ "$expected_sha" =~ ^[0-9a-f]{40}$ ]] ||
  fail "EXPECTED_COMMIT_SHA must be the full 40-character commit SHA"
actual_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$actual_sha" == "$expected_sha" ]] ||
  fail "checked out commit does not match EXPECTED_COMMIT_SHA"
if git -C "$repo_root" symbolic-ref -q HEAD >/dev/null 2>&1; then
  fail "acceptance must run from a detached immutable HEAD"
fi
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "worktree must be clean so the tested source matches the commit"

command_timeout="${ACCEPTANCE_COMMAND_TIMEOUT:-20m}"
[[ "$command_timeout" =~ ^[1-9][0-9]*[smhd]$ ]] ||
  fail "ACCEPTANCE_COMMAND_TIMEOUT must look like 20m, 2h, or 900s"

temp_root="${TMPDIR:-/tmp}"
[[ "$temp_root" == /* && -d "$temp_root" && -w "$temp_root" ]] ||
  fail "TMPDIR must be an absolute writable local directory"
temp_root="$(realpath "$temp_root")"
case "$temp_root/" in
  "$repo_root/"*) fail "TMPDIR must be outside the Git worktree" ;;
esac

if command -v docker >/dev/null 2>&1; then
  docker_context="$(
    timeout --foreground 10s docker context show 2>/dev/null
  )" || fail "could not resolve the Docker context"
  docker_endpoint="$(
    timeout --foreground 10s docker context inspect "$docker_context" \
      --format '{{(index .Endpoints "docker").Host}}' 2>/dev/null
  )" || fail "could not inspect the Docker endpoint"
  [[ "$docker_endpoint" == unix:///* ]] ||
    fail "the Docker engine must use a local Unix socket"
  docker_socket="${docker_endpoint#unix://}"
  [[ -S "$docker_socket" ]] ||
    fail "the Docker Unix socket is unavailable"
  container_engine=(docker --context "$docker_context")
elif command -v podman >/dev/null 2>&1; then
  for remote_variable in CONTAINER_HOST CONTAINER_CONNECTION; do
    remote_value="${!remote_variable:-}"
    if [[ -n "$remote_value" && "$remote_value" != unix://* ]]; then
      fail "the Podman engine must use a local Unix socket"
    fi
  done
  default_connection="$(
    timeout --foreground 10s podman system connection list \
      --format '{{if .Default}}{{.URI}}{{end}}' 2>/dev/null |
      awk 'NF { print; exit }'
  )" || fail "could not inspect the Podman connection"
  if [[ -n "$default_connection" && "$default_connection" != unix://* ]]; then
    fail "the Podman engine must use a local Unix socket"
  fi
  container_engine=(podman)
else
  fail "Docker or Podman is required for isolated MySQL acceptance"
fi
run_engine_quiet info ||
  fail "the selected local container engine is unavailable"

mysql_image='mysql:8.4@sha256:be18eb9dc45eea9b86cb74f8c68ab92ce8569ecc37ea4e6fade02e37036c5ff4'
run_engine_quiet image inspect "$mysql_image" ||
  fail "the immutable MySQL 8.x acceptance image is not available locally"

resource_nonce="i9$(openssl rand -hex 8)"
[[ "$resource_nonce" =~ ^i9[0-9a-f]{16}$ ]] ||
  fail "could not create an isolated resource nonce"
container_name="issue9-mysql-$resource_nonce"

decoy_schema="i9_prod_${resource_nonce}"
wrong_user="i9_wrong_${resource_nonce}"
business_table="i9_business_${resource_nonce}"
migration_table="i9_migration_${resource_nonce}"
bootstrap_table="i9_bootstrap_${resource_nonce}"
seed_table="i9_seed_${resource_nonce}"
crud_table="i9_crud_${resource_nonce}"
denied_create_table="i9_ddl_${resource_nonce}"
decoy_create_table="i9_decoy_ddl_${resource_nonce}"

runtime_dir="$(mktemp -d "$temp_root/issue9-runtime-XXXXXX")" ||
  fail "could not create the isolated runtime directory"
build_dir="$(mktemp -d "$temp_root/issue9-build-XXXXXX")" ||
  fail "could not create the isolated build directory"
if [[ -n "${EVIDENCE_DIR:-}" ]]; then
  [[ "$EVIDENCE_DIR" == /* ]] ||
    fail "EVIDENCE_DIR must be an absolute path outside the worktree"
  mkdir -p -- "$EVIDENCE_DIR"
  evidence_root="$(realpath "$EVIDENCE_DIR")"
  case "$evidence_root/" in
    "$repo_root/"*) fail "EVIDENCE_DIR must be outside the Git worktree" ;;
  esac
  evidence_dir="$(
    mktemp -d \
      "$evidence_root/remote-development-database-identity-${actual_sha:0:12}.XXXXXX"
  )" || fail "could not create the evidence directory"
else
  evidence_dir="$(mktemp -d "$temp_root/issue9-evidence-XXXXXX")" ||
    fail "could not create the temporary evidence directory"
  evidence_is_temporary=1
fi
summary="$evidence_dir/summary.txt"
{
  printf 'commit=%s\n' "$actual_sha"
  printf 'source=clean-detached-head\n'
  printf 'engine=local-unix-socket\n'
  printf 'mysql=isolated-loopback-8.x\n'
  printf 'container_storage=tmpfs\n'
  printf 'persistent_volume=none\n'
} >"$summary"
chmod 0600 "$summary"

root_password_file="$runtime_dir/root-password"
root_client_file="$runtime_dir/root-client.cnf"
app_dsn_file="$runtime_dir/app-dsn"
wrong_user_dsn_file="$runtime_dir/wrong-user-dsn"
server_uuid_file="$runtime_dir/server-uuid"
protected_patterns_file="$runtime_dir/protected-patterns"
provision_sql_file="$runtime_dir/provision.sql"

root_password="$(openssl rand -hex 32)" ||
  fail "could not generate the isolated root credential"
app_password="$(openssl rand -hex 32)" ||
  fail "could not generate the isolated application credential"
wrong_user_password="$(openssl rand -hex 32)" ||
  fail "could not generate the isolated negative-test credential"
printf '%s\n' "$root_password" >"$root_password_file"
{
  printf '[client]\n'
  printf 'user=root\n'
  printf 'password=%s\n' "$root_password"
} >"$root_client_file"
chmod 0600 "$root_password_file" "$root_client_file"

container_id="$(
  run_engine create \
    --name "$container_name" \
    --label "$container_label_key=$resource_nonce" \
    --publish 127.0.0.1:13307:3306 \
    --tmpfs /var/lib/mysql:rw,nosuid,nodev,noexec,size=768m \
    --env MYSQL_ROOT_PASSWORD_FILE=/run/secrets/issue9-root-password \
    --volume "$root_password_file:/run/secrets/issue9-root-password:ro" \
    --volume "$root_client_file:/run/secrets/issue9-root-client.cnf:ro" \
    "$mysql_image" \
    --max-connections=24 \
    --innodb-buffer-pool-size=128M \
    --skip-name-resolve 2>/dev/null
)" || fail "could not create the isolated MySQL container"
[[ "$container_id" =~ ^[0-9a-f]{12,64}$ ]] ||
  fail "the isolated MySQL container returned an invalid identifier"
container_matches_run "$container_id" ||
  fail "the isolated MySQL container identity is invalid"

port_bindings="$(
  run_engine inspect --format '{{json .HostConfig.PortBindings}}' \
    "$container_id" 2>/dev/null
)" || fail "could not inspect the isolated MySQL port binding"
[[ "$port_bindings" == *'"HostIp":"127.0.0.1"'* &&
  "$port_bindings" == *'"HostPort":"13307"'* ]] ||
  fail "isolated MySQL must bind only to loopback port 13307"
tmpfs_config="$(
  run_engine inspect --format '{{json .HostConfig.Tmpfs}}' \
    "$container_id" 2>/dev/null
)" || fail "could not inspect the isolated MySQL storage"
[[ "$tmpfs_config" == *'"/var/lib/mysql"'* ]] ||
  fail "isolated MySQL data storage must be tmpfs"
mount_types="$(
  run_engine inspect --format '{{range .Mounts}}{{println .Type .Destination}}{{end}}' \
    "$container_id" 2>/dev/null
)" || fail "could not inspect the isolated MySQL mounts"
if grep -Eq '^volume[[:space:]]' <<<"$mount_types"; then
  fail "isolated MySQL must not use persistent or anonymous volumes"
fi
unset port_bindings tmpfs_config mount_types

run_engine_quiet start "$container_id" ||
  fail "could not start the isolated MySQL container"
mysql_ready=0
for ((attempt = 0; attempt < 60; attempt++)); do
  if timeout --foreground 5s "${container_engine[@]}" exec "$container_id" \
    mysqladmin \
    --defaults-extra-file=/run/secrets/issue9-root-client.cnf \
    ping --silent >/dev/null 2>&1; then
    mysql_ready=1
    break
  fi
  sleep 1
done
[[ "$mysql_ready" -eq 1 ]] ||
  fail "isolated MySQL did not become ready"
mysql_version="$(
  timeout --foreground "$engine_timeout" \
    "${container_engine[@]}" exec -i "$container_id" \
    mysql \
    --defaults-extra-file=/run/secrets/issue9-root-client.cnf \
    --batch --skip-column-names 2>/dev/null <<'SQL'
SELECT VERSION();
SQL
)" || fail "could not read the isolated MySQL version"
mysql_version="$(printf '%s' "$mysql_version" | tr -d '\r\n')"
[[ "$mysql_version" =~ ^8\.[0-9]+\.[0-9]+ ]] ||
  fail "isolated database is not MySQL 8.x"
unset mysql_version

printf '%s\n' \
"CREATE DATABASE \`second_hand_market_dev\`;
CREATE DATABASE \`${decoy_schema}\`;
CREATE TABLE \`second_hand_market_dev\`.\`${business_table}\` (
  id BIGINT PRIMARY KEY, value VARCHAR(64) NOT NULL
);
CREATE TABLE \`second_hand_market_dev\`.\`${migration_table}\` (
  id BIGINT PRIMARY KEY, value VARCHAR(64) NOT NULL
);
CREATE TABLE \`second_hand_market_dev\`.\`${bootstrap_table}\` (
  id BIGINT PRIMARY KEY, value VARCHAR(64) NOT NULL
);
CREATE TABLE \`second_hand_market_dev\`.\`${seed_table}\` (
  id BIGINT PRIMARY KEY, value VARCHAR(64) NOT NULL
);
CREATE TABLE \`second_hand_market_dev\`.\`${crud_table}\` (
  id BIGINT PRIMARY KEY, value VARCHAR(64) NOT NULL
);
CREATE TABLE \`${decoy_schema}\`.\`${crud_table}\` (
  id BIGINT PRIMARY KEY, value VARCHAR(64) NOT NULL
);
INSERT INTO \`${decoy_schema}\`.\`${crud_table}\` (id, value)
VALUES (1, 'production-decoy');
CREATE USER 'shm_dev_app'@'%' IDENTIFIED BY '${app_password}';
GRANT SELECT, INSERT, UPDATE, DELETE
  ON \`second_hand_market_dev\`.* TO 'shm_dev_app'@'%';
CREATE USER '${wrong_user}'@'%' IDENTIFIED BY '${wrong_user_password}';
GRANT SELECT ON \`second_hand_market_dev\`.\`${crud_table}\`
  TO '${wrong_user}'@'%';" >"$provision_sql_file"
chmod 0600 "$provision_sql_file"
if ! timeout --foreground "$engine_timeout" \
  "${container_engine[@]}" exec -i "$container_id" \
  mysql \
  --defaults-extra-file=/run/secrets/issue9-root-client.cnf \
  --batch --skip-column-names \
  <"$provision_sql_file" >/dev/null 2>&1
then
  fail "could not provision the isolated MySQL acceptance fixture"
fi
rm -f -- "$provision_sql_file" ||
  fail "could not remove the isolated provisioning input"

server_uuid="$(
  timeout --foreground "$engine_timeout" \
    "${container_engine[@]}" exec -i "$container_id" \
    mysql \
    --defaults-extra-file=/run/secrets/issue9-root-client.cnf \
    --batch --skip-column-names 2>/dev/null <<'SQL'
SELECT @@GLOBAL.server_uuid;
SQL
)" || fail "could not read the isolated MySQL identity"
server_uuid="$(printf '%s' "$server_uuid" | tr -d '\r\n')"
[[ "$server_uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] ||
  fail "isolated MySQL returned an invalid server identity"
[[ "$server_uuid" != "00000000-0000-0000-0000-000000000000" ]] ||
  fail "isolated MySQL returned a zero server identity"

app_dsn="shm_dev_app:${app_password}@tcp(127.0.0.1:13307)/second_hand_market_dev?charset=utf8mb4&parseTime=true&loc=Local"
wrong_user_dsn="${wrong_user}:${wrong_user_password}@tcp(127.0.0.1:13307)/second_hand_market_dev?charset=utf8mb4&parseTime=true&loc=Local"
printf '%s\n' "$app_dsn" >"$app_dsn_file"
printf '%s\n' "$wrong_user_dsn" >"$wrong_user_dsn_file"
printf '%s\n' "$server_uuid" >"$server_uuid_file"
{
  printf '%s\n' \
    "$root_password" \
    "$app_password" \
    "$wrong_user_password" \
    "$app_dsn" \
    "$wrong_user_dsn" \
    "$server_uuid" \
    "$wrong_user" \
    "$decoy_schema" \
    "$business_table" \
    "$migration_table" \
    "$bootstrap_table" \
    "$seed_table" \
    "$crud_table" \
    "$denied_create_table" \
    "$decoy_create_table" \
    'second_hand_market_dev' \
    'shm_dev_app' \
    'shm_dev_app@%' \
    '127.0.0.1' \
    'SELECT DATABASE()' \
    '@@GLOBAL.server_uuid' \
    'CURRENT_USER()'
} >"$protected_patterns_file"
chmod 0600 \
  "$app_dsn_file" \
  "$wrong_user_dsn_file" \
  "$server_uuid_file" \
  "$protected_patterns_file"
unset root_password app_password wrong_user_password app_dsn wrong_user_dsn server_uuid

if [[ -n "${ACCEPTANCE_GOMODCACHE:-}" ]]; then
  [[ "$ACCEPTANCE_GOMODCACHE" == /* && -d "$ACCEPTANCE_GOMODCACHE" ]] ||
    fail "ACCEPTANCE_GOMODCACHE must be an absolute existing directory"
  module_cache="$(realpath "$ACCEPTANCE_GOMODCACHE")"
  case "$module_cache/" in
    "$repo_root/"*) fail "ACCEPTANCE_GOMODCACHE must be outside the Git worktree" ;;
  esac
else
  module_cache="$build_dir/go-mod-cache"
fi
mkdir -p -- \
  "$build_dir/tmp" \
  "$build_dir/go-cache" \
  "$build_dir/gopath" \
  "$module_cache"

tool_env=(
  PATH="$PATH"
  TMPDIR="$build_dir/tmp"
  GOTMPDIR="$build_dir/tmp"
  TEMP="$build_dir/tmp"
  TMP="$build_dir/tmp"
  GOCACHE="$build_dir/go-cache"
  GOMODCACHE="$module_cache"
  GOPATH="$build_dir/gopath"
  GOENV=off
  GOWORK=off
  GOTOOLCHAIN=local
  GOFLAGS=-buildvcs=false\ -mod=readonly
  GOPROXY=https://proxy.golang.org,direct
  GOSUMDB=sum.golang.org
  CGO_ENABLED=0
  APP_ENV=test
  DB_TARGET=local
  AUTO_MIGRATE=false
  SEED_DEFAULTS=false
  BUYER_WECHAT_LOGIN_MODE=mock
  BUYER_DOUYIN_LOGIN_MODE=mock
  ISSUE9_MYSQL_APP_DSN_FILE="$app_dsn_file"
  ISSUE9_MYSQL_WRONG_USER_DSN_FILE="$wrong_user_dsn_file"
  ISSUE9_MYSQL_SERVER_UUID_FILE="$server_uuid_file"
  ISSUE9_MYSQL_DECOY_SCHEMA="$decoy_schema"
  ISSUE9_MYSQL_BUSINESS_TABLE="$business_table"
  ISSUE9_MYSQL_MIGRATION_TABLE="$migration_table"
  ISSUE9_MYSQL_BOOTSTRAP_TABLE="$bootstrap_table"
  ISSUE9_MYSQL_SEED_TABLE="$seed_table"
  ISSUE9_MYSQL_CRUD_TABLE="$crud_table"
  ISSUE9_MYSQL_DENIED_CREATE_TABLE="$denied_create_table"
  ISSUE9_MYSQL_DECOY_CREATE_TABLE="$decoy_create_table"
)

scan_private_log() {
  local log_path="$1"
  local grep_status
  if grep -F -f "$protected_patterns_file" "$log_path" >/dev/null; then
    return 1
  fi
  grep_status=$?
  [[ "$grep_status" -eq 1 ]] || return 1
  if grep -Eq \
    ':13307([^0-9]|$)|[Pp][Oo][Rr][Tt][=:[:space:]]+13307([^0-9]|$)' \
    "$log_path" >/dev/null; then
    return 1
  fi
  grep_status=$?
  [[ "$grep_status" -eq 1 ]]
}

run_safe_command() {
  name="$1"
  shift
  log_path="$runtime_dir/$name.log"
  set +e
  (
    cd "$repo_root/backend"
    timeout --foreground "$command_timeout" "$@"
  ) >"$log_path" 2>&1
  command_status=$?
  set -e
  chmod 0600 "$log_path"
  if ! scan_private_log "$log_path"; then
    return 97
  fi
  printf '%s_exit_code=%d\n' "$name" "$command_status" >>"$summary"
  return "$command_status"
}

read_test_counts() {
  log_path="$1"
  awk '
    index($0, "\"Action\":\"pass\"") && index($0, "\"Test\":") { passes++ }
    index($0, "\"Action\":\"fail\"") && index($0, "\"Test\":") { failures++ }
    index($0, "\"Action\":\"skip\"") && index($0, "\"Test\":") { skips++ }
    END { print passes + 0, failures + 0, skips + 0 }
  ' "$log_path"
}

run_json_test() {
  name="$1"
  shift
  log_path="$runtime_dir/$name.log"
  set +e
  (
    cd "$repo_root/backend"
    timeout --foreground "$command_timeout" "$@"
  ) >"$log_path" 2>&1
  command_status=$?
  set -e
  chmod 0600 "$log_path"
  read -r pass_count fail_count skip_count < <(read_test_counts "$log_path")
  log_is_safe=1
  scan_private_log "$log_path" || log_is_safe=0
  printf '%s passed=%d failed=%d skipped=%d exit=%d\n' \
    "$name" "$pass_count" "$fail_count" "$skip_count" "$command_status"
  {
    printf '%s_passed=%d\n' "$name" "$pass_count"
    printf '%s_failed=%d\n' "$name" "$fail_count"
    printf '%s_skipped=%d\n' "$name" "$skip_count"
    printf '%s_exit_code=%d\n' "$name" "$command_status"
  } >>"$summary"
  case "$name" in
    go-test-focused)
      focused_passed="$pass_count"
      focused_failed="$fail_count"
      focused_skipped="$skip_count"
      ;;
    go-test-mysql)
      mysql_passed="$pass_count"
      mysql_failed="$fail_count"
      mysql_skipped="$skip_count"
      ;;
  esac
  if [[ "$log_is_safe" -ne 1 ]]; then
    return 97
  fi
  [[ "$command_status" -eq 0 &&
    "$pass_count" -gt 0 &&
    "$fail_count" -eq 0 &&
    "$skip_count" -eq 0 ]]
}

assert_required_tests() {
  log_path="$1"
  shift
  for required_test in "$@"; do
    awk -v required_test="$required_test" '
      index($0, "\"Action\":\"pass\"") &&
      index($0, "\"Test\":\"" required_test "\"") { found = 1 }
      END { exit(found ? 0 : 1) }
    ' "$log_path" ||
      return 1
  done
}

go_version="$(env -i "${tool_env[@]}" go env GOVERSION)"
if ! awk -v version="${go_version#go}" '
  BEGIN {
    split(version, parts, ".")
    exit((parts[1] > 1 || (parts[1] == 1 && parts[2] >= 22)) ? 0 : 1)
  }
'; then
  fail "Go 1.22 or newer is required"
fi
printf 'go=%s\n' "$(env -i "${tool_env[@]}" go version)" >>"$summary"

if ! run_safe_command go-mod-download \
  env -i "${tool_env[@]}" go mod download; then
  fail "go module download or protected-output scan failed"
fi
if ! run_safe_command go-mod-verify \
  env -i "${tool_env[@]}" go mod verify; then
  fail "go module verification or protected-output scan failed"
fi

focused_test_pattern='^(TestVerifyRemoteDevelopmentDatabaseIdentity|TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites|TestNewServerRejectsDatabaseWriteFlagsBeforeOpeningDatabase|TestNewServerRedactsDatabaseOpenErrors|TestVerifyConnectedDatabaseIdentitySkipsLocalTarget)$'
if ! run_json_test go-test-focused \
  env -i "${tool_env[@]}" \
  go test -json -p 1 -count=1 -run "$focused_test_pattern" ./internal/app; then
  fail "focused tests, counts, Skip gate, or protected-output scan failed"
fi
required_focused_tests=(
  "TestVerifyRemoteDevelopmentDatabaseIdentity/queries_and_accepts_exact_identity"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_query_failure_without_leaking_details"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/empty_database"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/empty_server_uuid"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/malformed_server_uuid"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/zero_server_uuid"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/noncanonical_server_uuid"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/uppercase_server_uuid"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/empty_user"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/user_without_host"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/user_with_empty_host"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_each_identity_mismatch_without_echoing_values/user_with_multiple_separators"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_nil_context_or_connection/nil_connection"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_nil_context_or_connection/nil_typed_connection"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_nil_context_or_connection/nil_sql_connection"
  "TestVerifyRemoteDevelopmentDatabaseIdentity/rejects_nil_context_or_connection/nil_typed_sql_connection"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/wrong_database"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/empty_database"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/wrong_server_uuid"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/empty_server_uuid"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/malformed_server_uuid"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/zero_server_uuid"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/wrong_user"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/empty_user"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/malformed_current_user"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/identity_query_error"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/nil_or_unavailable_connection_fails_closed/nil"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/nil_or_unavailable_connection_fails_closed/unavailable"
  "TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites/successful_identity_check_does_not_run_database_writes"
  "TestNewServerRejectsDatabaseWriteFlagsBeforeOpeningDatabase/auto_migrate"
  "TestNewServerRejectsDatabaseWriteFlagsBeforeOpeningDatabase/seed_defaults"
)
assert_required_tests \
  "$runtime_dir/go-test-focused.log" "${required_focused_tests[@]}" ||
  fail "focused evidence is missing a required passing test"

if ! run_json_test go-test-mysql \
  env -i "${tool_env[@]}" \
  go test -json -p 1 -count=1 -tags mysqlacceptance \
  -run '^TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance$' ./internal/app; then
  fail "MySQL tests, counts, Skip gate, or protected-output scan failed"
fi
required_mysql_tests=(
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/exact_identity"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/production_startup_failures_close_with_zero_writes/wrong_database"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/production_startup_failures_close_with_zero_writes/empty_database"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/production_startup_failures_close_with_zero_writes/wrong_server_uuid"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/production_startup_failures_close_with_zero_writes/wrong_user"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/production_startup_failures_close_with_zero_writes/identity_query_error"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/application_user_crud"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/application_user_ddl_denied"
  "TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance/production_schema_decoy_denied"
)
assert_required_tests \
  "$runtime_dir/go-test-mysql.log" "${required_mysql_tests[@]}" ||
  fail "MySQL evidence is missing a required passing test"

if ! scan_private_log "$summary"; then
  fail "acceptance summary contains protected identity or connection details"
fi
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "acceptance commands changed tracked or untracked source files"
final_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$final_sha" == "$actual_sha" ]] ||
  fail "HEAD changed during acceptance"
if git -C "$repo_root" symbolic-ref -q HEAD >/dev/null 2>&1; then
  fail "HEAD was no longer detached after acceptance"
fi

if ! cleanup_resources; then
  fail "isolated resource cleanup failed"
fi
cleanup_complete=1

if [[ "$evidence_is_temporary" -eq 1 ]]; then
  temporary_evidence="$evidence_dir"
  evidence_dir=''
  rm -rf -- "$temporary_evidence" ||
    fail "temporary evidence cleanup failed"
  [[ ! -e "$temporary_evidence" ]] ||
    fail "temporary evidence cleanup was incomplete"
else
  printf 'status=PASS\n' >>"$summary"
fi

trap - EXIT
printf \
  'remote-development-database-identity acceptance: PASS focused_passed=%d focused_failed=%d focused_skipped=%d mysql_passed=%d mysql_failed=%d mysql_skipped=%d\n' \
  "$focused_passed" \
  "$focused_failed" \
  "$focused_skipped" \
  "$mysql_passed" \
  "$mysql_failed" \
  "$mysql_skipped"
