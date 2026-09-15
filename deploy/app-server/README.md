# Application Server Deployment

This directory is the server-side half of the minimal CD flow. It deploys an immutable API image and switches the React static files through an atomic symbolic-link update. Database migrations are deliberately excluded.

## One-time server setup

Install Docker Engine with the Compose plugin, Nginx and `curl`. Create an unprivileged deployment user that can run Docker, then create the deployment root:

```bash
export DEPLOY_ROOT=/srv/second-hand-market
install -d -m 0750 "$DEPLOY_ROOT"/{deploy,shared,data/runtime,incoming,frontend/releases}
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
| `COMPOSE_PROJECT_NAME` | Unique Docker Compose project name for this environment |

Set `DEPLOY_SSH_PRIVATE_KEY` and `DEPLOY_KNOWN_HOSTS` as environment secrets. `DEPLOY_KNOWN_HOSTS` must contain the pinned host key line from the server; do not generate it in CI.

The GitHub token needs permission to publish packages. The workflow publishes API images as `ghcr.io/<owner>/<repository>/api:<commit-sha>`.

## Release and rollback

Pushes to `main` deploy to `staging` after CI succeeds. Run the workflow manually with a full commit SHA to deploy that exact revision to `production`.

The deployment script checks API health before switching the frontend link. If API startup fails, it restarts the image that was active before the deployment. To roll back a successful release, run the workflow manually for the earlier commit SHA. Review migrations separately before any release; schema rollback is not part of this workflow.
