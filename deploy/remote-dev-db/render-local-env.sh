#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() {
  printf 'remote-dev-db local env: FAIL: %s\n' "$*" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
container_name="secondhand-market-dev-mysql"
password_path="$script_dir/secrets/mysql_app_password"
output_path="${1:-$script_dir/secrets/backend.env.remote-dev}"

[[ -s "$password_path" ]] || fail "application password is missing"
app_password="$(<"$password_path")"
[[ "$app_password" =~ ^[A-Za-z0-9+/=]+$ ]] || fail "application password format is unsupported"

server_uuid="$({
  docker exec "$container_name" sh -eu -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    exec mysql --batch --skip-column-names -u shm_dev_app second_hand_market_dev \
      -e "SELECT @@GLOBAL.server_uuid;"
  '
} 2>/dev/null)" || fail "server UUID query failed"
[[ -n "$server_uuid" ]] || fail "server UUID is empty"

{
  printf 'APP_ENV=development\n'
  printf 'ADDR=127.0.0.1:8080\n'
  printf 'DB_TARGET=remote-development\n'
  printf 'DB_DRIVER=mysql\n'
  printf "DB_DSN='shm_dev_app:%s@tcp(127.0.0.1:13307)/second_hand_market_dev?charset=utf8mb4&parseTime=true&loc=Local'\n" "$app_password"
  printf 'DB_EXPECTED_DATABASE=second_hand_market_dev\n'
  printf 'DB_EXPECTED_SERVER_UUID=%s\n' "$server_uuid"
  printf 'DB_EXPECTED_USER=shm_dev_app\n'
  printf 'AUTO_MIGRATE=false\n'
  printf 'SEED_DEFAULTS=false\n'
  printf 'FILE_UPLOAD_LOCAL_DIR=runtime/dev-uploads\n'
  printf 'SSH_HOST=yu\n'
  printf 'SSH_LOCAL_PORT=13307\n'
  printf 'SSH_REMOTE_HOST=127.0.0.1\n'
  printf 'SSH_REMOTE_PORT=3307\n'
} >"$output_path"
chmod 600 -- "$output_path"

printf 'remote-dev-db local env: PASS\n'
