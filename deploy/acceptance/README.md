# Isolated acceptance environment

This stack runs a production-mode API and admin frontend against an isolated
MySQL 8.4 database on the same Docker host. It is intended for migration,
concurrency, and UI acceptance before a production deployment.

## Idempotency atomicity acceptance

The idempotency acceptance is a separately authorized, one-run check. Build its
transfer package locally from the reviewed commit. Source-list mode enumerates
the immutable `HEAD` tree, not the index or working tree, and performs no Docker
or remote action:

```bash
IDEMPOTENCY_SOURCE_LIST_ONLY=1 \
  ./deploy/acceptance/idempotency-atomicity-smoke.sh > /tmp/idempotency-source-list.z
```

Create a new package directory outside the repository. The exporter refuses an
existing or relative destination, writes the NUL list, exports the listed bytes
with `git archive HEAD`, hashes the extracted snapshot, and binds the list,
manifest, and archive in `package-sha256.txt`:

```bash
idempotency_export_root="$(mktemp -d)"
IDEMPOTENCY_SOURCE_EXPORT_DIR="$idempotency_export_root/.idempotency-source" \
  ./deploy/acceptance/idempotency-atomicity-smoke.sh
cmp /tmp/idempotency-source-list.z \
  "$idempotency_export_root/.idempotency-source/source-files.z"
(
  cd "$idempotency_export_root/.idempotency-source"
  sha256sum -c package-sha256.txt
)
sha256sum "$idempotency_export_root/.idempotency-source/package-sha256.txt"
```

Record the final digest out of band. Subject to separate transfer authorization,
transfer only the resulting `.idempotency-source` directory to the exact remote
directory `/home/yu/services/secondhand-idempotency-acceptance-20260728`, then
extract its `source.tar` there. Do not transfer a workspace, `.git`, untracked
source, environment files, or generated evidence. On the remote host, compare
the recorded digest before extracting and running anything:

```bash
cd /home/yu/services/secondhand-idempotency-acceptance-20260728
test "$(sha256sum .idempotency-source/package-sha256.txt | cut -d ' ' -f1)" = \
  "$EXPECTED_IDEMPOTENCY_PACKAGE_MANIFEST_SHA256"
(
  cd .idempotency-source
  sha256sum -c package-sha256.txt
)
tar -xf .idempotency-source/source.tar
```

The whitelist contains only `Makefile`, the backend Dockerfile and Go module
files, committed backend Go files, committed migrations, and committed
non-sensitive acceptance files. It excludes `.env`, secrets, databases,
`backend/app.db`, uploads, evidence, backups, `.git`, caches, `node_modules`,
`.tmp`, and protected review documents.

Generate only acceptance credentials with `deploy/acceptance/prepare.sh` in the
fixed remote directory. Normal mode defaults to `.idempotency-source`, validates
the package checksums, validates the received regular files against the source
manifest, and reconstructs the build context from the validated archive before
its first Docker call. It neither requires nor reads remote Git metadata. Run
exactly:

```bash
IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
COMPOSE_PROJECT_NAME=secondhand-idempotency-acceptance \
IDEMPOTENCY_SOURCE_PACKAGE_MANIFEST_SHA256="$EXPECTED_IDEMPOTENCY_PACKAGE_MANIFEST_SHA256" \
make acceptance-idempotency-smoke
```

The harness refuses any existing `secondhand-idempotency-acceptance` container,
volume, network, or evidence directory. It starts only that project's MySQL
8.4 service, applies migrations `0001` through `0009`, and runs the isolated
MySQL idempotency contract with AutoMigrate disabled and enabled. It then runs
the full backend tests, the focused race suites, and `go vet` with the MySQL
opt-in disabled. The build context is reconstructed from the committed
whitelist and verified against its SHA-256 manifest.

Evidence is written under
`deploy/acceptance/evidence/idempotency-atomicity`. Apart from the before/after
files containing one authorized snapshot row for each of the three production
containers, evidence contains only classified PASS lines, counts, manifest
digests, the zero-match sanitization result, and SHA-256 hashes of every
evidence file. It never includes test payloads, stored responses, credentials,
tokens, buyer contact fields, database connection values, or raw test
identifiers.

