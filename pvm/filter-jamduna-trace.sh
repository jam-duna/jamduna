#!/usr/bin/env bash
# Usage: ./filter-trace.sh [logfile]
# If no logfile is given, reads from stdin.

while IFS= read -r line; do
  # capture everything after "TRACE polkavm::interpreter "
  if [[ $line =~ TRACE[[:space:]]+polkavm::interpreter[[:space:]]+(.+) ]]; then
    out="${BASH_REMATCH[1]}"
    # only echo if there's an " = " in that suffix
    if [[ $out == *" = "* ]]; then
      echo "$out"
    fi
  fi
done < "${1:-/dev/stdin}"
