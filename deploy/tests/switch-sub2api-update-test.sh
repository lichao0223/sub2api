#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

mkdir -p "$TEMP_DIR/bin"

cat > "$TEMP_DIR/bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail

if [ "$*" = "compose version" ]; then
  exit 0
fi

printf '%s\n' "$*" >> "$DOCKER_ARGS_LOG"

case "$*" in
  *" ps --status running -q"*) echo running ;;
esac
EOF
chmod +x "$TEMP_DIR/bin/docker"

make_deploy_dir() {
  local dir=$1
  mkdir -p "$dir"
  cat > "$dir/docker-compose.yml" <<'EOF'
services:
  sub2api:
    image: ghcr.io/lichao0223/sub2api:latest
  postgres:
    image: postgres:18-alpine
  redis:
    image: redis:8-alpine
EOF
  : > "$dir/.env"
}

assert_order() {
  local log=$1
  local first=$2
  local second=$3
  local first_line second_line
  first_line=$(grep -nF "$first" "$log" | head -n 1 | cut -d: -f1)
  second_line=$(grep -nF "$second" "$log" | head -n 1 | cut -d: -f1)
  test "$first_line" -lt "$second_line"
}

no_backup_dir="$TEMP_DIR/no-backup"
no_backup_log="$TEMP_DIR/no-backup.log"
make_deploy_dir "$no_backup_dir"
DOCKER_ARGS_LOG="$no_backup_log" PATH="$TEMP_DIR/bin:$PATH" \
  "$ROOT_DIR/deploy/switch-sub2api-source.sh" update --dir "$no_backup_dir" --no-backup -y

grep -Fq " pull sub2api" "$no_backup_log"
grep -Fq " up -d --no-deps sub2api" "$no_backup_log"
if grep -Fq " down" "$no_backup_log"; then
  echo "no-backup update stopped the whole stack" >&2
  exit 1
fi
assert_order "$no_backup_log" " pull sub2api" " up -d --no-deps sub2api"

backup_dir="$TEMP_DIR/with-backup"
backup_log="$TEMP_DIR/with-backup.log"
make_deploy_dir "$backup_dir"
DOCKER_ARGS_LOG="$backup_log" PATH="$TEMP_DIR/bin:$PATH" \
  "$ROOT_DIR/deploy/switch-sub2api-source.sh" update --dir "$backup_dir" --backup -y

grep -Fq " down" "$backup_log"
grep -Fq " up -d" "$backup_log"
assert_order "$backup_log" " pull sub2api" " down"

switch_dir="$TEMP_DIR/switch-source"
switch_log="$TEMP_DIR/switch-source.log"
make_deploy_dir "$switch_dir"
DOCKER_ARGS_LOG="$switch_log" PATH="$TEMP_DIR/bin:$PATH" \
  "$ROOT_DIR/deploy/switch-sub2api-source.sh" official --dir "$switch_dir" -y

assert_order "$switch_log" "image pull weishaw/sub2api:latest" " down"
grep -Fq "image: weishaw/sub2api:latest" "$switch_dir/docker-compose.yml"
grep -Fq "image: ghcr.io/lichao0223/sub2api:latest" "$switch_dir"/docker-compose.yml.bak.*

echo "switch update ordering checks passed"