After the production-before snapshot succeeds, any MySQL, migration, test,
race, vet, later snapshot, or evidence failure stops isolated containers and
retains a freshly constructed failure evidence set. That set can contain only
validated PASS checkpoints, a fixed classified failure stage, complete
authorized snapshots already captured, the leak-scan classification, and
hashes. Raw command output stays in temporary storage and is deleted. If a
checkpoint, snapshot, or sanitization scan cannot be validated, the harness
publishes only hardcoded evidence-sanitization failure classifications and
their hashes.

Before and after the run, the harness compares only the name, container ID,
state, and restart count of `secondhand-market-api`, `secondhand-market-web`,
and `secondhand-market-mysql`. It does not inspect production SQL, data, logs,
environment, mounts, configuration, services, migrations, or deployments. The
isolated project containers are stopped after the comparison; its containers,
volumes, networks, and sanitized evidence are retained for separately
authorized review. The existing-resource and evidence guards make the run
one-shot. Do not delete, reuse, or rerun those retained resources without a new
authorization.

## Buyer intent open-uniqueness matrix

Task 9 must transfer the approved source whitelist to the exact remote path
`/home/yu/services/secondhand-buyer-intent-acceptance-20260727`. The transfer includes
only the repository `Makefile`, the backend Dockerfile and Go module files,
backend Go source excluding caches and uploads, migrations `0001..0009`, and
non-sensitive acceptance shell, YAML, configuration, Markdown, Dockerfile, and
SQL files. It excludes `.env`, `.env.*`, secrets, databases, `backend/app.db`,
uploads, evidence, backups, `.git`, caches, `node_modules`, `.tmp`, compiled
artifacts, configuration examples, and the protected review documents.

Generate `.env` and the acceptance-only secret on that host with
`deploy/acceptance/prepare.sh`. From the exact remote path, run:

```bash
BUYER_INTENT_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
COMPOSE_PROJECT_NAME=secondhand-buyer-intent-acceptance \
make acceptance-buyer-intent-smoke
```

The harness refuses an existing Compose project or evidence directory. It
stores sanitized results under
`deploy/acceptance/evidence/buyer-intent-open-uniqueness`, including
`production-before.txt` and `production-after.txt`. Those production snapshots
contain only the fixed container name, container ID, state, and restart count
for `secondhand-market-api`, `secondhand-market-web`, and
`secondhand-market-mysql`; the harness does not inspect production environment
variables, mounts, logs, networks, database content, uploads, or services.

After the run, isolated containers are stopped while the
`secondhand-buyer-intent-acceptance` containers, volumes, networks, and evidence
are retained for authorized inspection. A passing matrix validates isolated
MySQL 8.4 migration and application behavior; it does not execute production 0009.

It deliberately does not reuse production container names, networks, volumes,
credentials, upload storage, or published ports. MySQL is attached only to an
internal database network and has no host port. API and Web share a separate
edge network; their published ports listen on host loopback only:

- Web: `127.0.0.1:18081`
- API: `127.0.0.1:18082`

The three normal services are `mysql`, `api`, and `web`. The optional
`bootstrap-admin` service is behind the `tools` profile and never runs during a
normal `docker compose up`.

## Prerequisites

- Docker Engine with Docker Compose v2 and support for `depends_on.condition`.
- A dedicated checkout of the exact release commit to be tested.
- A fresh logical dump produced with consistent snapshot semantics and without
  `CREATE DATABASE`, `USE`, GTID, or production user/grant statements.
- Enough disk for a separate MySQL data volume, build cache, and acceptance
  uploads.

Do not run this stack from a live deployment directory. Do not mount a live
database volume or live upload directory into it.

Before creating anything, verify that the fixed Compose project name and
loopback ports are unused. Stop if any command prints an existing resource or
listener; do not delete or reuse it without identifying its owner:

