#!/usr/bin/env bash
# If someone runs "sh switch-sub2api-source.sh", re-exec with bash.
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

set -euo pipefail

FORK_IMAGE="ghcr.io/lichao0223/sub2api:latest"
OFFICIAL_IMAGE="weishaw/sub2api:latest"
MAX_BACKUPS=20

DEPLOY_DIR="$(pwd)"
COMPOSE_FILE="docker-compose.yml"
YES=0

usage() {
  cat <<'EOF'
Usage:
  ./switch-sub2api-source.sh
  ./switch-sub2api-source.sh --dir DEPLOY_DIR
  ./switch-sub2api-source.sh fork|official|update|backup|restore|reset-admin|status|logs --dir DEPLOY_DIR [-y]

Interactive menu:
  Run without a command to show a menu.

Commands:
  fork      Backup data, switch sub2api image to ghcr.io/lichao0223/sub2api:latest, pull, start.
  official  Backup data, switch sub2api image back to weishaw/sub2api:latest, pull, start.
  update    Detect current image source, optionally backup data, pull latest configured image, restart.
  backup    Backup compose/env/local data dirs and named volumes if detected.
  restore   Restore data from a backup under DEPLOY_DIR/backups.
  reset-admin Reset an admin account password in PostgreSQL.
  status    Print current sub2api image and compose service status.
  logs      Follow sub2api logs.

Options:
  --dir DIR       Deployment directory containing docker-compose.yml and .env.
  --compose FILE  Compose file name/path relative to deployment dir. Default: docker-compose.yml.
  --max-backups N Keep at most N backups. Default: 20.
  -y, --yes       Do not ask for confirmation in command mode.

Important:
  Switching always creates a backup first.
  This script never runs "docker compose down -v".
EOF
}

die() {
  echo "[ERROR] $*" >&2
  exit 1
}

info() {
  echo "[INFO] $*"
}

warn() {
  echo "[WARN] $*" >&2
}

pause() {
  printf "\nPress Enter to continue..."
  read -r _
}

confirm() {
  if [ "$YES" = "1" ]; then
    return 0
  fi
  printf "%s [y/N] " "$1"
  read -r answer
  case "$answer" in
    y|Y|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

confirm_default_yes() {
  if [ "$YES" = "1" ]; then
    return 0
  fi
  printf "%s [Y/n] " "$1"
  read -r answer
  case "$answer" in
    n|N|no|NO) return 1 ;;
    *) return 0 ;;
  esac
}

compose_cmd() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  elif command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
  else
    die "Docker Compose is not installed"
  fi
}

compose_args() {
  local args=(-f "$DEPLOY_DIR/$COMPOSE_FILE")
  if [ -f "$DEPLOY_DIR/.env" ]; then
    args+=(--env-file "$DEPLOY_DIR/.env")
  fi
  printf '%s\n' "${args[@]}"
}

run_compose() {
  local args=()
  while IFS= read -r arg; do
    args+=("$arg")
  done < <(compose_args)
  compose_cmd "${args[@]}" "$@"
}

parse_env_value() {
  local key="$1"
  local file="$2"
  if [ ! -f "$file" ]; then
    return 1
  fi
  grep -E "^${key}=" "$file" | tail -n 1 | cut -d= -f2- | sed 's/^"//; s/"$//; s/^'\''//; s/'\''$//'
}

compose_project_name() {
  local env_file="$DEPLOY_DIR/.env"
  local from_env
  from_env="$(parse_env_value COMPOSE_PROJECT_NAME "$env_file" || true)"
  if [ -n "$from_env" ]; then
    echo "$from_env"
    return
  fi
  basename "$DEPLOY_DIR" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]/_/g'
}

