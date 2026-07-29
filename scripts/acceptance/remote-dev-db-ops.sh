#!/usr/bin/env bash
set -Eeuo pipefail

fail() {
  printf 'remote-dev-db-ops acceptance: FAIL: %s\n' "$*" >&2
  exit 1
}

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" ||
  fail "run this script from a Git checkout"
compose_file="$repo_root/deploy/remote-dev-db/docker-compose.yml"
prepare_script="$repo_root/deploy/remote-dev-db/prepare-secrets.sh"
verify_script="$repo_root/deploy/remote-dev-db/verify.sh"
runbook="$repo_root/deploy/remote-dev-db/README.md"

for path in "$compose_file" "$prepare_script" "$verify_script" "$runbook"; do
  [[ -f "$path" ]] || fail "missing ${path#$repo_root/}"
done

bash -n "$prepare_script" || fail "prepare-secrets.sh has invalid syntax"
bash -n "$verify_script" || fail "verify.sh has invalid syntax"

required_compose_text=(
  'name: secondhand-market-dev-db'
  'mysql:8.4@sha256:be18eb9dc45eea9b86cb74f8c68ab92ce8569ecc37ea4e6fade02e37036c5ff4'
  'container_name: secondhand-market-dev-mysql'
  '127.0.0.1:3307:3306'
  'MYSQL_DATABASE: second_hand_market_dev'
  'MYSQL_USER: shm_dev_app'
  'MYSQL_PASSWORD_FILE: /run/secrets/mysql_app_password'
  'MYSQL_ROOT_PASSWORD_FILE: /run/secrets/mysql_root_password'
  'restart: unless-stopped'
  'mem_limit: 768m'
  'cpus: 0.75'
  'name: secondhand-market-dev-mysql-data'
  'name: secondhand-market-dev-db-net'
)
for text in "${required_compose_text[@]}"; do
  grep -Fq -- "$text" "$compose_file" || fail "compose is missing $text"
done

grep -Fq -- '--max-connections=40' "$compose_file" ||
  fail "compose does not cap MySQL connections"
grep -Fq -- '--innodb-buffer-pool-size=192M' "$compose_file" ||
  fail "compose does not cap the buffer pool"
grep -Fq -- 'mysql_app_password' "$compose_file" ||
  fail "compose does not mount the app password secret"
grep -Fq -- 'mysql_root_password' "$compose_file" ||
  fail "compose does not mount the root password secret"

grep -Fq -- '/deploy/remote-dev-db/secrets/*' "$repo_root/.gitignore" ||
  fail "remote database secrets are not ignored"

if git -C "$repo_root" ls-files --error-unmatch \
  deploy/remote-dev-db/secrets/mysql_app_password >/dev/null 2>&1; then
  fail "application password is tracked"
fi
if git -C "$repo_root" ls-files --error-unmatch \
  deploy/remote-dev-db/secrets/mysql_root_password >/dev/null 2>&1; then
  fail "root password is tracked"
fi

grep -Fq -- 'secondhand-market-mysql' "$verify_script" &&
  fail "verification script references the production container"
grep -Fq -- '/home/yu/services/secondhand-market/deploy' "$runbook" &&
  fail "runbook references the production deploy directory"

printf 'remote-dev-db-ops acceptance: PASS\n'
