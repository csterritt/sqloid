#!/usr/bin/env bash
# tar-new.sh: Create a tar archive of git-tracked new/modified files
# Usage: tar-new.sh <archive-name.tar[.gz|.bz2|.xz]>

set -euo pipefail

# Validate arguments
if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <archive-name.tar[.gz|.bz2|.xz]>" >&2
  exit 1
fi

archive_name="$1"

# Validate archive name has .tar suffix
if [[ ! "$archive_name" =~ ^.*\.tar(\.(gz|bz2|xz))?$ ]]; then
  echo "Error: Archive name must end with .tar, .tar.gz, .tar.bz2, or .tar.xz" >&2
  exit 1
fi

# Detect compression from extension
compress_flag=""
case "$archive_name" in
  *.tar.gz)  compress_flag="z" ;;
  *.tar.bz2) compress_flag="j" ;;
  *.tar.xz)  compress_flag="J" ;;
  *.tar)     compress_flag="" ;;
esac

# Get list of new/modified files (excluding deleted files)
# --diff-filter=ACMR: Added, Copied, Modified, Renamed
# if command -v mapfile >/dev/null 2>&1; then
#   mapfile -t files < <(git diff --name-only --diff-filter=ACMR HEAD)
# else
#   # Fallback for older bash versions
#   files=()
#   while IFS= read -r line; do
#     files+=("$line")
#   done < <(git diff --name-only --diff-filter=ACMR HEAD)
# fi

deleted_files=()
while IFS= read -r line; do
  deleted_files+=("$line")
done < <(git status -sb | sed 1d | egrep '^D |^ D' | /home/chris/chz 1)

files=()
while IFS= read -r line; do
  files+=("$line")
done < <(git status -sb | sed 1d | egrep -v '^D |^ D' | /home/chris/chz 1)

printf '%s\n' "${deleted_files[@]}" > /tmp/deleted-files.txt

if [[ ${#files[@]} -eq 0 ]]; then
  echo "No new or modified files to archive."
  exit 0
fi

echo "Archiving ${#files[@]} file(s):"
printf '  %s\n' "${files[@]}"
echo

# Create the tar archive
tar_cmd=(tar -c${compress_flag}f "$archive_name" -- "${files[@]}")

echo "Running: ${tar_cmd[*]}"
"${tar_cmd[@]}"

echo "Created archive: $archive_name"
