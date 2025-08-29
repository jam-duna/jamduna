#!/bin/bash

set -euo pipefail

GITHUB_API_URL="https://api.github.com/repos/paritytech/polkajam-releases/releases"

if [ -z "${JAM_PATH:-}" ]; then
    echo "Error: Environment variable JAM_PATH is not set." >&2
    echo "Please set it to your Jam installation directory, e.g.:" >&2
    echo "  export JAM_PATH=\"\${HOME}/.jam\"" >&2
    exit 1
fi
echo "Using JAM_PATH: $JAM_PATH"

TARGET_DIR="${JAM_TARGET_BIN_DIR:-${JAM_PATH}/bin}"

if [ -z "${POLKAJAM_RELEASES_PATH:-}" ]; then
    echo "Error: Environment variable POLKAJAM_RELEASES_PATH is not set." >&2
    echo "Please set it to your desired archive directory for Polkadot JAM releases, e.g.:" >&2
    echo "  export POLKAJAM_RELEASES_PATH=\"\${HOME}/.polkajam_releases_archive\"" >&2
    exit 1
fi
JAM_RELEASES_ARCHIVE_DIR="${POLKAJAM_RELEASES_PATH}"
echo "Using POLKAJAM_RELEASES_PATH for archive: $JAM_RELEASES_ARCHIVE_DIR"


DOWNLOAD_BASE_DIR="${JAM_DOWNLOAD_DIR:-/tmp}"
EXTRACTION_BASE_DIR="${JAM_EXTRACTION_DIR:-/tmp}"
PLATFORM=""
RUN_SPECIFIC_DOWNLOAD_DIR=""
RUN_SPECIFIC_EXTRACTION_DIR=""
SOURCE_BINARY_NAMES_IN_ARCHIVE=()
TARGET_BINARY_NAMES_IN_DEPLOY_DIR=()
SIMPLIFIED_PLATFORM_ASSET_SUFFIX=""

format_iso_date() {
    local timestamp="$1"
    local formatted_date
    
    if command -v gdate >/dev/null 2>&1; then
        formatted_date=$(gdate -d "$timestamp" "+%Y-%m-%d %H:%M" 2>/dev/null)
    elif date --version 2>/dev/null | grep -q GNU; then
        formatted_date=$(date -d "$timestamp" "+%Y-%m-%d %H:%M" 2>/dev/null)
    else
        local clean_timestamp="${timestamp%Z}"
        formatted_date=$(date -j -f "%Y-%m-%dT%H:%M:%S" "$clean_timestamp" "+%Y-%m-%d %H:%M" 2>/dev/null)
    fi
    if [ -z "$formatted_date" ]; then
        formatted_date="${timestamp%T*} ${timestamp#*T}"
        formatted_date="${formatted_date%Z}"
        formatted_date="${formatted_date%:*}"
    fi
    
    echo "$formatted_date"
}

check_dependencies() {
    local missing_deps=0
    for cmd in curl jq tar uname basename find; do
        if ! command -v "$cmd" &> /dev/null; then
            echo "Error: Required command '$cmd' is not installed." >&2
            missing_deps=1
        fi
    done
    if [ "$missing_deps" -eq 1 ]; then
        exit 1
    fi
}

determine_platform_or_use_env() {
    if [ -n "${JAM_PLATFORM:-}" ]; then
        PLATFORM="${JAM_PLATFORM}"
        echo "Using PLATFORM from environment variable: $PLATFORM"
    else
        local uname_s
        local uname_m
        uname_s=$(uname -s)
        uname_m=$(uname -m)

        if [ "$uname_s" == "Linux" ]; then
            if [ "$uname_m" == "x86_64" ]; then
                PLATFORM="x86_64-unknown-linux-gnu"
            elif [ "$uname_m" == "aarch64" ]; then
                PLATFORM="aarch64-unknown-linux-gnu"
            else
                echo "Error: Unsupported Linux architecture: $uname_m for dynamic platform detection." >&2
                echo "Please set JAM_PLATFORM manually." >&2
                exit 1
            fi
        elif [ "$uname_s" == "Darwin" ]; then
            if [ "$uname_m" == "x86_64" ]; then
                PLATFORM="x86_64-apple-darwin"
            elif [ "$uname_m" == "arm64" ]; then # Apple Silicon
                PLATFORM="aarch64-apple-darwin"
            else
                echo "Error: Unsupported Darwin architecture: $uname_m for dynamic platform detection." >&2
                echo "Please set JAM_PLATFORM manually." >&2
                exit 1
            fi
        else
            echo "Error: Unsupported operating system: $uname_s for dynamic platform detection." >&2
            echo "Please set JAM_PLATFORM manually." >&2
            exit 1
        fi
        echo "Dynamically determined PLATFORM: $PLATFORM"
    fi
}


