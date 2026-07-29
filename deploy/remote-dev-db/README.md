# Remote Development MySQL

This directory runs one independent MySQL instance for local development through an SSH tunnel. It does not modify or join the existing application Compose project.

Server path:

```text
/home/yu/services/secondhand-market-dev-db
```

First start:

```bash
cd /home/yu/services/secondhand-market-dev-db
./prepare-secrets.sh
docker compose pull
docker compose up -d
./verify.sh
./render-local-env.sh
```

The instance listens only on `127.0.0.1:3307`, uses database `second_hand_market_dev`, and creates application user `shm_dev_app`. The generated passwords and `backend.env.remote-dev` stay under the ignored `secrets/` directory. Transfer that generated environment file directly to the local ignored path `backend/.env.remote-dev`; do not print its contents.

Routine operations:

```bash
docker compose stop
docker compose start
./verify.sh
```

Do not run `docker compose down -v`. Database migrations, administrator bootstrap, and category seed remain explicit application commands and are run only against this development instance.