```bash
docker ps -a --filter label=com.docker.compose.project=secondhand-acceptance \
  --format '{{.ID}} {{.Names}} {{.Status}}'
docker volume ls --filter label=com.docker.compose.project=secondhand-acceptance
docker network ls --filter label=com.docker.compose.project=secondhand-acceptance
ss -ltnp | grep -E ':(18081|18082)\b' || true
```

## 1. Prepare secrets

Run all commands in this directory.

```bash
./prepare.sh
```

The preparation script refuses to overwrite existing files. It creates `.env`,
two administrator password files, and mode-`0700` backup/evidence directories.
All MySQL, JWT, and upload source-HMAC values are independent random acceptance
secrets; no production secret is read or reused. To prepare values manually
instead, start from `.env.example` and keep the MySQL application password
DSN-safe (letters, numbers, `_`, or `-`).

The administrator password files must remain mode `0600`; the bootstrap command
rejects files readable by group or other users. Do not place a password in a
shell argument or saved command output. Two acceptance-only administrators are
used so password rotation can prove that unrelated administrator sessions stay
active.

Before continuing, confirm that the resolved service list is isolated:

```bash
docker compose config --services
docker compose config --volumes
```

Do not save the full output of `docker compose config`, because it resolves and
prints secrets from `.env`.

On a shared host, build sequentially before starting acceptance services. The
backend image currently targets `amd64`; stop if the host architecture differs.
Recheck available memory after each build and stop if production headroom is
no longer adequate:

```bash
docker info --format '{{.Architecture}}'
free -h
docker compose build api
free -h
docker compose build web
free -h
docker compose --profile tools build bootstrap-admin
free -h
```

## 2. Start only MySQL and restore the clone

Start the database without starting the new API against the old schema:

```bash
docker compose up -d mysql
docker compose ps mysql
```

Restore a fresh dump into the acceptance database. This writes only to the
Compose-managed acceptance volume:

```bash
chmod 600 backups/source-clone.sql
stat -c '%a %n' backups/source-clone.sql
docker compose exec -T mysql sh -ec \
  'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE"' \
  < backups/source-clone.sql
```

The dump can contain production-derived personal data. Keep it mode `0600`, do
not commit it, and remove it securely according to the project's data handling
policy after acceptance is complete.

## 3. Preflight and explicitly apply migration 0004

Do not rely on GORM AutoMigrate for the first schema transition. Keep
`AUTO_MIGRATE=false`, stop if any preflight check fails, and record sanitized
results under `evidence/`.

Run the checked-in fail-fast preflight:

```bash
set -o pipefail
docker compose exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/migrations/0004_merchant_multi_stock.preflight.sql
' | tee evidence/preflight.txt
```

At minimum, verify all of the following before applying SQL:

- There are no active orders and no negative stock values.
- Order numbers are unique and every order references an existing product.
- `SHOW INDEX FROM orders` contains the unique index
  `uk_product_active` (or the actual reviewed equivalent).
- The expected protected merchant/product rows are recorded but not modified.
- The source dump can be restored again if the non-transactional MySQL DDL
  fails partway through.

Save the protected account and product fingerprints before migration, then run
the same query after all smoke tests and compare the files byte-for-byte:

```bash
docker compose exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/sql/protected-fingerprint.sql
' > evidence/protected-before.txt
```

Apply the checked-in migration exactly once:

```bash
set -o pipefail
docker compose exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/migrations/0004_merchant_multi_stock.up.sql
'
```

MySQL DDL auto-commits and this migration is not idempotent. On failure, do not
blindly rerun it. Capture the error, discard the acceptance MySQL volume, restore
a fresh clone, repeat preflight, and apply the migration again.

Postflight must prove:

- `orders.quantity` and `products.reserved_stock` exist with the intended
  defaults and constraints.
- Historical order quantities are `1` and reserved stock is consistent with
  active order quantities.
- Unique index `uk_product_active` is absent.
- Non-unique index `idx_order_product_active(product_id, is_active)` exists.
- No product has negative stock, reserved stock above stock, or an unresolved
  `LOCKED` status.

Run the checked-in post-migration gate before starting the API:

