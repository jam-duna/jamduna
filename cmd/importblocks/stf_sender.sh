#!/usr/bin/env sh

usage() {
  echo "Usage: $0 -d <dir> [-f bin|json] [-e endpoint] [-m fuzz|validate] [-v] [-h]"
  echo "  -d <dir>      Target directory (required)."
  echo "  -f <format>   \"bin\" or \"json\" (default: bin)."
  echo "  -e <endpoint> Base URL (default: http://localhost:8088)."
  echo "  -m <mode>     \"fuzz\" or \"validate\" (default: fuzz)."
  echo "  -v            Verbose output: show full server response."
  echo "  -h            Show help message."
}

# Defaults
TARGET_DIR=""
FORMAT="bin"
ENDPOINT="http://localhost:8088"
MODE="fuzz"
PRINT_RESPONSE="false"

# Parse args
while getopts ":d:f:e:m:vh" opt; do
  case "$opt" in
    d) TARGET_DIR="$OPTARG" ;;
    f) FORMAT="$OPTARG" ;;
    e) ENDPOINT="$OPTARG" ;;
    m) MODE="$OPTARG" ;;
    v) PRINT_RESPONSE="true" ;;
    h) usage; exit 0 ;;
    \?) echo "Invalid option: -$OPTARG" >&2; usage; exit 1 ;;
  esac
done
shift $((OPTIND - 1))

[ -z "$TARGET_DIR" ] && { echo "Error: -d <dir> is required."; usage; exit 1; }

# File type config
case "$FORMAT" in
  bin) CONTENT_TYPE="application/octet-stream"; EXTENSION="bin" ;;
  json) CONTENT_TYPE="application/json"; EXTENSION="json" ;;
  *) echo "Invalid format: $FORMAT (must be 'bin' or 'json')."; exit 1 ;;
esac

[ "$MODE" != "fuzz" ] && [ "$MODE" != "validate" ] && {
  echo "Invalid mode: $MODE (must be 'fuzz' or 'validate')."
  exit 1
}

[ ! -d "$TARGET_DIR" ] && {
  echo "Error: '$TARGET_DIR' is not a valid directory."
  exit 1
}

# Colors
GREEN="\033[0;32m"
RED="\033[0;31m"
RESET="\033[0m"

# Output header
echo "======================================="
echo "    STF Sender - $MODE Mode"
echo "---------------------------------------"
echo " Directory:   $TARGET_DIR"
echo " Format:      $FORMAT"
echo " Endpoint:    $ENDPOINT"
echo " Mode:        $MODE"
echo " Verbose?:    $PRINT_RESPONSE"
echo "======================================="
echo ""

FILES="$(find "$TARGET_DIR" -type f -name "*.$EXTENSION" 2>/dev/null)"
[ -z "$FILES" ] && {
  echo "No '.$EXTENSION' files found in '$TARGET_DIR'. Exiting."
  exit 0
}

TOTAL_FILES=$(echo "$FILES" | wc -l | awk '{print $1}')
echo "Found $TOTAL_FILES '.$EXTENSION' file(s)."
echo "$MODE start..."

success_count=0
fail_count=0

for file in $FILES; do
  echo "--------------------------------------"
  echo "Processing: $file"

  CURL_OUTPUT="$(mktemp)"
  HTTP_CODE="$(curl -s -w "%{http_code}" -o "$CURL_OUTPUT" \
    -X POST "$ENDPOINT/$MODE" \
    -H "Content-Type: $CONTENT_TYPE" \
    --data-binary "@$file")"

  RESPONSE="$(cat "$CURL_OUTPUT")"
  rm -f "$CURL_OUTPUT"

  if [ "$HTTP_CODE" -lt 200 ] || [ "$HTTP_CODE" -ge 300 ]; then
    echo "  ---> ${RED}Failed (HTTP $HTTP_CODE)${RESET}"
    error_msg=$(echo "$RESPONSE" | sed -n 's/.*"error":"\([^"]*\)".*/\1/p')
    [ -n "$error_msg" ] && echo "        Reason: $error_msg"
    fail_count=$((fail_count + 1))
    [ "$PRINT_RESPONSE" = "true" ] && echo "$RESPONSE"
    continue
  fi

  if [ "$MODE" = "fuzz" ]; then
    echo "$RESPONSE" | grep -q '"Mutated":true'
    if [ $? -eq 0 ]; then
      echo "  ---> ${GREEN}Fuzzed! (Mutated=true)${RESET}"
      success_count=$((success_count + 1))
    elif echo "$RESPONSE" | grep -q '"Mutated":false'; then
      echo "  ---> ${RED}Not-Fuzzable (Mutated=false)${RESET}"
      fail_count=$((fail_count + 1))
    else
      echo "  ---> ${RED}Unknown (no 'Mutated' field)${RESET}"
      fail_count=$((fail_count + 1))
    fi
  else
    if echo "$RESPONSE" | grep -q '"state_root"'; then
      state_root=$(echo "$RESPONSE" | sed -n 's/.*"state_root":"\([^"]*\)".*/\1/p')
      echo "  ---> ${GREEN}Valid STF (has state_root)${RESET}"
      [ -n "$state_root" ] && echo "        State Root: $state_root"
      success_count=$((success_count + 1))

    elif echo "$RESPONSE" | grep -q '"error"'; then
      error_msg=$(echo "$RESPONSE" | sed -n 's/.*"error":"\([^"]*\)".*/\1/p')
      echo "  ---> ${RED}Invalid STF (error returned)${RESET}"
      [ -n "$error_msg" ] && echo "        Reason: $error_msg"
      fail_count=$((fail_count + 1))

    elif echo "$RESPONSE" | grep -q '"valid":false'; then
      echo "  ---> ${RED}Invalid (valid=false)${RESET}"
      fail_count=$((fail_count + 1))

    else
      echo "  ---> ${RED}Unknown (no 'state_root' or 'error')${RESET}"
      fail_count=$((fail_count + 1))
    fi
  fi

  [ "$PRINT_RESPONSE" = "true" ] && echo "$RESPONSE"
  echo ""
done

# Summary
echo "--------------------------------------"
if [ "$MODE" = "fuzz" ]; then
  printf "Fuzz complete on %d '.$EXTENSION' file(s).\n" "$TOTAL_FILES"
  printf "  Fuzzed: ${GREEN}%d${RESET}, Not-Fuzzable/Unknown: ${RED}%d${RESET}\n" "$success_count" "$fail_count"
else
  printf "Validation complete on %d '.$EXTENSION' file(s).\n" "$TOTAL_FILES"
  printf "  Valid: ${GREEN}%d${RESET}, Invalid/Unknown: ${RED}%d${RESET}\n" "$success_count" "$fail_count"
fi

exit 0