#!/usr/bin/env sh
#
# stf_sender.sh - Sends .bin or .json STF files to a remote endpoint in either
#                 fuzz or validate mode, parsing the JSON response.
#
# Usage:
#   chmod +x stf_sender.sh
#   ./stf_sender.sh -d <directory> [-f bin|json] [-e endpoint] [-m fuzz|validate] [-v] [-h]
#
# Options:
#   -d <dir>      Target directory containing .bin or .json files (required).
#   -f <format>   "bin" or "json" (default: bin).
#   -e <endpoint> Base endpoint URL (default: http://localhost:8088).
#   -m <mode>     "fuzz" or "validate" (default: fuzz).
#   -v            Verbose: print the full server response.
#   -h            Show this help message and exit.
#
# Examples:
#   1) Fuzz .bin files from /tmp/fuzz_transitions (no response printed):
#      ./stf_sender.sh -d /tmp/fuzz_transitions
#
#   2) Validate .json files from /tmp/json_stf against custom endpoint:
#      ./stf_sender.sh -d /tmp/json_stf -f json -m validate -e http://example.com:1234
#
#   3) Same as #2, but print the server response:
#      ./stf_sender.sh -d /tmp/json_stf -f json -m validate -e http://example.com:1234 -v
#

###############################################################################
# 1. Usage Function
###############################################################################
usage() {
  echo "Usage: $0 -d <dir> [-f bin|json] [-e endpoint] [-m fuzz|validate] [-v] [-h]"
  echo "  -d <dir>      Target directory (required)."
  echo "  -f <format>   \"bin\" or \"json\" (default: bin)."
  echo "  -e <endpoint> Base URL (default: http://localhost:8088)."
  echo "  -m <mode>     \"fuzz\" or \"validate\" (default: fuzz)."
  echo "  -v            Verbose output: show the full server response."
  echo "  -h            Show help message."
}

###############################################################################
# 2. Parse Command-Line Options
###############################################################################

# Defaults
TARGET_DIR=""
FORMAT="bin"
ENDPOINT="http://localhost:8088"
MODE="fuzz"
PRINT_RESPONSE="false"

while getopts ":d:f:e:m:vh" opt; do
  case "$opt" in
    d)
      TARGET_DIR="$OPTARG"
      ;;
    f)
      FORMAT="$OPTARG"
      ;;
    e)
      ENDPOINT="$OPTARG"
      ;;
    m)
      MODE="$OPTARG"
      ;;
    v)
      PRINT_RESPONSE="true"
      ;;
    h)
      usage
      exit 0
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      usage
      exit 1
      ;;
  esac
done

shift $((OPTIND - 1))  # Remove parsed options from $@

# Check required directory
if [ -z "$TARGET_DIR" ]; then
  echo "Error: -d <dir> is required."
  usage
  exit 1
fi

###############################################################################
# 3. Validate & Configure Format/Mode
###############################################################################

case "$FORMAT" in
  bin)
    CONTENT_TYPE="application/octet-stream"
    EXTENSION="bin"
    ;;
  json)
    CONTENT_TYPE="application/json"
    EXTENSION="json"
    ;;
  *)
    echo "Invalid format: $FORMAT (must be 'bin' or 'json')."
    exit 1
    ;;
esac

if [ "$MODE" != "fuzz" ] && [ "$MODE" != "validate" ]; then
  echo "Invalid mode: $MODE (must be 'fuzz' or 'validate')."
  exit 1
fi

if [ ! -d "$TARGET_DIR" ]; then
  echo "Error: '$TARGET_DIR' is not a valid directory."
  exit 1
fi

###############################################################################
# 4. ANSI Colors & Print Helpers
###############################################################################

GREEN="$(printf '\033[0;32m')"
RED="$(printf '\033[0;31m')"
RESET="$(printf '\033[0m')"

print_line() {
  printf "%s\n" "$1"
}

###############################################################################
# 5. Intro Banner
###############################################################################

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

###############################################################################
# 6. Gather Files
###############################################################################

FILES="$(find "$TARGET_DIR" -type f -name "*.$EXTENSION" 2>/dev/null)"
if [ -z "$FILES" ]; then
  print_line "No '.$EXTENSION' files found in '$TARGET_DIR'. Exiting."
  exit 0
fi

TOTAL_FILES="$(printf "%s\n" "$FILES" | wc -l | awk '{print $1}')"
print_line "Found $TOTAL_FILES '.$EXTENSION' file(s) in '$TARGET_DIR'."
print_line "$MODE start..."

###############################################################################
# 7. Send & Process Response
###############################################################################

success_count=0
fail_count=0

for file in $FILES; do
  print_line "--------------------------------------"
  # POST to /fuzz or /validate
  # Store both HTTP status code and response body
  CURL_OUTPUT="$(mktemp)"
  HTTP_CODE="$(curl -s -w "%{http_code}" -o "$CURL_OUTPUT" \
                   -X POST "${ENDPOINT}/${MODE}" \
                   -H "Content-Type: $CONTENT_TYPE" \
                   --data-binary "@$file")"

  # Check if curl command itself failed
  if [ $? -ne 0 ]; then
    print_line "Sending STF: $file  ---> ${RED}Failed (cURL error).${RESET}"
    fail_count=$((fail_count+1))
    [ "$PRINT_RESPONSE" = "true" ] && cat "$CURL_OUTPUT"
    rm -f "$CURL_OUTPUT"
    continue
  fi

  RESPONSE="$(cat "$CURL_OUTPUT")"
  rm -f "$CURL_OUTPUT"

  # If HTTP status code is not 2xx, treat as failure
  if [ "$HTTP_CODE" -lt 200 ] || [ "$HTTP_CODE" -ge 300 ]; then
    print_line "Sending STF: $file  ---> ${RED}Failed (HTTP $HTTP_CODE).${RESET}"
    fail_count=$((fail_count+1))
    [ "$PRINT_RESPONSE" = "true" ] && print_line "$RESPONSE"
    continue
  fi

  # Parse JSON according to mode
  if [ "$MODE" = "fuzz" ]; then
    ###########################################################################
    # Fuzz Mode
    # Look for "Mutated":true (success) or "Mutated":false (failure)
    ###########################################################################
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
    ###########################################################################
    # Validate Mode
    # The server returns {"valid":true, ...} on success,
    # or {"valid":false, "error":"..."} on failure.
    ###########################################################################
    echo "$RESPONSE" | grep -q '"valid":true'
    V_OK=$?
    echo "$RESPONSE" | grep -q '"valid":false'
    V_BAD=$?

    if [ "$V_OK" -eq 0 ]; then
      print_line "Validating STF: $file  ---> ${GREEN}Valid! (valid=true)${RESET}"
      success_count=$((success_count+1))
    elif [ "$V_BAD" -eq 0 ]; then
      # optional: parse the error for clarity
      error_msg="$(echo "$RESPONSE" | sed -n 's/.*"error":"\([^"]*\)".*/\1/p')"
      print_line "Validating STF: $file  ---> ${RED}Invalid (valid=false)${RESET}"
      if [ -n "$error_msg" ]; then
        print_line "Reason: $error_msg"
      fi
      fail_count=$((fail_count+1))
    else
      print_line "Validating STF: $file  ---> ${RED}Unknown (no 'valid' field)${RESET}"
      fail_count=$((fail_count+1))
    fi
  fi

  # Print the full response if verbose
  if [ "$PRINT_RESPONSE" = "true" ]; then
    print_line "$RESPONSE"
  fi

  print_line ""
done

###############################################################################
# 8. Final Tally
###############################################################################

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
