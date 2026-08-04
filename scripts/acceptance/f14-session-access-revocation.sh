#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() {
  printf 'f14-session-access-revocation acceptance: FAIL: %s\n' "$*" >&2
  exit 1
}

for command_name in \
  git go timeout vips node npm openssl awk grep sed stat tee seq sleep cat cc id; do
  command -v "$command_name" >/dev/null 2>&1 ||
    fail "$command_name is required"
done

expected_sha="${EXPECTED_COMMIT_SHA:-}"
[[ "$expected_sha" =~ ^[0-9a-f]{40}$ ]] ||
  fail "EXPECTED_COMMIT_SHA must be the full 40-character commit SHA"

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" ||
  fail "run this script from a Git checkout"
repo_root="$(cd "$repo_root" && pwd -P)"
actual_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$actual_sha" == "$expected_sha" ]] ||
  fail "checked out commit does not match EXPECTED_COMMIT_SHA"
if git -C "$repo_root" symbolic-ref -q HEAD >/dev/null 2>&1; then
  fail "acceptance must run from a detached HEAD"
fi
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "worktree must be clean so the tested source matches the commit"

script_relative_path="scripts/acceptance/f14-session-access-revocation.sh"
git -C "$repo_root" cat-file -e "$actual_sha:$script_relative_path" 2>/dev/null ||
  fail "acceptance script must be tracked at the tested commit"
script_blob="$(git -C "$repo_root" rev-parse "$actual_sha:$script_relative_path")"
tree_sha="$(git -C "$repo_root" rev-parse "$actual_sha^{tree}")"

command_timeout="${ACCEPTANCE_COMMAND_TIMEOUT:-30m}"
[[ "$command_timeout" =~ ^[1-9][0-9]*[smhd]$ ]] ||
  fail "ACCEPTANCE_COMMAND_TIMEOUT must look like 30m, 2h, or 900s"