```bash
set -o pipefail
docker compose exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/migrations/0004_merchant_multi_stock.postflight.sql
' | tee evidence/post-migration.txt
```

## 4. Create two acceptance-only administrators

The cloned password hashes are not acceptance credentials. Add a control
administrator and a disposable password-rotation target only inside the clone:

```bash
ADMIN_BOOTSTRAP_USERNAME=acceptance_control \
ADMIN_BOOTSTRAP_DISPLAY_NAME='Acceptance Control Admin' \
ADMIN_BOOTSTRAP_PASSWORD_FILE_HOST=./secrets/control-admin-password \
docker compose --profile tools run --rm bootstrap-admin

ADMIN_BOOTSTRAP_USERNAME=acceptance_rotate \
ADMIN_BOOTSTRAP_DISPLAY_NAME='Acceptance Rotation Target' \
ADMIN_BOOTSTRAP_PASSWORD_FILE_HOST=./secrets/rotate-admin-password \
docker compose --profile tools run --rm bootstrap-admin
```

Bootstrap refuses to overwrite an existing username. If the configured name
already exists, choose a new acceptance-only username; never reset or modify an
existing administrator or merchant account.

## 5. Start the API and web UI

Start the previously built application with explicit SQL migration ownership:

```bash
docker compose up -d api web
docker compose ps
```

Both published ports must show `127.0.0.1`, and MySQL must show no published
port. The API health endpoint proves that Gin is responding, but it does not
query MySQL. Also verify an authenticated, database-backed endpoint before
declaring the application healthy.

From another machine, use an SSH tunnel instead of opening firewall ports:

```bash
ssh -N \
  -L 18081:127.0.0.1:18081 \
  -L 18082:127.0.0.1:18082 \
  user@acceptance-host
```

Then open `http://127.0.0.1:18081`. The direct API base is
`http://127.0.0.1:18082/api/v1`.

## 6. Run acceptance tests

First prove that changing one administrator password immediately revokes only
that account's sessions. The target password becomes intentionally disposable;
the unchanged control account remains available for the other smoke tests:

```bash
set -o pipefail
ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_CHANGES_AN_ACCEPTANCE_PASSWORD \
SMOKE_TARGET_ADMIN_USERNAME=acceptance_rotate \
SMOKE_TARGET_ADMIN_PASSWORD="$(tr -d '\r\n' < secrets/rotate-admin-password)" \
SMOKE_CONTROL_ADMIN_USERNAME=acceptance_control \
SMOKE_CONTROL_ADMIN_PASSWORD="$(tr -d '\r\n' < secrets/control-admin-password)" \
API_BASE_URL=http://127.0.0.1:18082/api/v1 \
node ../../scripts/smoke-admin-security.mjs \
  | tee evidence/admin-security.txt
```

Treat this administrator test as one-shot. If it reports a failure after the
password-change request succeeds, the generated replacement password may no
longer be recoverable. Discard the acceptance database volume, restore a fresh
clone, and bootstrap both acceptance administrators again; do not reset or
reuse a cloned administrator such as `admin`, `superadmin`, or `yaner`.

Use only the control administrator and acceptance-created merchant/product
records for the remaining flows. The existing main smoke flow creates and
completes orders, so never point it at production:

```bash
SMOKE_ADMIN_USERNAME=acceptance_control \
SMOKE_ADMIN_PASSWORD="$(tr -d '\r\n' < secrets/control-admin-password)" \
API_BASE_URL=http://127.0.0.1:18082/api/v1 \
node ../../scripts/smoke-flow.mjs
```

### Browser acceptance

Create a disposable merchant and product for browser checks. The credentials
file must be an absolute path that does not already exist; keep its parent
directory mode `0700` and never add it to evidence or command output:

