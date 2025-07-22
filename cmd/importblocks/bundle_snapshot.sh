#!/bin/bash
set -e
shopt -s nullglob

# --- Default exclusions ---
INCLUDE_HEX=false
INCLUDE_FOLLOWER=false

# --- Flag parsing ---
while [[ $1 == --* ]]; do
  case $1 in
    --hex)
      INCLUDE_HEX=true
      ;;
    --follower)
      INCLUDE_FOLLOWER=true
      ;;
    *)
      echo "Unknown flag: $1"
      echo "Usage: $0 [--hex] [--follower]"
      exit 1
      ;;
  esac
  shift
done

# --- Env checks ---
[ -n "${JAM_PATH}" ]        || { echo "Error: JAM_PATH not set."; exit 1; }
[ -n "${JAMTEST_DATADIR}" ] || { echo "Error: JAMTEST_DATADIR not set."; exit 1; }

DATADIR="${JAMTEST_DATADIR}"
DEST_DIR="${JAM_PATH}/node/test"

# --- JOBID prompt ---
read -p "Please enter the JOBID: " JOBID
[ -n "${JOBID}" ] || { echo "Error: No JOBID entered."; exit 1; }
echo "Using JOBID: ${JOBID}"

# --- Prepare DEST_DIR ---
mkdir -p "${DEST_DIR}"
find "${DEST_DIR}" -mindepth 1 -delete
echo "Destination cleaned."

# --- 1) Copy bundle_snapshots & collect prefixes ---
prefixes=()
for i in {0..5}; do
  BS_DIR="${DATADIR}/${JOBID}/node${i}/data/bundle_snapshots"
  [ -d "${BS_DIR}" ] || continue

  echo "Node ${i}: copying bundle_snapshots..."
  for f in "${BS_DIR}"/*; do
    base=$(basename "$f")
    ext="${base##*.}"
    [[ "$ext" == "hex" && "$INCLUDE_HEX" == false ]] && continue
    [[ "$base" == *"_guarantor_follower."* && "$INCLUDE_FOLLOWER" == false ]] && continue
    cp -f "$f" "${DEST_DIR}/"
  done

  for g in "${BS_DIR}"/*_guarantor_*.bin; do
    pre="${g##*/}"
    pre="${pre%.bin}"
    pre="${pre%%_*}"
    prefixes+=("$pre")
  done
done

# --- 2) Dedupe prefixes ---
unique_prefixes=($(printf "%s\n" "${prefixes[@]}" | sort -u))

# --- 3) Copy state_transitions for each prefix ---
for pre in "${unique_prefixes[@]}"; do
  echo "→ Block ${pre}: copying state_transitions..."
  for i in {0..5}; do
    ST_DIR="${DATADIR}/${JOBID}/node${i}/data/state_transitions"
    [ -d "${ST_DIR}" ] || continue
    for suf in bin hex json; do
      [[ "$suf" == "hex" && "$INCLUDE_HEX" == false ]] && continue
      src="${ST_DIR}/${pre}.${suf}"
      [ -f "${src}" ] && cp -f "${src}" "${DEST_DIR}/"
    done
  done
done

echo "Done! Files are in: ${DEST_DIR}"