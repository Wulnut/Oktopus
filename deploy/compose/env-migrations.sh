#!/bin/bash

# Apply an exact environment default change once. The marker prevents a later
# operator override from being mistaken for the original default.
migrate_env_default_once() {
  if [ "$#" -ne 6 ]; then
    printf 'migrate_env_default_once: expected 6 arguments\n' >&2
    return 2
  fi

  local target_file="$1"
  local marker_file="$2"
  local migration_id="$3"
  local key="$4"
  local old_value="$5"
  local new_value="$6"
  local line temp_file replaced=0

  if [ -z "$target_file" ] || [ -z "$marker_file" ] || [ -z "$migration_id" ] \
    || [ -z "$key" ] || [ -z "$old_value" ] || [ -z "$new_value" ]; then
    printf 'migrate_env_default_once: arguments must be non-empty\n' >&2
    return 2
  fi
  if [ ! -f "$target_file" ]; then
    printf 'migrate_env_default_once: target file not found: %s\n' "$target_file" >&2
    return 1
  fi
  if [ -f "$marker_file" ] && grep -Fqx -- "$migration_id" "$marker_file"; then
    return 0
  fi

  temp_file="${target_file}.migration.$$"
  if ! cp -p -- "$target_file" "$temp_file"; then
    return 1
  fi
  : > "$temp_file"

  while IFS= read -r line || [ -n "$line" ]; do
    if [ "$line" = "${key}=${old_value}" ]; then
      printf '%s=%s\n' "$key" "$new_value" >> "$temp_file"
      replaced=1
    else
      printf '%s\n' "$line" >> "$temp_file"
    fi
  done < "$target_file"

  if [ "$replaced" -eq 1 ]; then
    if ! mv -- "$temp_file" "$target_file"; then
      rm -f -- "$temp_file"
      return 1
    fi
  else
    rm -f -- "$temp_file"
  fi

  mkdir -p -- "$(dirname "$marker_file")"
  printf '%s\n' "$migration_id" >> "$marker_file"
}