cleanup_temp_dirs() {
    if [ -n "$RUN_SPECIFIC_DOWNLOAD_DIR" ] && [ -d "$RUN_SPECIFIC_DOWNLOAD_DIR" ]; then
        echo "Cleaning up run-specific download directory: $RUN_SPECIFIC_DOWNLOAD_DIR"
        rm -rf "$RUN_SPECIFIC_DOWNLOAD_DIR"
    fi
    if [ -n "$RUN_SPECIFIC_EXTRACTION_DIR" ] && [ -d "$RUN_SPECIFIC_EXTRACTION_DIR" ]; then
        echo "Cleaning up run-specific extraction directory: $RUN_SPECIFIC_EXTRACTION_DIR"
        rm -rf "$RUN_SPECIFIC_EXTRACTION_DIR"
    fi
}

trap cleanup_temp_dirs EXIT

initialize_platform_config() {
    if [ -z "$PLATFORM" ]; then
        echo "Error: PLATFORM variable not set. Call determine_platform_or_use_env first." >&2
        exit 1
    fi
    case "$PLATFORM" in
        "x86_64-unknown-linux-gnu")
            SIMPLIFIED_PLATFORM_ASSET_SUFFIX="linux-x86_64"
            ;;
        "aarch64-unknown-linux-gnu")
            SIMPLIFIED_PLATFORM_ASSET_SUFFIX="linux-aarch64"
            ;;
        "x86_64-apple-darwin")
            SIMPLIFIED_PLATFORM_ASSET_SUFFIX="macos-x86_64"
            ;;
        "aarch64-apple-darwin")
            SIMPLIFIED_PLATFORM_ASSET_SUFFIX="macos-aarch64"
            ;;
        *)
            echo "Error: Invalid or unsupported PLATFORM '$PLATFORM' in initialize_platform_config." >&2
            echo "Supported values are typically: x86_64-unknown-linux-gnu, aarch64-unknown-linux-gnu, x86_64-apple-darwin, aarch64-apple-darwin, or custom if JAM_PLATFORM is set." >&2
            exit 1
            ;;
    esac

    SOURCE_BINARY_NAMES_IN_ARCHIVE=(
        "polkajam"
        "jamt"
        "jamtop"
        "polkajam-repl"
        "polkajam-testnet"
    )
    TARGET_BINARY_NAMES_IN_DEPLOY_DIR=(
        "polkajam"
        "jamt"
        "jamtop"
        "polkajam-repl"
        "polkajam-testnet"
    )
}



