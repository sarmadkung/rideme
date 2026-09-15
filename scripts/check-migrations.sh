#!/usr/bin/env bash
#
# Refuses a migrations directory golang-migrate would refuse.
#
# Two migrations claimed version 000014 and reached main, because they were
# merged from branches that never saw each other — neither PR's diff showed the
# other's file. golang-migrate then fails outright, so `migrate up` did not run
# at all, and the first person to notice would have been whoever deployed.
#
# The integration job would have caught it, eventually, on the next pull
# request that touched services/api. This catches it in a second, on every
# pull request, and locally before one is opened.
set -euo pipefail

DIR="${1:-services/api/migrations}"
status=0

if [ ! -d "$DIR" ]; then
  echo "check-migrations: no such directory: $DIR" >&2
  exit 1
fi

names=$(find "$DIR" -name '*.sql' -exec basename {} \; | sort)

# 1. One version, one migration. This is the failure that reached main.
duplicates=$(
  printf '%s\n' "$names" \
    | sed -E 's/\.(up|down)\.sql$//' \
    | sort -u \
    | grep -oE '^[0-9]+' \
    | sort | uniq -d
)
if [ -n "$duplicates" ]; then
  echo "Two migrations share a version. golang-migrate refuses this, so no migration runs:" >&2
  for version in $duplicates; do
    printf '%s\n' "$names" | grep -E "^${version}_" | sed 's/^/  /' >&2
  done
  echo "  Renumber the one that merged later, and correct schema_migrations by hand" >&2
  echo "  on any database that already applied the other." >&2
  status=1
fi

# 2. Every migration is reversible. The CI job proves down works by running it;
#    this proves the file exists before anyone depends on it.
for up in $(printf '%s\n' "$names" | grep '\.up\.sql$' || true); do
  down="${up%.up.sql}.down.sql"
  if ! printf '%s\n' "$names" | grep -qx "$down"; then
    echo "Missing rollback: $up has no $down" >&2
    status=1
  fi
done

# 3. Names golang-migrate can parse at all.
for name in $names; do
  if ! printf '%s' "$name" | grep -qE '^[0-9]{6}_[a-z0-9_]+\.(up|down)\.sql$'; then
    echo "Unparseable migration name: $name (expected 000123_lower_snake.up.sql)" >&2
    status=1
  fi
done

if [ "$status" -eq 0 ]; then
  count=$(printf '%s\n' "$names" | grep -c '\.up\.sql$')
  echo "check-migrations: $count migrations, versions unique, all reversible"
fi
exit "$status"
