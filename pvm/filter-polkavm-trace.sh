#!/usr/bin/env bash
# Usage: ./filter-trace.sh [logfile]
# If no logfile is given, reads from stdin.

while IFS= read -r line; do
  # match TRACE polkavm::interpreter anywhere in the line
  if [[ $line =~ TRACE[[:space:]]+polkavm::interpreter[[:space:]]+(.+) ]]; then
    out="${BASH_REMATCH[1]}"
    # only echo if there's an " = " in the output
    if [[ $out == *" = "* ]]; then
      echo "$out"
    fi
  fi
done < "${1:-/dev/stdin}"