PROCESSED_RELEASE_TAG=""
fetch_and_store_release() {
    local release_tag_to_fetch="$1"
    local release_json
    local asset_name_filter
    local asset_url
    local asset_filename
    local download_path
    local extracted_binaries_path
    local archive_storage_path_for_release

    PROCESSED_RELEASE_TAG=""
    RUN_SPECIFIC_DOWNLOAD_DIR="${DOWNLOAD_BASE_DIR}/polkajam_dl_$(date +%s)_$$"
    RUN_SPECIFIC_EXTRACTION_DIR="${EXTRACTION_BASE_DIR}/polkajam_extract_$(date +%s)_$$"
    mkdir -p "$RUN_SPECIFIC_DOWNLOAD_DIR"
    mkdir -p "$RUN_SPECIFIC_EXTRACTION_DIR"

    echo "Fetching release information..."
    ALL_RELEASES_JSON=$(curl -s "$GITHUB_API_URL")

    if [ -n "$release_tag_to_fetch" ]; then
        release_json=$(echo "$ALL_RELEASES_JSON" | jq -r --arg TAG "$release_tag_to_fetch" '.[] | select(.tag_name == $TAG)')
        if [ -z "$release_json" ] || [ "$release_json" == "null" ]; then
            echo "Error: Could not find release with tag '$release_tag_to_fetch'." >&2
            return 1
        fi
        PROCESSED_RELEASE_TAG="$release_tag_to_fetch"
        echo "Found specified release: $PROCESSED_RELEASE_TAG"
    else
        release_json=$(echo "$ALL_RELEASES_JSON" | \
            jq -r '[.[] | select((.tag_name | test("^v[0-9]+\\.[0-9]+\\.[0-9]+")) or (.tag_name | ascii_downcase | contains("nightly")))] | sort_by(.published_at) | .[-1]')
        
        if [ -z "$release_json" ] || [ "$release_json" == "null" ]; then
            echo "Error: Could not find any suitable release (stable or nightly)." >&2
            echo "Current releases found (tag_name, prerelease, published_at):" >&2
            echo "$ALL_RELEASES_JSON" | jq -r '.[] | "\(.tag_name) | Prerelease: \(.prerelease) | Published: \(.published_at)"' >&2
            return 1
        fi
        PROCESSED_RELEASE_TAG=$(echo "$release_json" | jq -r '.tag_name')
        echo "Found latest release: $PROCESSED_RELEASE_TAG"
    fi

    archive_storage_path_for_release="${JAM_RELEASES_ARCHIVE_DIR}/${PROCESSED_RELEASE_TAG}"
    if [ -d "$archive_storage_path_for_release" ]; then
        echo "Info: Release '$PROCESSED_RELEASE_TAG' already exists in archive at '$archive_storage_path_for_release'. Fetch and store steps skipped."
        return 0 
    fi

    asset_name_filter="polkajam-${PROCESSED_RELEASE_TAG}-${SIMPLIFIED_PLATFORM_ASSET_SUFFIX}.tgz"

    asset_url=$(echo "$release_json" | jq -r --arg FILTER "$asset_name_filter" '.assets[] | select(.name == $FILTER) | .browser_download_url')
    asset_filename=$(echo "$release_json" | jq -r --arg FILTER "$asset_name_filter" '.assets[] | select(.name == $FILTER) | .name')

    if [ -z "$asset_url" ] || [ "$asset_url" == "null" ]; then
        echo "Error: Could not find asset URL for '$asset_name_filter' in release '$PROCESSED_RELEASE_TAG'." >&2
        echo "Available assets in this release ($PROCESSED_RELEASE_TAG):" >&2
        echo "$release_json" | jq -r '.assets[].name' >&2
        PROCESSED_RELEASE_TAG="" 
        return 1
    fi

    download_path="$RUN_SPECIFIC_DOWNLOAD_DIR/$asset_filename"
    echo "Downloading $asset_filename..."
    curl -L --progress-bar "$asset_url" -o "$download_path" || { echo "Error: Download failed." >&2; PROCESSED_RELEASE_TAG=""; return 1; }
    echo "Download complete."

    echo "Extracting $asset_filename..."
    tar -xzf "$download_path" -C "$RUN_SPECIFIC_EXTRACTION_DIR" || { echo "Error: Extraction failed." >&2; PROCESSED_RELEASE_TAG=""; return 1; }
    echo "Extraction complete."
    
    local extracted_subfolder_name
    extracted_subfolder_name=$(basename "$asset_filename" .tgz)
    extracted_binaries_path="$RUN_SPECIFIC_EXTRACTION_DIR/$extracted_subfolder_name"

    if [ ! -d "$extracted_binaries_path" ]; then
        if find "$RUN_SPECIFIC_EXTRACTION_DIR" -maxdepth 1 -type f -name "*polkajam*" | grep -q .; then
            extracted_binaries_path="$RUN_SPECIFIC_EXTRACTION_DIR"
        else
            local possible_dirs=()
            while IFS= read -r -d '' dir; do
                possible_dirs+=("$dir")
            done < <(find "$RUN_SPECIFIC_EXTRACTION_DIR" -type d -mindepth 1 -maxdepth 2 -print0)
            
            for dir in "${possible_dirs[@]}"; do
                if find "$dir" -maxdepth 1 -type f -name "*polkajam*" | grep -q .; then
                    extracted_binaries_path="$dir"
                    break
                fi
            done
        fi
    fi

    echo "Storing release files..."
    mkdir -p "$archive_storage_path_for_release"

    local files_stored=0
    local polkajam_found=false
    
    while IFS= read -r -d '' file; do
        local filename
        filename=$(basename "$file")
        local storage_file_path="$archive_storage_path_for_release/$filename"
        
        cp -p "$file" "$storage_file_path"
        chmod --reference="$file" "$storage_file_path" 2>/dev/null || chmod $(stat -f %Mp%Lp "$file" 2>/dev/null || echo 755) "$storage_file_path"
        if [[ "$filename" == *"polkajam"* ]] || [[ "$filename" == *"jam"* ]]; then
            polkajam_found=true
        fi
        files_stored=$((files_stored + 1))
    done < <(find "$extracted_binaries_path" -type f -print0)
    if [ "$polkajam_found" = false ] && [ $files_stored -eq 0 ]; then
        echo "  ERROR: No polkajam binary found and no files stored. Cannot store release." >&2
        PROCESSED_RELEASE_TAG=""
        return 1
    fi
    echo "Stored $files_stored files."
    return 0
}

