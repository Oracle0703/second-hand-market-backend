# Application Server Deployment

This directory is the server-side half of the minimal CD flow. It deploys an immutable API image and switches the React static files through an atomic symbolic-link update. Database migrations are deliberately excluded.

## One-time server setup

Install Docker Engine with the Compose plugin, Nginx and `curl`. Create an unprivileged deployment user that can run Docker, then create the deployment root:

```bash
export DEPLOY_ROOT=/srv/second-hand-market
install -d -m 0750 "$DEPLOY_ROOT"/{deploy,shared,data/runtime,data/videos,incoming,frontend/releases}
```

Copy `backend/configs/.env.production.mysql.example` to `$DEPLOY_ROOT/shared/api.env`, replace every placeholder, and give it mode `0600`. The API account must not have migration privileges. Keep `AUTO_MIGRATE=false` and `SEED_DEFAULTS=false`.

Install the Nginx template after replacing `__DEPLOY_ROOT__` with the actual root, then validate and reload it:

```bash
sudo nginx -t
sudo systemctl reload nginx
```

The Nginx host configuration must proxy both `/api/v1/` and `/uploads/` to the API. Do not expose the upload volume as an Nginx `alias`.

## GitHub Environment configuration

Create `staging` and `production` environments. Protect `production` with required reviewers. Set these environment variables in each environment:

| Name | Purpose |
| --- | --- |
| `DEPLOY_HOST` | Deployment server hostname or address |
| `DEPLOY_PORT` | SSH port, usually `22` |
| `DEPLOY_ROOT` | Server deployment root, for example `/srv/second-hand-market` |
| `DEPLOY_USER` | Unprivileged deployment account |
| `API_HOST_PORT` | Loopback API port, `8080` by default |
| `WEB_HOST_PORT` | Loopback port for the React Nginx container |
| `COMPOSE_PROJECT_NAME` | Unique Docker Compose project name for this environment |
| `API_RUNTIME_HOST_PATH` | Host directory mounted for database/uploads or uploads only |
| `API_RUNTIME_CONTAINER_PATH` | Matching API container path |
| `API_NETWORK_NAME` | Dedicated network, or the existing database Compose network |
| `API_NETWORK_EXTERNAL` | `false` for a managed network; `true` for an existing network |
| `WEB_VIDEOS_HOST_PATH` | Host directory mounted at `/assets/videos` |
| `LEGACY_COMPOSE_FILE` | Optional existing Compose file used only for first-cutover stop and rollback |

Set `DEPLOY_SSH_PRIVATE_KEY` and `DEPLOY_KNOWN_HOSTS` as environment secrets. `DEPLOY_KNOWN_HOSTS` must contain the pinned host key line from the server; do not generate it in CI.

The GitHub token needs permission to publish packages. The workflow publishes API images as `ghcr.io/<owner>/<repository>/api:<commit-sha>`.

## Release and rollback

Pushes to `main` deploy to `staging` after CI succeeds. Run the workflow manually with a full commit SHA to deploy that exact revision to `production`.

The deployment script checks API health before switching the frontend link and starting Web. A failed update restores the running API's image ID and frontend link, then recreates Web after the API. A first handover runs the legacy Compose file without the CD project name and restarts only the original API/Web containers on failure; it never builds legacy images or restarts MySQL. Failed containers left in `Created` state do not count as a previous release. A per-directory lock prevents concurrent activation.

On Linux with Docker Compose, `python3 deploy/app-server/tests/handover_test.py` rehearses failed first handover, retry, managed-release rollback, and repeated deployment using temporary directories and isolated containers. CI runs this alongside the image build.

To roll back a successful release, run the workflow manually for the earlier commit SHA. Review migrations separately before any release; schema rollback is not part of this workflow.
