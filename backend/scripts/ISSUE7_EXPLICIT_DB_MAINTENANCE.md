# Explicit database maintenance commands

These are three separate, one-shot commands. Run them from the `backend` directory. None of
the commands invokes either of the other two, and none is part of normal API startup.

Set `DB_DRIVER` and `DB_DSN` explicitly for every invocation. Keep the DSN in the process
environment or an existing secret-management wrapper; do not place it on the command line or
in this repository.

## Apply one existing migration

The migration command accepts exactly one allowlisted existing MySQL `up` migration:

```text
go run ./scripts/migrate --migration 0001_init
go run ./scripts/migrate --migration 0002_buyer_domain
go run ./scripts/migrate --migration 0003_buyer_auth_provider
```

Each invocation validates and executes only the selected file from `migrations/`. It does not
chain earlier or later migrations, run GORM AutoMigrate, apply a down migration, bootstrap an
administrator, or seed categories. Missing, repeated, unknown, extra, or path-like selections
fail before a database connection is opened. The existing migration files use MySQL syntax, so
this command rejects `DB_DRIVER=sqlite`. MySQL may commit DDL statements individually; if an
execution fails, inspect the selected migration's database state before deciding whether to
retry it.

## Bootstrap one administrator

Provide the non-secret identity fields through:

```text
ADMIN_USERNAME
ADMIN_DISPLAY_NAME
ADMIN_ROLE
```

`ADMIN_ROLE` must be `ADMIN` or `SUPER_ADMIN`. On POSIX systems, also set
`ADMIN_PASSWORD_FILE` to an absolute path to a regular, non-symlink file containing one
password of at most 72 bytes, with one optional LF or CRLF record terminator. Its mode must be
exactly `0400` or `0600`.

On Windows, leave `ADMIN_PASSWORD_FILE` unset and run the command in a console. The command
prompts for the password with console echo disabled. Password files are rejected on Windows
because Go file modes cannot verify their ACLs.

Remove `ADMIN_PASSWORD` from the environment before invoking the command; its presence is
rejected. The password is never accepted as a command-line argument, built in, or logged.

```text
go run ./scripts/bootstrap_admin
```

The required `admin_users` schema must already exist. If the username already exists, the
command leaves its ID, password hash, display name, role, status, and timestamps unchanged.

## Seed the default category set

The category schema must already exist. Select the only supported seed explicitly:

```text
go run ./scripts/seed_categories --seed default-categories
```

The stable business key is `(parent_id, name)`. Existing rows can update only `status` and
`sort`; the command never rewrites ID, parent, level, name, creation time, update time, or
soft-delete identity. An identity conflict fails the transaction instead of moving or reviving
a category. Runs are serialized in-process, and MySQL runs additionally use a named advisory
lock so concurrent command processes cannot create duplicate root categories.