list_stored_releases_and_deploy() {
    echo "Available stored releases in '$JAM_RELEASES_ARCHIVE_DIR':"
    if [ ! -d "$JAM_RELEASES_ARCHIVE_DIR" ] || [ -z "$(ls -A "$JAM_RELEASES_ARCHIVE_DIR")" ]; then
        echo "  No releases found in archive."
        return 1
    fi
    
    local release_options=()
    local OLD_COLUMNS=${COLUMNS:-80}
    COLUMNS=1
    while IFS= read -r line; do
        if [ -n "$line" ]; then 
            release_options+=("$line")
        fi
    done < <(find "$JAM_RELEASES_ARCHIVE_DIR" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort -Vr)
    
    if [ ${#release_options[@]} -eq 0 ]; then
        echo "  No releases found in archive."
        COLUMNS=$OLD_COLUMNS
        return 1
    fi

    PS3="Select a release number to deploy (or q to quit): "
    select selected_tag in "${release_options[@]}" "Quit"; do
        COLUMNS=$OLD_COLUMNS
        if [[ "$REPLY" == "q" || "$REPLY" == "Q" || "$selected_tag" == "Quit" ]]; then
             echo "Deployment cancelled."
             return 1
        elif [ -n "$selected_tag" ]; then
            deploy_release "$selected_tag"
            return $? 
        else
            echo "Invalid selection. Please try again."
            COLUMNS=1 
        fi
    done
    COLUMNS=$OLD_COLUMNS
    return 1 
}

process_and_deploy_specific_tag() {
    local tag_to_process="$1"
    if [ -z "$tag_to_process" ]; then
        echo "Error: No tag provided to process_and_deploy_specific_tag." >&2
        return 1
    fi

    if fetch_and_store_release "$tag_to_process"; then 
        if [ -n "$PROCESSED_RELEASE_TAG" ]; then 
            if deploy_release "$PROCESSED_RELEASE_TAG"; then
                return 0 
            else
                echo "Deployment of $PROCESSED_RELEASE_TAG failed."
                return 1
            fi
        else
            echo "Warning: PROCESSED_RELEASE_TAG was not set by fetch_and_store_release, attempting deploy with input tag '$tag_to_process'." >&2
            if deploy_release "$tag_to_process"; then
                 return 0
            else
                echo "Deployment of $tag_to_process failed."
                return 1
            fi
        fi
    else
        echo "Fetching/storing $tag_to_process failed."
        return 1
    fi
}


list_remote_releases_and_select_for_processing() {
    echo "Fetching available releases from GitHub..."
    ALL_RELEASES_JSON=$(curl -s "$GITHUB_API_URL")
    
    local stable_releases=()
    while IFS='|' read -r tag timestamp; do
        if [ -n "$tag" ]; then
            stable_releases+=("$tag|$timestamp")
        fi
    done < <(echo "$ALL_RELEASES_JSON" | jq -r '[.[] | select((.tag_name | test("^v[0-9]+\\.[0-9]+\\.[0-9]+")) and (.prerelease == false))] | sort_by(.published_at) | reverse | .[] | "\(.tag_name)|\(.published_at)"')
    
    local nightly_releases=()
    while IFS='|' read -r tag timestamp; do
        if [ -n "$tag" ]; then
            nightly_releases+=("$tag|$timestamp")
        fi
    done < <(echo "$ALL_RELEASES_JSON" | jq -r '[.[] | select((.tag_name | ascii_downcase | contains("nightly")))] | sort_by(.published_at) | reverse | .[] | "\(.tag_name)|\(.published_at)"')
    
    local all_release_tags=()
    local release_tag_map=()
    
    for release in "${stable_releases[@]}"; do
        IFS='|' read -r tag timestamp <<< "$release"
        local formatted_date
        formatted_date=$(format_iso_date "$timestamp")
        local display_string="$tag ($formatted_date)"
        all_release_tags+=("$display_string")
        release_tag_map+=("$tag")
    done
    
    for release in "${nightly_releases[@]}"; do
        IFS='|' read -r tag timestamp <<< "$release"
        local formatted_date
        formatted_date=$(format_iso_date "$timestamp")
        local display_string="$tag ($formatted_date)"
        all_release_tags+=("$display_string")
        release_tag_map+=("$tag")
    done

    if [ ${#all_release_tags[@]} -eq 0 ]; then
        echo "  No releases found on GitHub."
        return 1 
    fi

    echo "Available releases on GitHub (newest first):"
    echo "  Stable releases: ${#stable_releases[@]}"
    echo "  Nightly releases: ${#nightly_releases[@]}"
    
    local OLD_COLUMNS=${COLUMNS:-80}
    COLUMNS=1
    PS3="Select a release to fetch, store, and deploy (or 'm' to enter manually, 'q' to quit): "
    select selected_tag_option in "${all_release_tags[@]}" "Enter tag manually" "Quit"; do
        COLUMNS=$OLD_COLUMNS
        if [[ "$REPLY" == "q" || "$REPLY" == "Q" || "$selected_tag_option" == "Quit" ]]; then 
            echo "Quit."
            return 1
        elif [[ "$selected_tag_option" == "Enter tag manually" ]]; then
            read -r -p "Enter the exact release tag: " manual_tag
            if [ -n "$manual_tag" ]; then
                process_and_deploy_specific_tag "$manual_tag"
                return $? 
            else
                echo "No tag entered."
                return 1 
            fi
        elif [ -n "$selected_tag_option" ]; then
            local selected_index=$((REPLY - 1))
            if [ $selected_index -ge 0 ] && [ $selected_index -lt ${#release_tag_map[@]} ]; then
                local actual_tag="${release_tag_map[$selected_index]}"
                process_and_deploy_specific_tag "$actual_tag"
                return $?
            else
                echo "Invalid selection index."
                COLUMNS=1
            fi
        else
            echo "Invalid selection. Please try again."
            COLUMNS=1 
        fi
    done
    COLUMNS=$OLD_COLUMNS
    return 1 
}


deploy_release() {
    local selected_tag_name="$1"
    local stored_release_path="${JAM_RELEASES_ARCHIVE_DIR}/${selected_tag_name}"

    if [ ! -d "$stored_release_path" ]; then
        echo "Error: Stored release '$selected_tag_name' not found at '$stored_release_path'." >&2
        return 1
    fi

    echo "Deploying $selected_tag_name..."
    mkdir -p "$TARGET_DIR"

    echo "Cleaning target directory..."
    local files_to_remove=()
    while IFS= read -r -d '' file; do
        files_to_remove+=("$file")
    done < <(find "$TARGET_DIR" -maxdepth 1 -type f -not -name ".DS_Store" -not -name ".*" -print0)
    
    if [ ${#files_to_remove[@]} -gt 0 ]; then
        echo "Removing ${#files_to_remove[@]} existing files..."
        rm -f "${files_to_remove[@]}"
    else
        echo "No existing files to remove."
    fi


    local files_deployed=0
    while IFS= read -r -d '' file; do
        local filename
        filename=$(basename "$file")
        local target_file_path="$TARGET_DIR/$filename"
        
        cp -p "$file" "$target_file_path"
        
        if [ -x "$file" ]; then
            chmod +x "$target_file_path"
        fi
        
        files_deployed=$((files_deployed + 1))
    done < <(find "$stored_release_path" -maxdepth 1 -type f -print0)
    
    echo "Deployed $files_deployed files."
    touch "${TARGET_DIR}/${selected_tag_name}"

    echo "Deployment of '$selected_tag_name' complete."
    echo "Files in target directory ($TARGET_DIR):"
    while IFS= read -r -d '' file; do
        local filename
        filename=$(basename "$file")
        local file_hash file_size file_date
        
        if command -v shasum >/dev/null 2>&1; then
            file_hash=$(shasum -a 256 "$file" | cut -d' ' -f1 | cut -c1-16)
        elif command -v sha256sum >/dev/null 2>&1; then
            file_hash=$(sha256sum "$file" | cut -d' ' -f1 | cut -c1-16)
        else
            file_hash="(unavailable)"
        fi
        
        if command -v gstat >/dev/null 2>&1; then
            file_size=$(gstat -c%s "$file")
            file_date=$(gstat -c%y "$file" | cut -d' ' -f1,2 | cut -d'.' -f1)
        elif stat --version 2>/dev/null | grep -q GNU; then
            file_size=$(stat -c%s "$file")
            file_date=$(stat -c%y "$file" | cut -d' ' -f1,2 | cut -d'.' -f1)
        else
            file_size=$(stat -f%z "$file")
            file_date=$(stat -f%Sm -t "%Y-%m-%d %H:%M:%S" "$file")
        fi
        
        printf "%-25s %10s  %19s  %s\n" "$filename" "$file_size" "$file_date" "$file_hash"
    done < <(find "$TARGET_DIR" -maxdepth 1 -type f -not -name "${selected_tag_name}" -not -name ".DS_Store" -print0 | sort -z)
    return 0
}

clear_stored_releases() {
    echo "Clearing stored releases..."
    if [ ! -d "$JAM_RELEASES_ARCHIVE_DIR" ]; then
        echo "No release archive directory found at '$JAM_RELEASES_ARCHIVE_DIR'."
        return 0
    fi
    
    local release_count
    release_count=$(find "$JAM_RELEASES_ARCHIVE_DIR" -mindepth 1 -maxdepth 1 -type d | wc -l)
    
    if [ "$release_count" -eq 0 ]; then
        echo "No stored releases found to clear."
        return 0
    fi
    
    echo "Found $release_count stored releases, clearing all..."
    find "$JAM_RELEASES_ARCHIVE_DIR" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort
    rm -rf "${JAM_RELEASES_ARCHIVE_DIR:?}"/*
    echo "All stored releases cleared."
    return 0
}

show_menu() {
    echo ""
    echo "Polkadot JAM Release Manager"
    echo "----------------------------"
    echo "Platform            : $PLATFORM" 
    echo "JAM Base Path       : $JAM_PATH"
    echo "Active Target Dir   : $TARGET_DIR"
    echo "Release Archive Dir : $JAM_RELEASES_ARCHIVE_DIR"
    echo "----------------------------"
    echo "1. Select Latest Release"
    echo "2. Select Specific Release"
    echo "3. Clear Stored Releases"
    echo "Q. Quit"
    echo ""
    read -r -p "Enter your choice: " MENU_CHOICE
}


check_dependencies
determine_platform_or_use_env 
initialize_platform_config 

mkdir -p "$JAM_RELEASES_ARCHIVE_DIR"
mkdir -p "$TARGET_DIR"


while true; do
    show_menu
    case "$MENU_CHOICE" in
        1)
            if fetch_and_store_release ""; then
                if [ -n "$PROCESSED_RELEASE_TAG" ]; then
                    echo "Latest release tag is: $PROCESSED_RELEASE_TAG"
                    if deploy_release "$PROCESSED_RELEASE_TAG"; then
                        echo "Latest release ($PROCESSED_RELEASE_TAG) processed and deployed. Exiting."
                        exit 0
                    else
                        echo "Deployment of latest release ($PROCESSED_RELEASE_TAG) did not complete successfully. Returning to menu."
                        read -r -n 1 -s -p "Press any key to return to menu..." && echo
                    fi
                else
                    echo "Could not determine latest release tag."
                    read -r -n 1 -s -p "Press any key to return to menu..." && echo
                fi
            else
                echo "Could not fetch latest release."
                read -r -n 1 -s -p "Press any key to return to menu..." && echo
            fi
            ;;
        2)
            if list_remote_releases_and_select_for_processing; then 
                echo "Selected release processed and deployed. Exiting."
                exit 0
            else
                exit 1
            fi
            ;;
        3)
            if clear_stored_releases; then
                echo "Exiting."
                exit 0
            else
                read -r -n 1 -s -p "Press any key to return to menu..." && echo
            fi
            ;;
        [qQ])
            echo "Quit."
            break
            ;;
        *)
            echo "Invalid choice. Please try again."
            read -r -n 1 -s -p "Press any key to return to menu..." && echo
            ;;
    esac
done

exit 0
