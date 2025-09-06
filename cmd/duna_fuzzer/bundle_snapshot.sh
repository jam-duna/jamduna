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

# --- 3) Copy state_transitions for each prefix AND their parents ---
for pre in "${unique_prefixes[@]}"; do
  # Convert prefix to number to calculate parent
  pre_num=$(echo "$pre" | sed 's/^0*//')  # Remove leading zeros
  [ -n "$pre_num" ] || pre_num=0          # Handle "00000000" case
  
  # Calculate parent slot (current - 1)
  if [ "$pre_num" -gt 0 ]; then
    parent_num=$((pre_num - 1))
    parent_pre=$(printf "%08d" "$parent_num")
    
    echo "→ Block ${pre}: copying state_transitions (+ parent ${parent_pre})..."
    
    # Copy both current and parent state transitions
    for target_pre in "$pre" "$parent_pre"; do
      for i in {0..5}; do
        ST_DIR="${DATADIR}/${JOBID}/node${i}/data/state_transitions"
        [ -d "${ST_DIR}" ] || continue
        for suf in bin hex json; do
          [[ "$suf" == "hex" && "$INCLUDE_HEX" == false ]] && continue
          src="${ST_DIR}/${target_pre}.${suf}"
          if [ -f "${src}" ]; then
            cp -f "${src}" "${DEST_DIR}/"
            if [ "$target_pre" = "$parent_pre" ]; then
              echo "   ↳ Copied parent STF: ${target_pre}.${suf}"
            fi
          fi
        done
      done
    done
  else
    echo "→ Block ${pre}: copying state_transitions (no parent for slot 0)..."
    for i in {0..5}; do
      ST_DIR="${DATADIR}/${JOBID}/node${i}/data/state_transitions"
      [ -d "${ST_DIR}" ] || continue
      for suf in bin hex json; do
        [[ "$suf" == "hex" && "$INCLUDE_HEX" == false ]] && continue
        src="${ST_DIR}/${pre}.${suf}"
        [ -f "${src}" ] && cp -f "${src}" "${DEST_DIR}/"
      done
    done
  fi
done

echo "Done! Files are in: ${DEST_DIR}"