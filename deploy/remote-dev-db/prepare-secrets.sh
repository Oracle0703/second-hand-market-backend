#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
secrets_dir="$script_dir/secrets"
mkdir -p -- "$secrets_dir"

command -v openssl >/dev/null 2>&1 || {
  printf 'remote-dev-db secrets: FAIL: openssl is required\n' >&2
  exit 1
}

for name in mysql_app_password mysql_root_password; do
  path="$secrets_dir/$name"
  if [[ ! -e "$path" ]]; then
    openssl rand -base64 36 | tr -d '\r\n' >"$path"
  fi
  [[ -s "$path" ]] || {
    printf 'remote-dev-db secrets: FAIL: %s is empty\n' "$name" >&2
    exit 1
  }
  chmod 600 -- "$path"
done

printf 'remote-dev-db secrets: PASS\n'