mysql_image='mysql:8.4@sha256:be18eb9dc45eea9b86cb74f8c68ab92ce8569ecc37ea4e6fade02e37036c5ff4'
container_engine=""
if command -v docker >/dev/null 2>&1; then
  [[ -z "${DOCKER_HOST:-}" ]] ||
    fail "DOCKER_HOST must be unset; only the local Unix-socket engine is allowed"
  container_engine="$(command -v docker)"
  docker_host="$("$container_engine" context inspect \
    --format '{{ (index .Endpoints "docker").Host }}' 2>/dev/null)" ||
    fail "the active Docker context could not be inspected"
  [[ "$docker_host" == unix:///* ]] ||
    fail "the active Docker context is not a local Unix socket"
  docker_socket="${docker_host#unix://}"
  [[ "$docker_socket" == /* && -S "$docker_socket" ]] ||
    fail "the active Docker Unix socket is unavailable"
elif command -v podman >/dev/null 2>&1; then
  [[ -z "${CONTAINER_HOST:-}" ]] ||
    fail "CONTAINER_HOST must be unset; only the local engine is allowed"
  container_engine="$(command -v podman)"
  podman_socket="/run/user/$(id -u)/podman/podman.sock"
  [[ -S "$podman_socket" || -S /run/podman/podman.sock ]] ||
    fail "a local Podman Unix socket is required"
else
  fail "a local Docker or Podman engine is required for isolated MySQL 8.4"
fi

"$container_engine" info >/dev/null 2>&1 ||
  fail "the local container engine is unavailable"
"$container_engine" image inspect "$mysql_image" >/dev/null 2>&1 ||
  fail "the pinned MySQL 8.4 image is not present locally; network pulls are forbidden"

evidence_is_temporary=0
if [[ -n "${EVIDENCE_DIR:-}" ]]; then
  mkdir -p -- "$EVIDENCE_DIR"
  evidence_root="$(cd "$EVIDENCE_DIR" && pwd -P)"
  case "$evidence_root/" in
    "$repo_root/"*) fail "EVIDENCE_DIR must be outside the Git worktree" ;;
  esac
  evidence_dir="$(mktemp -d "$evidence_root/f14-session-access-${actual_sha:0:12}.XXXXXX")"
else
  evidence_dir="$(mktemp -d)"
  evidence_is_temporary=1
fi

build_dir="$(mktemp -d)"
fixture_dir="$(mktemp -d)"
if [[ -n "${ACCEPTANCE_GOMODCACHE:-}" ]]; then
  [[ "$ACCEPTANCE_GOMODCACHE" == /* ]] ||
    fail "ACCEPTANCE_GOMODCACHE must be an absolute path"
  [[ -d "$ACCEPTANCE_GOMODCACHE" ]] ||
    fail "ACCEPTANCE_GOMODCACHE must be an existing directory"
  module_cache="$(cd "$ACCEPTANCE_GOMODCACHE" && pwd -P)"
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
  "$build_dir/bin" \
  "$module_cache"

suffix="$(openssl rand -hex 6)"
container_name="f14-session-access-${suffix}"
container_label="f14-session-access=${actual_sha}"
database_name="f14_session_access_${suffix}"
application_user="f14_app_${suffix}"
root_password="$(openssl rand -hex 24)"
application_password="$(openssl rand -hex 24)"
fixture_nonce="$(openssl rand -hex 24)"
container_started=0
acceptance_succeeded=0

summary="$evidence_dir/summary.txt"
cleanup() {
  status=$?
  trap - EXIT
  if [[ "$container_started" -eq 1 ]]; then
    labels="$("$container_engine" inspect --format '{{ index .Config.Labels "f14-session-access" }}' "$container_name" 2>/dev/null || true)"
    if [[ "$labels" == "$actual_sha" ]]; then
      "$container_engine" rm -f "$container_name" >/dev/null 2>&1 || true
    fi
  fi
  rm -rf -- "$build_dir" "$fixture_dir"
  if [[ "$evidence_is_temporary" -eq 1 && "$acceptance_succeeded" -eq 1 ]]; then
    rm -rf -- "$evidence_dir"
  else
    printf 'evidence_dir=%s\n' "$evidence_dir" >&2
  fi
  exit "$status"
}
trap cleanup EXIT

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
  DB_DRIVER=sqlite
  DB_DSN=acceptance-no-external-database
  AUTO_MIGRATE=false
  SEED_DEFAULTS=false
  BUYER_WECHAT_LOGIN_MODE=mock
  BUYER_DOUYIN_LOGIN_MODE=mock
  STRICT_IMAGE_VIPS_INTEGRATION=1
  IMAGE_PROCESSOR_BIN=vips
)

run_logged() {
  name="$1"
  shift
  log_path="$evidence_dir/$name.log"
  set +e
  (
    cd "$repo_root/backend"
    timeout --foreground "$command_timeout" "$@"
  ) 2>&1 | tee "$log_path"
  pipeline_status=("${PIPESTATUS[@]}")
  set -e
  {
    printf '%s_exit_code=%d\n' "$name" "${pipeline_status[0]}"
    printf '%s_tee_exit_code=%d\n' "$name" "${pipeline_status[1]}"
  } >>"$summary"
  [[ "${pipeline_status[0]}" -eq 0 && "${pipeline_status[1]}" -eq 0 ]] ||
    fail "$name or its evidence capture failed"
}

assert_test_log() {
  name="$1"
  log_path="$evidence_dir/$name.log"
  read -r pass_count fail_count skip_count < <(
    awk '
      index($0, "\"Action\":\"pass\"") && index($0, "\"Test\":") { passes++ }
      index($0, "\"Action\":\"fail\"") && index($0, "\"Test\":") { failures++ }
      index($0, "\"Action\":\"skip\"") && index($0, "\"Test\":") { skips++ }
      END { print passes + 0, failures + 0, skips + 0 }
    ' "$log_path"
  )
  {
    printf '%s_pass_actions=%d\n' "$name" "$pass_count"
    printf '%s_fail_actions=%d\n' "$name" "$fail_count"
    printf '%s_test_level_skips=%d\n' "$name" "$skip_count"
  } >>"$summary"
  [[ "$pass_count" -gt 0 ]] || fail "$name reported zero passing test actions"
  [[ "$fail_count" -eq 0 ]] || fail "$name reported failing test actions"
  [[ "$skip_count" -eq 0 ]] || fail "$name reported test-level skips"
}

assert_required_test_passed() {
  log_name="$1"
  test_name="$2"
  awk -v test_name="$test_name" '
    index($0, "\"Action\":\"pass\"") &&
    index($0, "\"Test\":\"" test_name "\"") { found = 1 }
    END { exit(found ? 0 : 1) }
  ' "$evidence_dir/$log_name.log" ||
    fail "$log_name did not report PASS for $test_name"
}

write_fixture() {
  path="$1"
  value="$2"
  (umask 077; printf '%s\n' "$value" >"$path")
  [[ "$(stat -c '%a' "$path")" == "600" ]] ||
    fail "fixture files must have mode 0600"
}

verify_temp_env() {
  env -i "${tool_env[@]}" "$BASH" -c '
    set -Eeuo pipefail
    expected="$1"
    for name in TMPDIR GOTMPDIR TEMP TMP; do
      [[ "${!name-}" == "$expected" ]] || exit 1
    done
    : >"$GOTMPDIR/f14-session-access-write-probe"
    rm -f -- "$GOTMPDIR/f14-session-access-write-probe"
  ' f14-session-access-temp-check "$build_dir/tmp" ||
    fail "isolated temporary directory environment is incomplete or not writable"
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

{
  printf 'commit=%s\n' "$actual_sha"
  printf 'tree=%s\n' "$tree_sha"
  printf 'script_blob=%s\n' "$script_blob"
  printf 'go=%s\n' "$(env -i "${tool_env[@]}" go version)"
  printf 'mysql_image=%s\n' "$mysql_image"
  printf 'external_database=none\n'
  printf 'test_level_skip_policy=zero\n'
} >"$summary"

verify_temp_env
run_logged go-mod-download env -i "${tool_env[@]}" go mod download
run_logged go-mod-verify env -i "${tool_env[@]}" go mod verify

"$container_engine" run -d --rm \
  --name "$container_name" \
  --label "$container_label" \
  --pull=never \
  -p 127.0.0.1::3306 \
  --tmpfs /var/lib/mysql:rw,nosuid,nodev,noexec,size=768m \
  -e "MYSQL_ROOT_PASSWORD=$root_password" \
  -e "MYSQL_DATABASE=$database_name" \
  "$mysql_image" \
  --skip-name-resolve >/dev/null ||
  fail "failed to start the isolated MySQL 8.4 container"
container_started=1

ready=0
for _ in $(seq 1 90); do
  if "$container_engine" exec \
    -e "MYSQL_PWD=$root_password" \
    "$container_name" \
    mysqladmin --protocol=socket -uroot ping --silent >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
[[ "$ready" -eq 1 ]] || fail "isolated MySQL 8.4 did not become ready"

mysql_version="$("$container_engine" exec \
  -e "MYSQL_PWD=$root_password" \
  "$container_name" \
  mysql --protocol=socket -uroot --batch --skip-column-names \
  -e 'SELECT VERSION()' 2>/dev/null)" ||
  fail "could not read the isolated MySQL version"
[[ "$mysql_version" == 8.4.* ]] ||
  fail "the isolated database is not MySQL 8.4"

for migration in "$repo_root"/backend/migrations/*.up.sql; do
  [[ -f "$migration" ]] || fail "no migration artifacts were found"
  "$container_engine" exec -i \
    -e "MYSQL_PWD=$root_password" \
    "$container_name" \
    mysql --protocol=socket -uroot "$database_name" <"$migration" >/dev/null 2>&1 ||
    fail "an isolated MySQL migration failed"
done

setup_sql="$fixture_dir/setup.sql"
cat >"$setup_sql" <<SQL
CREATE USER '${application_user}'@'%' IDENTIFIED BY '${application_password}';
GRANT SELECT, INSERT, UPDATE, DELETE ON \`${database_name}\`.* TO '${application_user}'@'%';
CREATE TABLE \`${database_name}\`.f14_acceptance_guard (
  id BIGINT PRIMARY KEY,
  marker VARCHAR(64) NOT NULL
);
INSERT INTO \`${database_name}\`.f14_acceptance_guard (id, marker)
VALUES (1, '${fixture_nonce}');
FLUSH PRIVILEGES;
SQL
"$container_engine" exec -i \
  -e "MYSQL_PWD=$root_password" \
  "$container_name" \
  mysql --protocol=socket -uroot <"$setup_sql" >/dev/null 2>&1 ||
  fail "failed to provision the isolated MySQL application identity"
rm -f -- "$setup_sql"

server_uuid="$("$container_engine" exec \
  -e "MYSQL_PWD=$root_password" \
  "$container_name" \
  mysql --protocol=socket -uroot --batch --skip-column-names \
  -e 'SELECT @@server_uuid' 2>/dev/null)" ||
  fail "could not read the isolated MySQL server UUID"
[[ "$server_uuid" =~ ^[0-9a-fA-F-]{36}$ ]] ||
  fail "isolated MySQL server UUID is invalid"

published="$("$container_engine" port "$container_name" 3306/tcp 2>/dev/null)" ||
  fail "could not resolve the isolated MySQL loopback port"
[[ "$published" =~ ^127\.0\.0\.1:([0-9]+)$ ]] ||
  fail "isolated MySQL must publish only on 127.0.0.1"
mysql_port="${BASH_REMATCH[1]}"

dsn_file="$fixture_dir/dsn"
database_file="$fixture_dir/database"
user_file="$fixture_dir/user"
uuid_file="$fixture_dir/server-uuid"
nonce_file="$fixture_dir/nonce"
mysql_dsn="${application_user}:${application_password}@tcp(127.0.0.1:${mysql_port})/${database_name}?charset=utf8mb4&parseTime=true&loc=UTC&timeout=5s&readTimeout=5s&writeTimeout=5s"
write_fixture "$dsn_file" "$mysql_dsn"
write_fixture "$database_file" "$database_name"
write_fixture "$user_file" "${application_user}@%"
write_fixture "$uuid_file" "$server_uuid"
write_fixture "$nonce_file" "$fixture_nonce"

focused_pattern='^(TestSessionIdentityResolverUsesCurrentAccountState|TestSessionIdentityResolverRejectsInvalidSessions|TestSessionIdentityResolverRejectsAccountInvariants|TestSessionIdentityResolverRedactsDatabaseErrors|TestSessionLogoutRevokesOnlyCurrentSession|TestSessionAccessUsesCurrentAccountState|TestSessionIdentityMismatchFailsClosed|TestAnonymousRequestSkipsSessionLookupAndDatabaseErrorsAreRedacted|TestSessionImmediateRefreshSucceeds)$'
run_logged go-test-focused env -i "${tool_env[@]}" \
  go test -json -p 1 -count=1 -run "$focused_pattern" \
  ./internal/auth ./internal/middleware ./tests
assert_test_log go-test-focused
for required_test in \
  TestSessionIdentityResolverUsesCurrentAccountState \
  TestSessionIdentityResolverRejectsInvalidSessions \
  TestSessionIdentityResolverRejectsAccountInvariants \
  TestSessionIdentityResolverRedactsDatabaseErrors \
  TestSessionLogoutRevokesOnlyCurrentSession \
  TestSessionAccessUsesCurrentAccountState \
  TestSessionIdentityMismatchFailsClosed \
  TestAnonymousRequestSkipsSessionLookupAndDatabaseErrorsAreRedacted \
  TestSessionImmediateRefreshSucceeds; do
  assert_required_test_passed go-test-focused "$required_test"
done

run_logged go-test-full env -i "${tool_env[@]}" \
  go test -json -p 1 -count=1 ./...
assert_test_log go-test-full

run_logged go-test-race env -i "${tool_env[@]}" \
  CGO_ENABLED=1 CC="$(command -v cc)" \
  go test -json -race -p 1 -count=1 ./...
assert_test_log go-test-race

mysql_env=(
  "${tool_env[@]}"
  F14_MYSQL_DSN_FILE="$dsn_file"
  F14_MYSQL_DATABASE_FILE="$database_file"
  F14_MYSQL_USER_FILE="$user_file"
  F14_MYSQL_SERVER_UUID_FILE="$uuid_file"
  F14_MYSQL_NONCE_FILE="$nonce_file"
)
run_logged go-test-mysql env -i "${mysql_env[@]}" \
  go test -json -p 1 -count=1 -tags mysqlacceptance \
  -run '^TestSessionAccessRevocationMySQLAcceptance$' ./internal/auth
assert_test_log go-test-mysql
assert_required_test_passed go-test-mysql TestSessionAccessRevocationMySQLAcceptance
for required_subtest in \
  isolated_mysql_8_4_fixture \
  stale_jwt_uses_current_identity \
  invalid_sessions_fail_closed \
  logout_revokes_only_current_session \
  immediate_refresh_succeeds \
  database_errors_are_redacted; do
  assert_required_test_passed \
    go-test-mysql \
    "TestSessionAccessRevocationMySQLAcceptance/$required_subtest"
done

run_logged go-vet env -i "${tool_env[@]}" go vet -p 1 ./...
run_logged go-build env -i "${tool_env[@]}" go build -p 1 ./...

for secret_value in \
  "$root_password" \
  "$application_password" \
  "$mysql_dsn" \
  "$database_name" \
  "$application_user" \
  "$server_uuid" \
  "$fixture_nonce" \
  "$mysql_port"; do
  if grep -R -F -- "$secret_value" "$evidence_dir" >/dev/null 2>&1; then
    fail "acceptance evidence contains an isolated MySQL secret or identity value"
  fi
done

[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  fail "acceptance commands changed tracked or untracked source files"
final_sha="$(git -C "$repo_root" rev-parse HEAD)"
final_tree="$(git -C "$repo_root" rev-parse "$final_sha^{tree}")"
final_script_blob="$(git -C "$repo_root" rev-parse "$final_sha:$script_relative_path")"
[[ "$final_sha" == "$actual_sha" ]] || fail "HEAD changed during acceptance"
[[ "$final_tree" == "$tree_sha" ]] || fail "source tree changed during acceptance"
[[ "$final_script_blob" == "$script_blob" ]] ||
  fail "acceptance script changed during acceptance"
if git -C "$repo_root" symbolic-ref -q HEAD >/dev/null 2>&1; then
  fail "acceptance worktree stopped being detached"
fi

{
  printf 'mysql_version=%s\n' "$mysql_version"
  printf 'focused=PASS\n'
  printf 'full=PASS\n'
  printf 'race=PASS\n'
  printf 'mysql_8_4=PASS\n'
  printf 'vet=PASS\n'
  printf 'build=PASS\n'
  printf 'status=VERIFIED\n'
} >>"$summary"
printf 'f14-session-access-revocation acceptance: PASS\n'
acceptance_succeeded=1