find_sub2api_service_index() {
  awk '
    function trim(s) { sub(/^[ \t]*/, "", s); sub(/[ \t]*$/, "", s); return s }
    function strip_quotes(s) { gsub(/^["\047]+|["\047]+$/, "", s); return s }
    function key_name(s) {
      s = trim(s)
      sub(/[ \t]*#.*/, "", s)
      sub(/:.*/, "", s)
      return strip_quotes(trim(s))
    }
    function scalar_value(s) {
      s = trim(s)
      sub(/[ \t]*#.*/, "", s)
      return strip_quotes(trim(s))
    }
    function looks_like_key(s) {
      return s !~ /^-/ && s ~ /^[^:#][^:]*:[ \t]*(#.*)?$/
    }
    function finish_service() {
      if (service_index == 0) return
      if (service_name == "sub2api") named_index = service_index
      if (saw_container == 1 && container_index == 0) container_index = service_index
    }
    BEGIN {
      in_services = 0
      services_indent = -1
      service_key_indent = -1
      service_index = 0
      named_index = 0
      container_index = 0
      saw_container = 0
    }
    {
      raw = $0
      tmp = raw
      sub(/^[ \t]*/, "", tmp)
      indent = length(raw) - length(tmp)

      if (tmp ~ /^services:[ \t]*(#.*)?$/) {
        finish_service()
        in_services = 1
        services_indent = indent
        service_key_indent = -1
        service_index = 0
        next
      }

      if (!in_services) next

      if (tmp !~ /^($|#)/ && indent <= services_indent) {
        finish_service()
        in_services = 0
        next
      }

      if (tmp !~ /^($|#)/ && looks_like_key(tmp)) {
        if (service_key_indent < 0 && indent > services_indent) service_key_indent = indent
        if (indent == service_key_indent) {
          finish_service()
          service_index++
          service_name = key_name(tmp)
          saw_container = 0
          next
        }
      }

      if (service_index > 0 && indent > service_key_indent && tmp ~ /^container_name:[ \t]*/) {
        value = tmp
        sub(/^container_name:[ \t]*/, "", value)
        if (scalar_value(value) == "sub2api") saw_container = 1
      }
    }
    END {
      finish_service()
      if (named_index > 0) {
        print named_index
      } else if (container_index > 0) {
        print container_index
      } else {
        exit 42
      }
    }
  ' "$DEPLOY_DIR/$COMPOSE_FILE"
}

current_image() {
  local target_index
  target_index="$(find_sub2api_service_index 2>/dev/null || true)"
  [ -n "$target_index" ] || return 0

  awk -v target_index="$target_index" '
    function looks_like_key(s) {
      return s !~ /^-/ && s ~ /^[^:#][^:]*:[ \t]*(#.*)?$/
    }
    BEGIN {
      in_services = 0
      services_indent = -1
      service_key_indent = -1
      service_index = 0
      child_indent = -1
    }
    {
      raw = $0
      tmp = raw
      sub(/^[ \t]*/, "", tmp)
      indent = length(raw) - length(tmp)

      if (tmp ~ /^services:[ \t]*(#.*)?$/) {
        in_services = 1
        services_indent = indent
        service_key_indent = -1
        service_index = 0
        next
      }

      if (!in_services) next

      if (tmp !~ /^($|#)/ && indent <= services_indent) {
        in_services = 0
        next
      }

      if (tmp !~ /^($|#)/ && looks_like_key(tmp)) {
        if (service_key_indent < 0 && indent > services_indent) service_key_indent = indent
        if (indent == service_key_indent) {
          service_index++
          child_indent = -1
          next
        }
      }

      if (service_index == target_index && indent > service_key_indent && tmp !~ /^($|#)/) {
        if (child_indent < 0) child_indent = indent
        if (indent == child_indent && tmp ~ /^image:[ \t]*/) {
          sub(/^image:[ \t]*/, "", tmp)
          print tmp
          exit
        }
      }
    }
  ' "$DEPLOY_DIR/$COMPOSE_FILE"
}

normalize_image_name() {
  local image="$1"
  image="${image%%#*}"
  image="${image%"${image##*[![:space:]]}"}"
  image="${image#"${image%%[![:space:]]*}"}"
  image="${image%\"}"
  image="${image#\"}"
  image="${image%\'}"
  image="${image#\'}"
  echo "$image"
}

detect_current_source_image() {
  local image
  image="$(normalize_image_name "$(current_image || true)")"

  case "$image" in
    ghcr.io/lichao0223/sub2api|ghcr.io/lichao0223/sub2api:*)
      echo "$image"
      return 0
      ;;
    weishaw/sub2api|weishaw/sub2api:*)
      echo "$image"
      return 0
      ;;
    "")
      return 1
      ;;
    *)
      return 2
      ;;
  esac
}

detect_current_source() {
  local image
  image="$(normalize_image_name "$(current_image || true)")"

  case "$image" in
    ghcr.io/lichao0223/sub2api|ghcr.io/lichao0223/sub2api:*)
      echo "fork"
      return 0
      ;;
    weishaw/sub2api|weishaw/sub2api:*)
      echo "official"
      return 0
      ;;
    "")
      return 1
      ;;
    *)
      return 2
      ;;
  esac
}

set_sub2api_image() {
  local image="$1"
  local compose_path="$DEPLOY_DIR/$COMPOSE_FILE"
  local target_index
  local tmp

  set +e
  target_index="$(find_sub2api_service_index)"
  local find_status=$?
  set -e
  if [ "$find_status" -ne 0 ] || [ -z "$target_index" ]; then
    if [ "$find_status" -eq 42 ]; then
      die "Failed to update image: neither service name 'sub2api' nor 'container_name: sub2api' was found in $compose_path"
    fi
    die "Failed to inspect services in $compose_path (awk exit $find_status)"
  fi

  tmp="$(mktemp "${compose_path}.tmp.XXXXXX")"

  set +e
  awk -v image="$image" -v target_index="$target_index" '
    function spaces(n, s, i) {
      s = ""
      for (i = 0; i < n; i++) s = s " "
      return s
    }
    function looks_like_key(s) {
      return s !~ /^-/ && s ~ /^[^:#][^:]*:[ \t]*(#.*)?$/
    }
    function line_indent(s, t) {
      t = s
      sub(/^[ \t]*/, "", t)
      return length(s) - length(t)
    }
    function line_trim(s) {
      sub(/^[ \t]*/, "", s)
      return s
    }
    function flush_block(    i, raw, t, ind, changed, child_indent, has_image) {
      if (block_count == 0) return

      changed = 0
      has_image = 0
      child_indent = service_indent + 2
      if (service_index == target_index) {
        for (i = 2; i <= block_count; i++) {
          raw = block[i]
          t = line_trim(raw)
          ind = line_indent(raw)
          if (t !~ /^($|#)/ && ind > service_indent) {
            child_indent = ind
            break
          }
        }
        for (i = 2; i <= block_count; i++) {
          raw = block[i]
          t = line_trim(raw)
          ind = line_indent(raw)
          if (ind == child_indent && t ~ /^image:[ \t]*/) {
            has_image = 1
            break
          }
        }
      }

      for (i = 1; i <= block_count; i++) {
        raw = block[i]
        t = line_trim(raw)
        ind = line_indent(raw)

        if (service_index == target_index && ind == child_indent && t ~ /^image:[ \t]*/ && changed == 0) {
          print spaces(ind) "image: " image
          changed = 1
          continue
        }

        print raw

        if (service_index == target_index && i == 1 && has_image == 0 && changed == 0) {
          print spaces(child_indent) "image: " image
          changed = 1
        }
      }

      block_count = 0
    }
    BEGIN {
      in_services = 0
      services_indent = -1
      service_key_indent = -1
      service_index = 0
      block_count = 0
    }
    {
      raw = $0
      tmp_line = raw
      sub(/^[ \t]*/, "", tmp_line)
      indent = length(raw) - length(tmp_line)

      if (tmp_line ~ /^services:[ \t]*(#.*)?$/) {
        flush_block()
        in_services = 1
        services_indent = indent
        service_key_indent = -1
        service_index = 0
        print raw
        next
      }

      if (!in_services) {
        print raw
        next
      }

      if (tmp_line !~ /^($|#)/ && indent <= services_indent) {
        flush_block()
        in_services = 0
        print raw
        next
      }

      if (tmp_line !~ /^($|#)/ && looks_like_key(tmp_line)) {
        if (service_key_indent < 0 && indent > services_indent) service_key_indent = indent
        if (indent == service_key_indent) {
          flush_block()
          service_index++
          service_indent = indent
          block_count = 0
          block[++block_count] = raw
          next
        }
      }

      if (block_count > 0) {
        block[++block_count] = raw
      } else {
        print raw
      }
    }
    END {
      flush_block()
    }
  ' "$compose_path" > "$tmp"
  local awk_status=$?
  set -e

  if [ "$awk_status" -ne 0 ]; then
    rm -f "$tmp"
    die "Failed to update sub2api image in $compose_path (awk exit $awk_status)"
  fi

  cp "$compose_path" "${compose_path}.bak.$(date +%Y%m%d-%H%M%S)"
  mv "$tmp" "$compose_path"
}

is_running() {
  run_compose ps --status running -q 2>/dev/null | grep -q .
}

stop_services() {
  info "Stopping services with docker compose down (without -v)..."
  run_compose down
}

start_services() {
  info "Starting services..."
  run_compose up -d
}

backup_local_files() {
  local backup_dir="$1"
  local archive="$backup_dir/local-files.tar.gz"
  local items=()

  for item in ".env" "$COMPOSE_FILE" "data" "postgres_data" "redis_data" "config.yaml"; do
    if [ -e "$DEPLOY_DIR/$item" ]; then
      items+=("$item")
    fi
  done

  if [ "${#items[@]}" -eq 0 ]; then
    warn "No local files/directories found to tar"
    return
  fi

  info "Backing up local files/directories to $archive"
  tar czf "$archive" -C "$DEPLOY_DIR" "${items[@]}"
}

backup_named_volumes() {
  local backup_dir="$1"
  local project="$2"
  local volumes

  volumes="$(run_compose config --volumes 2>/dev/null || true)"
  if [ -z "$volumes" ]; then
    return
  fi

  while IFS= read -r volume; do
    [ -n "$volume" ] || continue
    local actual="${project}_${volume}"
    if ! docker volume inspect "$actual" >/dev/null 2>&1; then
      continue
    fi
    info "Backing up Docker volume $actual"
    docker run --rm \
      -v "$actual:/volume:ro" \
      -v "$backup_dir:/backup" \
      alpine:3.20 \
      sh -c "cd /volume && tar czf /backup/volume-${actual}.tar.gz ."
  done <<< "$volumes"
}

list_backups() {
  local backup_root="$DEPLOY_DIR/backups"
  [ -d "$backup_root" ] || return 0
  ls -1d "$backup_root"/backup-* 2>/dev/null | sort
}

validate_backup_prerequisites() {
  local backup_root="$DEPLOY_DIR/backups"
  local compose_path="$DEPLOY_DIR/$COMPOSE_FILE"
  local item

  info "Validating backup prerequisites..."

  [ -d "$DEPLOY_DIR" ] || die "Deployment directory not found: $DEPLOY_DIR"
  [ -f "$compose_path" ] || die "Compose file not found: $compose_path"
  [ -r "$compose_path" ] || die "Compose file is not readable: $compose_path"

  mkdir -p "$backup_root"
  [ -d "$backup_root" ] || die "Backup root is not a directory: $backup_root"
  [ -w "$backup_root" ] || die "Backup root is not writable: $backup_root"

  for item in ".env" "$COMPOSE_FILE" "data" "postgres_data" "redis_data" "config.yaml"; do
    if [ -e "$DEPLOY_DIR/$item" ] && [ ! -r "$DEPLOY_DIR/$item" ]; then
      die "Backup source is not readable: $DEPLOY_DIR/$item"
    fi
  done

  if ! run_compose config >/dev/null; then
    die "Docker Compose config validation failed for $compose_path"
  fi

  info "Backup prerequisite validation passed."
}

prune_old_backups() {
  local backup_root="$DEPLOY_DIR/backups"
  local backups=()
  local count remove_count i

  if ! [[ "$MAX_BACKUPS" =~ ^[0-9]+$ ]] || [ "$MAX_BACKUPS" -lt 1 ]; then
    warn "Invalid MAX_BACKUPS=$MAX_BACKUPS; skip backup pruning"
    return
  fi

  [ -d "$backup_root" ] || return

  while IFS= read -r backup; do
    [ -n "$backup" ] || continue
    backups+=("$backup")
  done < <(list_backups)

  count="${#backups[@]}"
  if [ "$count" -le "$MAX_BACKUPS" ]; then
    info "Backup retention: $count/$MAX_BACKUPS backup(s), no pruning needed."
    return
  fi

  remove_count=$((count - MAX_BACKUPS))
  info "Backup retention: $count/$MAX_BACKUPS backup(s), pruning $remove_count old backup(s)."
  for ((i = 0; i < remove_count; i++)); do
    info "Deleting old backup: ${backups[$i]}"
    rm -rf "${backups[$i]}"
  done
}

choose_backup() {
  local backups=()
  while IFS= read -r backup; do
    [ -n "$backup" ] || continue
    backups+=("$backup")
  done < <(list_backups)

  echo "[DEBUG] Backup root: $DEPLOY_DIR/backups" >&2
  echo "[DEBUG] Found ${#backups[@]} backup(s)" >&2

  if [ "${#backups[@]}" -eq 0 ]; then
    die "No backups found under $DEPLOY_DIR/backups"
  fi

  while true; do
    echo "" >&2
    echo "Available backups:" >&2
    local i
    for i in "${!backups[@]}"; do
      printf "  %d) %s\n" "$((i + 1))" "${backups[$i]}" >&2
    done
    echo "  q) Back to menu" >&2
    echo "" >&2
    printf "Choose backup number: " >&2
    read -r choice

    case "$choice" in
      q|Q)
        return 1
        ;;
      '')
        echo "Please input a backup number, or q to return." >&2
        continue
        ;;
    esac

    if ! [[ "$choice" =~ ^[0-9]+$ ]]; then
      echo "Invalid backup number: $choice" >&2
      continue
    fi
    if [ "$choice" -lt 1 ] || [ "$choice" -gt "${#backups[@]}" ]; then
      echo "Invalid backup number: $choice" >&2
      continue
    fi

    echo "${backups[$((choice - 1))]}"
    return 0
  done
}

restore_local_files() {
  local backup_dir="$1"
  local archive="$backup_dir/local-files.tar.gz"

  if [ ! -f "$archive" ]; then
    warn "No local-files.tar.gz found in $backup_dir"
    return
  fi

  info "Removing current local data paths before restore..."
  for item in ".env" "$COMPOSE_FILE" "data" "postgres_data" "redis_data" "config.yaml"; do
    if [ -e "$DEPLOY_DIR/$item" ]; then
      rm -rf "$DEPLOY_DIR/$item"
    fi
  done

  info "Restoring local files/directories from $archive"
  tar xzf "$archive" -C "$DEPLOY_DIR"
}

restore_named_volumes() {
  local backup_dir="$1"
  local found=0

  for archive in "$backup_dir"/volume-*.tar.gz; do
    [ -f "$archive" ] || continue
    found=1

    local base="${archive##*/}"
    local volume="${base#volume-}"
    volume="${volume%.tar.gz}"

    if ! docker volume inspect "$volume" >/dev/null 2>&1; then
      info "Creating Docker volume $volume"
      docker volume create "$volume" >/dev/null
    fi

    info "Restoring Docker volume $volume"
    docker run --rm \
      -v "$volume:/volume" \
      -v "$backup_dir:/backup:ro" \
      alpine:3.20 \
      sh -c "find /volume -mindepth 1 -maxdepth 1 -exec rm -rf {} + && cd /volume && tar xzf /backup/$base"
  done

  if [ "$found" = "0" ]; then
    info "No Docker volume backup archives found in $backup_dir"
  fi
}

restore_backup() {
  local backup_dir="${1:-}"
  if [ -z "$backup_dir" ]; then
    if ! backup_dir="$(choose_backup)"; then
      info "Cancelled"
      return
    fi
  fi
  [ -d "$backup_dir" ] || die "Backup directory not found: $backup_dir"

  echo ""
  echo "Backup to restore:"
  echo "  $backup_dir"
  echo ""
  warn "Restore will stop services and overwrite current deployment data."
  warn "A safety backup of current data will be created before restore."
  if ! confirm "Continue restore?"; then
    info "Cancelled"
    return
  fi

  info "Creating safety backup before restore..."
  backup_data 0

  info "Restoring selected backup..."
  stop_services
  restore_local_files "$backup_dir"
  restore_named_volumes "$backup_dir"
  start_services
  info "Restore completed."
}

backup_data() {
  local restart_after="${1:-0}"
  local was_running=0

  validate_backup_prerequisites

  if is_running; then
    was_running=1
  fi

  stop_services

  local backup_root="$DEPLOY_DIR/backups"
  local backup_dir="$backup_root/backup-$(date +%Y%m%d-%H%M%S)"
  mkdir -p "$backup_dir"

  backup_local_files "$backup_dir"
  backup_named_volumes "$backup_dir" "$(compose_project_name)"

  info "Backup completed: $backup_dir"
  prune_old_backups

  if [ "$restart_after" = "1" ] && [ "$was_running" = "1" ]; then
    start_services
  fi
}

status() {
  echo "Deploy dir:   $DEPLOY_DIR"
  echo "Compose file: $COMPOSE_FILE"
  echo "Image:        $(current_image || true)"
  echo ""
  run_compose ps || true
}

follow_logs() {
  run_compose logs -f sub2api
}

sql_quote_literal() {
  printf "%s" "$1" | sed "s/'/''/g"
}

postgres_exec_sql() {
  local sql="$1"
  local db_user db_name
  db_user="$(parse_env_value POSTGRES_USER "$DEPLOY_DIR/.env" || true)"
  db_name="$(parse_env_value POSTGRES_DB "$DEPLOY_DIR/.env" || true)"
  db_user="${db_user:-sub2api}"
  db_name="${db_name:-sub2api}"

  if run_compose ps --services --status running | grep -qx "postgres"; then
    run_compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$db_user" -d "$db_name" -c "$sql"
    return
  fi

  info "Postgres service is not running; starting postgres temporarily..."
  run_compose up -d postgres
  info "Waiting for postgres to become ready..."
  local i
  for i in $(seq 1 30); do
    if run_compose exec -T postgres pg_isready -U "$db_user" -d "$db_name" >/dev/null 2>&1; then
      run_compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$db_user" -d "$db_name" -c "$sql"
      return
    fi
    sleep 2
  done
  die "Postgres did not become ready"
}

reset_admin_password() {
  local email password email_sql password_sql

  echo ""
  printf "Admin email [admin@sub2api.local]: "
  read -r email
  email="${email:-admin@sub2api.local}"

  echo ""
  echo "Input new password, or press Enter to use temporary password: ChangeMe123!"
  printf "New password: "
  read -r password
  password="${password:-ChangeMe123!}"

  if [ "${#password}" -lt 6 ]; then
    echo "Password must be at least 6 characters."
    return
  fi

  echo ""
  echo "This will reset password for:"
  echo "  $email"
  echo "It will also disable TOTP for this account."
  if ! confirm "Continue?"; then
    info "Cancelled"
    return
  fi

  info "Creating safety backup before password reset..."
  backup_data 0

  email_sql="$(sql_quote_literal "$email")"
  password_sql="$(sql_quote_literal "$password")"

  postgres_exec_sql "
CREATE EXTENSION IF NOT EXISTS pgcrypto;
UPDATE users
SET password_hash = crypt('${password_sql}', gen_salt('bf')),
    totp_secret_encrypted = NULL,
    totp_enabled = FALSE,
    totp_enabled_at = NULL,
    updated_at = NOW()
WHERE lower(email) = lower('${email_sql}')
  AND deleted_at IS NULL
RETURNING id, email, role, status, totp_enabled, updated_at;
"

  echo ""
  info "Password reset completed."
  echo "Login email:    $email"
  echo "Login password: $password"
  echo "Please log in and change this password immediately."
}

switch_image() {
  local target="$1"
  local image="$2"

  echo "Target:        $target"
  echo "Deploy dir:    $DEPLOY_DIR"
  echo "Compose file:  $COMPOSE_FILE"
  echo "Current image: $(current_image || true)"
  echo "New image:     $image"
  echo ""

  if ! confirm "This will stop services, backup data, switch image, pull, and restart. Continue?"; then
    info "Cancelled"
    return
  fi

  backup_data 0

  info "Updating sub2api image in $COMPOSE_FILE"
  set_sub2api_image "$image"

  info "Pulling sub2api image..."
  run_compose pull sub2api

  start_services
  info "Done."
}

update_current_image() {
  local current
  local source
  local latest_image
  local detect_status

  set +e
  source="$(detect_current_source)"
  detect_status=$?
  set -e
  current="$(normalize_image_name "$(current_image || true)")"

  if [ "$detect_status" -eq 1 ]; then
    die "Cannot detect current sub2api image from $DEPLOY_DIR/$COMPOSE_FILE"
  fi
  if [ "$detect_status" -eq 2 ]; then
    die "Current image is not recognized as fork or official: $current"
  fi

  case "$source" in
    fork)
      latest_image="$FORK_IMAGE"
      ;;
    official)
      latest_image="$OFFICIAL_IMAGE"
      ;;
    *)
      die "Unknown image source: $source"
      ;;
  esac

  echo "Deploy dir:    $DEPLOY_DIR"
  echo "Compose file:  $COMPOSE_FILE"
  echo "Current image: $current"
  case "$source" in
    fork)
      echo "Source:        my fork"
      ;;
    official)
      echo "Source:        official"
      ;;
  esac
  echo "Latest image:  $latest_image"
  echo ""

  if ! confirm "This will stop services, switch to latest image for the current source, pull, and restart. Continue?"; then
    info "Cancelled"
    return
  fi

  if confirm_default_yes "Create a backup before update?"; then
    backup_data 0
  else
    warn "Skipping backup before update by user choice."
    stop_services
  fi

  info "Updating sub2api image in $COMPOSE_FILE"
  set_sub2api_image "$latest_image"

  info "Pulling latest sub2api image: $latest_image"
  run_compose pull sub2api

  start_services
  info "Update completed."
}

ensure_ready() {
  DEPLOY_DIR="$(cd "$DEPLOY_DIR" && pwd)"
  [ -f "$DEPLOY_DIR/$COMPOSE_FILE" ] || die "Compose file not found: $DEPLOY_DIR/$COMPOSE_FILE"
  [ -f "$DEPLOY_DIR/.env" ] || warn ".env not found in $DEPLOY_DIR; compose may still work if env vars are already exported"
  command -v docker >/dev/null 2>&1 || die "docker is not installed"
}

ask_deploy_dir() {
  if [ -f "$DEPLOY_DIR/$COMPOSE_FILE" ]; then
    DEPLOY_DIR="$(cd "$DEPLOY_DIR" && pwd)"
    return
  fi

  while true; do
    echo ""
    echo "Current deployment directory:"
    echo "  $DEPLOY_DIR"
    echo "Compose file:"
    echo "  $COMPOSE_FILE"
    echo ""
    printf "Input deployment directory path, or press Enter to use current directory: "
    read -r input
    if [ -n "$input" ]; then
      DEPLOY_DIR="$input"
    fi
    if [ -f "$DEPLOY_DIR/$COMPOSE_FILE" ]; then
      DEPLOY_DIR="$(cd "$DEPLOY_DIR" && pwd)"
      break
    fi
    echo "Cannot find $DEPLOY_DIR/$COMPOSE_FILE"
  done
}

menu() {
  ask_deploy_dir
  ensure_ready

  while true; do
    echo ""
    echo "=============================================="
    echo " Sub2API deployment source switcher"
    echo "=============================================="
    echo "Deploy dir:   $DEPLOY_DIR"
    echo "Compose file: $COMPOSE_FILE"
    echo "Current image: $(current_image || true)"
    echo ""
    echo "1) View status"
    echo "2) Backup data only"
    echo "3) Restore data from backup"
    echo "4) Switch to my fork image    ($FORK_IMAGE)"
    echo "5) Switch back to official    ($OFFICIAL_IMAGE)"
    echo "6) Update current image to latest"
    echo "7) Reset admin password"
    echo "8) Follow sub2api logs"
    echo "9) Change deployment directory"
    echo "0) Exit"
    echo ""
    printf "Choose: "
    read -r choice

    case "$choice" in
      1) status; pause ;;
      2)
        if confirm "Backup will stop services temporarily. Continue?"; then
          backup_data 1
        else
          info "Cancelled"
        fi
        pause
        ;;
      3) restore_backup; pause ;;
      4) switch_image "fork" "$FORK_IMAGE"; pause ;;
      5) switch_image "official" "$OFFICIAL_IMAGE"; pause ;;
      6) update_current_image; pause ;;
      7) reset_admin_password; pause ;;
      8) follow_logs ;;
      9) ask_deploy_dir; ensure_ready ;;
      0) exit 0 ;;
      *) echo "Invalid choice"; pause ;;
    esac
  done
}

COMMAND=""
if [ "$#" -gt 0 ] && [[ "${1:-}" != --* ]] && [[ "${1:-}" != -* ]]; then
  COMMAND="$1"
  shift
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir)
      DEPLOY_DIR="${2:-}"
      shift 2
      ;;
    --compose)
      COMPOSE_FILE="${2:-}"
      shift 2
      ;;
    --max-backups)
      MAX_BACKUPS="${2:-}"
      shift 2
      ;;
    -y|--yes)
      YES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown option: $1"
      ;;
  esac
done

if [ -z "$COMMAND" ]; then
  menu
fi

ensure_ready

case "$COMMAND" in
  fork)
    switch_image "fork" "$FORK_IMAGE"
    ;;
  official)
    switch_image "official" "$OFFICIAL_IMAGE"
    ;;
  update)
    update_current_image
    ;;
  backup)
    if confirm "Backup will stop services temporarily. Continue?"; then
      backup_data 1
    else
      info "Cancelled"
    fi
    ;;
  restore)
    restore_backup
    ;;
  reset-admin)
    reset_admin_password
    ;;
  status)
    status
    ;;
  logs)
    follow_logs
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage
    die "Unknown command: $COMMAND"
    ;;
esac
