#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$base_dir"

managed_paths=(
  .env
  secrets/control-admin-password
  secrets/rotate-admin-password
)
for path in "${managed_paths[@]}"; do
  if [[ -e "$path" ]]; then
    echo "refusing to overwrite existing acceptance secret: $path" >&2
    exit 1
  fi
done

install -d -m 700 secrets backups evidence
umask 077

mysql_password="$(openssl rand -hex 32)"
mysql_root_password="$(openssl rand -hex 32)"
jwt_access_secret="$(openssl rand -hex 64)"
jwt_refresh_secret="$(openssl rand -hex 64)"

printf '%s\n' \
  'MYSQL_DATABASE=second_hand_market_acceptance' \
  'MYSQL_USER=shm_acceptance' \
  "MYSQL_PASSWORD=$mysql_password" \
  "MYSQL_ROOT_PASSWORD=$mysql_root_password" \
  '' \
  "JWT_ACCESS_SECRET=$jwt_access_secret" \
  "JWT_REFRESH_SECRET=$jwt_refresh_secret" \
  '' \
  'AUTO_MIGRATE=false' \
  '' \
  'ADMIN_BOOTSTRAP_USERNAME=acceptance_control' \
  'ADMIN_BOOTSTRAP_DISPLAY_NAME=Acceptance Control Admin' \
  > .env

openssl rand -base64 24 > secrets/control-admin-password
openssl rand -base64 24 > secrets/rotate-admin-password
chmod 600 .env secrets/control-admin-password secrets/rotate-admin-password

unset mysql_password mysql_root_password jwt_access_secret jwt_refresh_secret
echo 'acceptance secrets and directories prepared'