```bash
credentials_dir="$(mktemp -d)"
chmod 700 "$credentials_dir"

ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_CREATES_AN_ACCEPTANCE_MERCHANT \
SMOKE_ADMIN_USERNAME=acceptance_control \
SMOKE_ADMIN_PASSWORD="$(tr -d '\r\n' < secrets/control-admin-password)" \
UI_ACCEPTANCE_CREDENTIALS_FILE="$credentials_dir/merchant.json" \
API_BASE_URL=http://127.0.0.1:18082/api/v1 \
node ../../scripts/prepare-ui-acceptance.mjs
```

Through the SSH tunnel, log in only with that disposable merchant and verify
the following at desktop and narrow mobile viewports:

- The product list shows total, reserved, and available stock separately.
- Creating an order accepts a positive integer quantity and a unit price, and
  recalculates the total before submission.
- Closing an order releases its quantity exactly once.
- Completing an order deducts its quantity exactly once without marking a
  product sold while stock remains.
- Order list and detail views show quantity, unit price, total price, and the
  three inventory values without hiding required actions.

Use test records only. Do not create an order against a cloned merchant or a
`yaner` product. Delete the temporary credentials directory after the browser
session is finalized.

Run the checked-in MySQL migration assertions and concurrency acceptance test
as part of the same session. Required behavior includes:

- Stock `5`, ten concurrent quantity-`1` creates: exactly five succeed and
  total reservations never exceed five.
- Closing releases a reservation once; completing deducts stock once.
- Concurrent complete/close attempts yield one terminal transition.
- Multiple active and multiple historical orders for one product do not hit a
  uniqueness error.
- Quantity, unit price, calculated total, total/reserved/available stock, and
  buyer-compatible available stock are correct.

The concurrency script refuses non-loopback URLs and requires two explicit
acceptance confirmations:

```bash
set -o pipefail
ACCEPTANCE_DB_ENGINE=mysql8.4 \
ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_WRITES_TEST_DATA \
SMOKE_ADMIN_USERNAME=acceptance_control \
SMOKE_ADMIN_PASSWORD="$(tr -d '\r\n' < secrets/control-admin-password)" \
API_BASE_URL=http://127.0.0.1:18082/api/v1 \
node ../../scripts/smoke-mysql-concurrency.mjs \
  | tee evidence/mysql-concurrency.txt
```

The test closes active test orders on success and performs best-effort cleanup
on failure. It intentionally leaves acceptance rows for auditability. Run the
SQL invariant gate after both smoke scripts:

```bash
set -o pipefail
docker compose exec -T mysql sh -ec '
  MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/sql/post-smoke.sql
' | tee evidence/post-smoke.txt
```

Finally keep `.env` at its safe default `AUTO_MIGRATE=false`, but recreate only
the API with a one-command override to test AutoMigrate compatibility:

```bash
AUTO_MIGRATE=true docker compose up -d --force-recreate api
docker compose ps api
docker compose exec -T api sh -ec 'test "$AUTO_MIGRATE" = true'
```

Rerun `smoke-mysql-concurrency.mjs`, then rerun `post-smoke.sql` and save their
outputs under distinct `after-automigrate` evidence names. This proves a
process restart cannot recreate `uk_product_active` and that MySQL concurrency
semantics remain intact. The recreated container retains `AUTO_MIGRATE=true`
when restarted with `docker compose start`. Running `docker compose up` or
recreating the API later without the same override uses `.env=false` and no
longer reproduces the tested container configuration. Record the override in
the evidence; it does not choose the eventual production setting.

## File record schema acceptance

