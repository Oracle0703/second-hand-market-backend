#!/usr/bin/env bash
set -euo pipefail

root=${DEPLOY_ROOT:?DEPLOY_ROOT is required}
backend_image=${BACKEND_IMAGE:?BACKEND_IMAGE is required}
release_id=${RELEASE_ID:?RELEASE_ID is required}
frontend_archive=${FRONTEND_ARCHIVE:?FRONTEND_ARCHIVE is required}
api_host_port=${API_HOST_PORT:-8080}
web_host_port=${WEB_HOST_PORT:?WEB_HOST_PORT is required}
compose_project_name=${COMPOSE_PROJECT_NAME:?COMPOSE_PROJECT_NAME is required}

if ! [[ "$release_id" =~ ^[0-9a-f]{40}$ ]]; then
    echo 'RELEASE_ID must be a lowercase hexadecimal commit SHA' >&2
    exit 1
fi

compose_file="$root/deploy/docker-compose.yml"
release_dir="$root/frontend/releases/$release_id"
current_link="$root/frontend/current"
previous_release_target=''

if [ -L "$current_link" ]; then
  previous_release_target=$(readlink "$current_link")
fi

test -f "$compose_file"
test -f "$frontend_archive"
mkdir -p "$root/frontend/releases" "$root/incoming"

if [ ! -d "$release_dir" ]; then
  staging_dir=$(mktemp -d "$root/frontend/releases/.${release_id}.XXXXXX")
  trap 'rm -rf "$staging_dir"' EXIT
  tar -xzf "$frontend_archive" -C "$staging_dir" --no-same-owner --no-same-permissions
  test -f "$staging_dir/index.html"
  mv "$staging_dir" "$release_dir"
  trap - EXIT
fi

container_id=$(COMPOSE_PROJECT_NAME="$compose_project_name" docker compose -f "$compose_file" images -q api || true)
previous_image=''
if [ -n "$container_id" ]; then
  previous_image=$(docker inspect --format '{{.Config.Image}}' "$container_id" 2>/dev/null || true)
fi

rollback_api() {
  if [ -n "$previous_image" ]; then
    echo "Restoring API image $previous_image" >&2
    COMPOSE_PROJECT_NAME="$compose_project_name" BACKEND_IMAGE="$previous_image" docker compose -f "$compose_file" up -d --no-deps api || true
  fi
}

rollback_frontend() {
  if [ -n "$previous_release_target" ]; then
    rollback_link="$root/frontend/.rollback-$release_id"
    rm -f "$rollback_link"
    ln -s "$previous_release_target" "$rollback_link"
    mv -Tf "$rollback_link" "$current_link"
    COMPOSE_PROJECT_NAME="$compose_project_name" BACKEND_IMAGE="${previous_image:-$backend_image}" docker compose -f "$compose_file" up -d --no-deps --force-recreate web || true
  fi
}

if ! COMPOSE_PROJECT_NAME="$compose_project_name" API_HOST_PORT="$api_host_port" BACKEND_IMAGE="$backend_image" docker compose -f "$compose_file" pull api web \
  || ! COMPOSE_PROJECT_NAME="$compose_project_name" API_HOST_PORT="$api_host_port" BACKEND_IMAGE="$backend_image" docker compose -f "$compose_file" up -d --no-deps --force-recreate --wait api \
  || ! curl --fail --silent --show-error "http://127.0.0.1:$api_host_port/healthz" >/dev/null; then
  rollback_api
  exit 1
fi

next_link="$root/frontend/.current-$release_id"
rm -f "$next_link"
ln -s "releases/$release_id" "$next_link"
mv -Tf "$next_link" "$current_link"

if ! COMPOSE_PROJECT_NAME="$compose_project_name" BACKEND_IMAGE="$backend_image" docker compose -f "$compose_file" up -d --no-deps --force-recreate --wait web \
  || ! curl --fail --silent --show-error "http://127.0.0.1:$web_host_port/" >/dev/null \
  || ! curl --fail --silent --show-error "http://127.0.0.1:$web_host_port/healthz" >/dev/null; then
  rollback_frontend
  rollback_api
  exit 1
fi

rm -f "$frontend_archive"

echo "Deployed release $release_id"
