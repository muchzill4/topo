#!/usr/bin/env bash
set -euo pipefail

examples_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output_directory="$examples_directory/bin"
mkdir -p "$output_directory"

cd "$examples_directory"
executable_suffix="$(go env GOEXE)"

for name in cpu memory private; do
  plugin_name="topo-plugin-$name"
  printf 'Building %s\n' "$plugin_name"
  go build -o "$output_directory/$plugin_name$executable_suffix" \
    "./$name/cmd/$plugin_name"
done

printf 'Plugins built in %s\n' "$output_directory"