This matrix uses the separate Compose project
`secondhand-file-schema-acceptance`; it never accepts a DSN and never targets
production. Prepare acceptance-only secrets with `./prepare.sh`, then run from
the repository root:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-schema-smoke
```

The command covers files-only rename, file_records-only no-op, both-table and
neither-table failures, the full SQL migration chain through `0009`, an
`AUTO_MIGRATE=false` file upload flow, and one `AUTO_MIGRATE=true`
compatibility startup. It leaves the isolated project and evidence in place
for inspection. It does not deploy or migrate production.

## File binding authorization acceptance

The F-02 matrix uses a third fixed Compose project,
`secondhand-file-binding-acceptance`. It refuses any existing container,
volume, or network carrying that project label, accepts no external DSN, and
writes evidence only under `evidence/file-binding-authorization/`.

Prepare acceptance-only secrets with `./prepare.sh`, then run from the
repository root:

```bash
FILE_BINDING_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-binding-smoke
```

The command recreates the `0001..0005` schema for each dirty-data fixture and
proves that orphan references, wrong file types, non-PASS files, empty URLs,
cross-merchant reuse, and uploader-account mismatches fail before `0006` DDL.
It then verifies clean ownership backfill, unbound PUBLIC/MERCHANT behavior,
the complete `0001..0009` chain, API registration/product binding, concurrent
one-time claim, and `AUTO_MIGRATE=true` compatibility. The command must never
run from a production checkout or against production volumes.

Successful runs retain the isolated project for review. After evidence is
approved, remove only that project explicitly:

```bash
docker compose --project-name secondhand-file-binding-acceptance \
  --env-file deploy/acceptance/.env \
  --file deploy/acceptance/docker-compose.yml \
  down --volumes --remove-orphans
```

## License file privacy acceptance

The F-04/F-13 matrix uses the separate fixed Compose project
`secondhand-license-privacy-acceptance`. It accepts no external DSN, refuses
pre-existing resources with that project label, and writes sanitized evidence
only under `evidence/license-file-privacy/`.

Prepare acceptance-only secrets with `./prepare.sh`, then run from the dedicated
acceptance checkout at the exact reviewed commit:

```bash
LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-license-file-privacy-smoke
```

The command rebuilds the `0001..0006` schema for every dirty fixture, proves
that `0007` preflight failures do not change file rows or license URLs, applies
the clean `0006 -> 0007 -> 0008 -> 0009 -> new API/frontend` sequence, and
runs the private-file API matrix with both `AUTO_MIGRATE=false` and
`AUTO_MIGRATE=true`. It requires MySQL 8.4.x, retains the isolated Compose
resources for review, and records source-independent runtime evidence under
the directory above.

This procedure must not execute production SQL, deploy backend or frontend
artifacts, read or change production uploads, or mutate production data. Its
only production interaction is read-only `docker inspect` of the named API,
Web, and MySQL containers before and after the isolated run; the snapshots must
match exactly before the success marker is printed.

## Anonymous upload governance acceptance

The F-06 matrix uses only the fixed Compose project
`secondhand-upload-governance-acceptance`. It refuses any existing container,
volume, or network with that project label, accepts no external DSN, keeps
MySQL internal, and binds API/Web only to `127.0.0.1`. Run it only from the
dedicated authorized checkout at the exact reviewed commit:

```bash
ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-anonymous-upload-governance-smoke
```

The command requires MySQL 8.4.x and applies the full `0001..0009` chain. It
proves dirty `0008` states fail with SQLSTATE `45000`, historical rows and
physical files retain their fingerprints, two independent MySQL pools enforce
one-winner quota semantics, cleanup and registration cannot delete a bound
file, and cleanup claims remain retryable and fail closed. The same focused
test runs with `AUTO_MIGRATE=false` and `AUTO_MIGRATE=true`, followed by the
full backend/frontend gates and exact 10 MiB file / 11 MiB request boundary
checks through both the API and Nginx.

Sanitized results, source hashes, and an evidence SHA-256 manifest are written
only under the ignored
`deploy/acceptance/evidence/anonymous-upload-governance/` directory. The
script's only production interaction is read-only inspection of container ID,
state, and restart count for the three named production containers; before and
after snapshots must match. It never executes production SQL, reads production
uploads, deploys an artifact, prints a secret, or automatically removes a
container, network, or volume.

The script stops isolated services at exit but retains all project resources
and evidence for review. After evidence has been approved, remove only this
project with a separate, explicit command:

```bash
docker compose --project-name secondhand-upload-governance-acceptance \
  --env-file deploy/acceptance/.env \
  --file deploy/acceptance/docker-compose.yml \
  down --volumes --remove-orphans
