#!/usr/bin/env sh
#
# stf_sender.sh - Sends .bin or .json STF files to a remote endpoint in either
# fuzz or validate mode, parsing the JSON response.
#
# Usage:
#   ./stf_sender.sh -d <directory> [-f bin|json] [-e endpoint] [-m fuzz|validate] [-v] [-h]

usage() {
  echo "Usage: $0 -d <dir> [-f bin|json] [-e endpoint] [-m fuzz|validate] [-v] [-h]"
  echo "  -d <dir>      Target directory (required)."
  echo "  -f <format>   \"bin\" or \"json\" (default: bin)."
  echo "  -e <endpoint> Base URL (default: http://localhost:8088)."
  echo "  -m <mode>     \"fuzz\" or \"validate\" (default: fuzz)."
  echo "  -v            Verbose output: show the full server response."
  echo "  -h            Show help message."
}

TARGET_DIR=""
FORMAT="bin"
ENDPOINT="http://localhost:8088"
MODE="fuzz"
PRINT_RESPONSE="false"

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

GREEN="$(printf '\033[0;32m')"
RED="$(printf '\033[0;31m')"
RESET="$(printf '\033[0m')"
print_line() { printf "%s\n" "$1"; }

print_line "======================================="
print_line "    STF Sender - $MODE Mode"
print_line "---------------------------------------"
print_line " Directory:   $TARGET_DIR"
print_line " Format:      $FORMAT"
print_line " Endpoint:    $ENDPOINT"
print_line " Mode:        $MODE"
print_line " Verbose?:    $PRINT_RESPONSE"
print_line "======================================="
print_line ""

FILES="$(find "$TARGET_DIR" -type f -name "*.$EXTENSION" 2>/dev/null)"
[ -z "$FILES" ] && {
  print_line "No '.$EXTENSION' files found in '$TARGET_DIR'. Exiting."
  exit 0
}

TOTAL_FILES="$(printf "%s\n" "$FILES" | wc -l | awk '{print $1}')"
print_line "Found $TOTAL_FILES '.$EXTENSION' file(s) in '$TARGET_DIR'."
print_line "$MODE start..."

success_count=0
fail_count=0

for file in $FILES; do
  print_line "--------------------------------------"
  CURL_OUTPUT="$(mktemp)"
  HTTP_CODE="$(curl -s -w "%{http_code}" -o "$CURL_OUTPUT" \
                   -X POST "${ENDPOINT}/${MODE}" \
                   -H "Content-Type: $CONTENT_TYPE" \
                   --data-binary "@$file")"

  [ $? -ne 0 ] && {
    print_line "Sending STF: $file  ---> ${RED}Failed (cURL error).${RESET}"
    fail_count=$((fail_count+1))
    [ "$PRINT_RESPONSE" = "true" ] && cat "$CURL_OUTPUT"
    rm -f "$CURL_OUTPUT"
    continue
  }

  RESPONSE="$(cat "$CURL_OUTPUT")"
  rm -f "$CURL_OUTPUT"

  [ "$HTTP_CODE" -lt 200 ] || [ "$HTTP_CODE" -ge 300 ] && {
    print_line "Sending STF: $file  ---> ${RED}Failed (HTTP $HTTP_CODE).${RESET}"
    fail_count=$((fail_count+1))
    [ "$PRINT_RESPONSE" = "true" ] && print_line "$RESPONSE"
    continue
  }

  if [ "$MODE" = "fuzz" ]; then
    echo "$RESPONSE" | grep -q '"Mutated":true'
    GRPT=$?
    echo "$RESPONSE" | grep -q '"Mutated":false'
    GRPF=$?

    if [ "$GRPT" -eq 0 ]; then
      print_line "Sending STF: $file  ---> ${GREEN}Fuzzed! (Mutated=true)${RESET}"
      success_count=$((success_count+1))
    elif [ "$GRPF" -eq 0 ]; then
      print_line "Sending STF: $file  ---> ${RED}Not-Fuzzable (Mutated=false)${RESET}"
      fail_count=$((fail_count+1))
    else
      print_line "Sending STF: $file  ---> ${RED}Unknown (no 'Mutated' field)${RESET}"
      fail_count=$((fail_count+1))
    fi
  else
    echo "$RESPONSE" | grep -q '"valid":true'
    V_OK=$?
    echo "$RESPONSE" | grep -q '"valid":false'
    V_BAD=$?

    if [ "$V_OK" -eq 0 ]; then
      print_line "Validating STF: $file  ---> ${GREEN}Valid! (valid=true)${RESET}"
      success_count=$((success_count+1))
    elif [ "$V_BAD" -eq 0 ]; then
      error_msg="$(echo "$RESPONSE" | sed -n 's/.*"error":"\([^"]*\)".*/\1/p')"
      print_line "Validating STF: $file  ---> ${RED}Invalid (valid=false)${RESET}"
      [ -n "$error_msg" ] && print_line "Reason: $error_msg"
      fail_count=$((fail_count+1))
    else
      print_line "Validating STF: $file  ---> ${RED}Unknown (no 'valid' field)${RESET}"
      fail_count=$((fail_count+1))
    fi
  fi

  [ "$PRINT_RESPONSE" = "true" ] && print_line "$RESPONSE"
  print_line ""
done

print_line "--------------------------------------"
if [ "$MODE" = "fuzz" ]; then
  printf "Fuzz complete on %d '.%s' file(s) in '%s'.\n" \
         "$TOTAL_FILES" "$EXTENSION" "$TARGET_DIR"
  printf "Fuzzed: %b%d%b, Not-Fuzzable/Unknown: %b%d%b\n" \
         "$GREEN" "$success_count" "$RESET" \
         "$RED"   "$fail_count"   "$RESET"
else
  printf "Validation complete on %d '.%s' file(s) in '%s'.\n" \
         "$TOTAL_FILES" "$EXTENSION" "$TARGET_DIR"
  printf "Valid: %b%d%b, Invalid/Unknown: %b%d%b\n" \
         "$GREEN" "$success_count" "$RESET" \
         "$RED"   "$fail_count"   "$RESET"
fi

exit 0
