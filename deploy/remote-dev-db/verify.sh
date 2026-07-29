#!/usr/bin/env bash
set -Eeuo pipefail

fail() {
  printf 'remote-dev-db verification: FAIL: %s\n' "$*" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
compose_file="$script_dir/docker-compose.yml"
container_name="secondhand-market-dev-mysql"

command -v docker >/dev/null 2>&1 || fail "docker is required"
[[ -f "$compose_file" ]] || fail "docker-compose.yml is missing"
for name in mysql_app_password mysql_root_password; do
  path="$script_dir/secrets/$name"
  [[ -s "$path" ]] || fail "$name is missing or empty"
  [[ "$(stat -c '%a' "$path")" == "600" ]] || fail "$name permissions must be 600"
done

docker compose -f "$compose_file" config --quiet || fail "compose configuration is invalid"
docker inspect "$container_name" >/dev/null 2>&1 || fail "development MySQL container is missing"

for _ in $(seq 1 30); do
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container_name")"
  [[ "$health" == "healthy" ]] && break
  sleep 2
done
[[ "${health:-missing}" == "healthy" ]] || fail "development MySQL is not healthy"

[[ "$(docker port "$container_name" 3306/tcp)" == "127.0.0.1:3307" ]] ||
  fail "development MySQL is not bound to 127.0.0.1:3307"
[[ "$(docker inspect --format '{{.HostConfig.Memory}}' "$container_name")" == "805306368" ]] ||
  fail "memory limit is not 768 MiB"
[[ "$(docker inspect --format '{{.HostConfig.NanoCpus}}' "$container_name")" == "750000000" ]] ||
  fail "CPU limit is not 0.75"

identity="$({
  docker exec "$container_name" sh -eu -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    exec mysql --batch --skip-column-names -u shm_dev_app second_hand_market_dev \
      -e "SELECT DATABASE(), @@GLOBAL.server_uuid, CURRENT_USER();"
  '
} 2>/dev/null)" || fail "database identity query failed"

IFS=$'\t' read -r database_name server_uuid current_user <<<"$identity"
[[ "$database_name" == "second_hand_market_dev" ]] || fail "database name mismatch"
[[ -n "$server_uuid" ]] || fail "server UUID is empty"
[[ "$current_user" == shm_dev_app@* ]] || fail "database account mismatch"

production_schema_count="$({
  docker exec "$container_name" sh -eu -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    exec mysql --batch --skip-column-names -u shm_dev_app information_schema \
      -e "SELECT COUNT(*) FROM SCHEMATA WHERE SCHEMA_NAME = '\''second_hand_market'\'';"
  '
} 2>/dev/null)" || fail "schema isolation query failed"
[[ "$production_schema_count" == "0" ]] || fail "production schema exists in the development instance"

printf 'database=second_hand_market_dev\n'
printf 'server_uuid=%s\n' "$server_uuid"
printf 'database_user=shm_dev_app\n'
printf 'remote-dev-db verification: PASS\n'
