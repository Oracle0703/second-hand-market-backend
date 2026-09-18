#!/usr/bin/env bash
set -euo pipefail

root=${DEPLOY_ROOT:?DEPLOY_ROOT is required}
backend_image=${BACKEND_IMAGE:?BACKEND_IMAGE is required}
release_id=${RELEASE_ID:?RELEASE_ID is required}
frontend_archive=${FRONTEND_ARCHIVE:?FRONTEND_ARCHIVE is required}
export API_HOST_PORT=${API_HOST_PORT:-8080}
web_host_port=${WEB_HOST_PORT:?WEB_HOST_PORT is required}
compose_project_name=${COMPOSE_PROJECT_NAME:?COMPOSE_PROJECT_NAME is required}
legacy_compose_file=${LEGACY_COMPOSE_FILE:-}

[[ "$release_id" =~ ^[0-9a-f]{40}$ ]] || { echo 'RELEASE_ID must be a full commit SHA' >&2; exit 1; }
compose_file="$root/deploy/docker-compose.yml"
release_dir="$root/frontend/releases/$release_id"
current_link="$root/frontend/current"
test -f "$compose_file"
test -f "$frontend_archive"
mkdir -p "$root/frontend/releases"
exec 9>"$root/.deploy.lock"
flock -n 9 || { echo 'Another deployment is active' >&2; exit 1; }

compose() {
  COMPOSE_PROJECT_NAME="$compose_project_name" BACKEND_IMAGE="$backend_image" \
    docker compose -f "$compose_file" "$@"
}

legacy_compose() (
  # A caller's CD project name must never select the legacy services.
  unset COMPOSE_PROJECT_NAME
  cd "$(dirname "$legacy_compose_file")"
  docker compose --project-directory "$PWD" --env-file "$PWD/.env" -f "$legacy_compose_file" "$@"
)

health() {
  curl --fail --silent --show-error --connect-timeout 3 --max-time 10 \
    --retry 5 --retry-all-errors --retry-delay 1 --retry-max-time 30 "$1" >/dev/null
}

switch_frontend() {
  local target=$1
  local next_link="$root/frontend/.next-$$"
  ln -s "$target" "$next_link"
  mv -Tf "$next_link" "$current_link"
}

previous_target=''
if [ -L "$current_link" ]; then
  previous_target=$(readlink "$current_link")
elif [ -e "$current_link" ]; then
  echo 'frontend/current must be a symbolic link' >&2
  exit 1
fi

if [ ! -d "$release_dir" ]; then
  staging_dir=$(mktemp -d "$root/frontend/releases/.${release_id}.XXXXXX")
  tar -xzf "$frontend_archive" -C "$staging_dir" --no-same-owner --no-same-permissions
  test -f "$staging_dir/index.html"
  chmod 755 "$staging_dir"
  mv "$staging_dir" "$release_dir"
fi
test -f "$release_dir/index.html"
mkdir -p "$release_dir/assets/videos"
if [ -n "$previous_target" ]; then
  mkdir -p "$current_link/assets/videos"
fi

# `images -q` returns an image ID, not a container. Only a running container
# can be a rollback target; a failed first attempt may leave a Created object.
previous_container=$(compose ps --status running -q api)
previous_web=$(compose ps --status running -q web)
previous_image=''
if [ -n "$previous_container" ]; then
  previous_image=$(docker inspect --format '{{.Image}}' "$previous_container")
fi

if [ -z "$previous_image" ] && [ -n "$legacy_compose_file" ]; then
  test -f "$legacy_compose_file"
  test -f "$(dirname "$legacy_compose_file")/.env"
  for service in api web; do
    legacy_id=$(legacy_compose ps --status running -q "$service")
    test -n "$legacy_id" || { echo "Legacy $service is not running; refusing cutover" >&2; exit 1; }
    legacy_project=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$legacy_id")
    test "$legacy_project" != "$compose_project_name" || { echo 'Legacy and CD projects must differ' >&2; exit 1; }
  done
fi

compose config --quiet
compose pull api web

legacy_stopped=false
activation_started=false
frontend_switched=false
recover() {
  local rc=$?
  trap - EXIT INT TERM
  if [ "$rc" -eq 0 ] || [ "$activation_started" = false ]; then
    exit "$rc"
  fi
  echo 'Deployment failed; restoring previous services' >&2
  local recovery_failed=0
  if [ "$frontend_switched" = true ]; then
    if [ -n "$previous_target" ]; then
      switch_frontend "$previous_target" || recovery_failed=1
    else
      unlink "$current_link" || recovery_failed=1
    fi
  fi
  if [ "$legacy_stopped" = true ]; then
    compose stop api web || recovery_failed=1
    # Restart the original containers and images. Never build or touch MySQL.
    legacy_compose start api web || recovery_failed=1
  elif [ -n "$previous_image" ]; then
    backend_image=$previous_image
    compose up -d --no-deps --pull never --force-recreate --wait --wait-timeout 90 api || recovery_failed=1
    if [ -n "$previous_web" ]; then
      # Recreate after the API so Nginx resolves the restored container address.
      compose up -d --no-deps --pull never --force-recreate --wait --wait-timeout 90 web || recovery_failed=1
    else
      compose stop web || recovery_failed=1
    fi
  else
    compose stop api web || recovery_failed=1
  fi
  if [ "$legacy_stopped" = true ] || [ -n "$previous_image" ]; then
    health "http://127.0.0.1:$API_HOST_PORT/healthz" || recovery_failed=1
  fi
  if [ "$legacy_stopped" = true ] || [ -n "$previous_web" ]; then
    health "http://127.0.0.1:$web_host_port/" || recovery_failed=1
  fi
  if [ "$recovery_failed" -ne 0 ]; then
    echo 'Recovery failed; operator intervention is required' >&2
  else
    echo 'Previous services restored' >&2
  fi
  exit "$rc"
}
trap recover EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

activation_started=true
if [ -z "$previous_image" ] && [ -n "$legacy_compose_file" ]; then
  legacy_stopped=true
  legacy_compose stop api web
fi
compose up -d --no-deps --force-recreate --wait --wait-timeout 90 api
health "http://127.0.0.1:$API_HOST_PORT/healthz"
frontend_switched=true
switch_frontend "releases/$release_id"
compose up -d --no-deps --force-recreate --wait --wait-timeout 90 web
health "http://127.0.0.1:$web_host_port/"
health "http://127.0.0.1:$web_host_port/healthz"
activation_started=false
echo "Deployed release $release_id"