```

## Session access revocation acceptance

The F-14 matrix runs only from the separately authorized directory
`/home/yu/services/secondhand-session-revocation-acceptance-20260727` and uses
the fixed Compose project `secondhand-session-revocation-acceptance`. It
refuses an existing project container, volume, network, or evidence directory;
accepts only the internal Compose DSN
`mysql:3306/second_hand_market_acceptance`; and requires MySQL 8.4.x.

After `deploy/acceptance/prepare.sh` has generated remote-only secrets, run:

```bash
COMPOSE_PROJECT_NAME=secondhand-session-revocation-acceptance \
SESSION_REVOCATION_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-session-revocation-smoke
```

The script applies the full `0001..0009` migration chain and runs the focused
ADMIN, MERCHANT, and BUYER revocation matrix with both `AUTO_MIGRATE=false` and
`AUTO_MIGRATE=true`. It proves one-winner concurrent logout, unrelated-session
survival, explicit account disablement, merchant review downgrade, invalid
session rejection, fail-closed database errors, and primary-key MySQL query
plans. It then runs the complete backend suite and `go vet ./...`.

The transfer whitelist is limited to backend Go source/tests/migrations and
Dockerfile, non-sensitive `deploy/acceptance/` source/manifests, `Makefile`,
`backend/go.mod`, and `backend/go.sum`. Never transfer `.env`, secrets,
databases, uploads, evidence, `.git`, caches, `node_modules`, `backend/app.db`,
miniapp private configuration, `.tmp/`, or any protected review document. The
script creates a temporary Docker build context from this same whitelist, so
the remote-only `.env`, secrets, and evidence never enter the Docker build
context.

Sanitized evidence is retained under
`deploy/acceptance/evidence/session-access-revocation/`. It contains committed
source hashes, MySQL/tool results, PASS summaries, query-plan assertions,
production-container snapshots, and an evidence SHA-256 manifest. The only
production interaction is read-only `docker inspect` of three named container
identities, states, and restart counts; before and after snapshots must match.
The script never executes production SQL, reads production uploads, deploys or
restarts production services, or changes production data or sessions. It stops
the dedicated project services at exit and retains its resources for review.

## Miniapp auth refresh acceptance

The F-05 matrix is a source-only test-server review. It uses no database,
Docker service, DSN, application server, or application API request. It must
run from the retained dedicated source directory at the exact reviewed commit
with Node `v22.22.2` and npm `10.9.7`:

```bash
MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS \
make acceptance-miniapp-auth-refresh-smoke
```

The guard and exact toolchain checks run before `npm ci`. Dependency downloads
use the public `https://registry.npmmirror.com` registry with npm lockfile-host
replacement, so stale private registry URLs cannot receive a connection. The
script then runs the focused refresh suite, the full miniapp suite, and both
WeChat and Douyin production builds with
`TARO_APP_API_BASE_URL=https://example.invalid/api/v1`. It does not start either
bundle, contact a production API, read an `.env` file, or modify production
data.

Sanitized command output and its SHA-256 manifest are written only under the
ignored `deploy/acceptance/evidence/miniapp-auth-refresh/` directory. Keep the
authorized source directory and evidence in place for review. Passing this
matrix is test-server approval of the reviewed source; it is not a production
miniapp release and neither generated bundle may be deployed by this workflow.

## 7. Inspect and tear down

Capture only sanitized evidence:

```bash
docker compose ps
docker compose logs --since=30m api web
```

On a shared low-memory host, stop the acceptance services after evidence is
captured. This preserves the containers, isolated volumes, clone, and evidence
while releasing runtime memory and preventing `restart: unless-stopped` from
bringing the stack back after a host reboot:

```bash
docker compose stop
```

Do not include environment dumps, DSNs, passwords, JWTs, raw personal data, or
the full output of `docker compose config` in evidence.

Keep the environment until results have been reviewed. When teardown is
explicitly approved, remove the isolated containers and volumes from this
directory:

```bash
docker compose down --volumes --remove-orphans
```

This permanently deletes only the Compose-managed acceptance database and
upload volumes. The source dump, `.env`, password file, and evidence are host
files and require a separate, explicitly approved retention or deletion step.
