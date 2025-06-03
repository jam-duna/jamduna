#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e
# Treat unset variables as an error when substituting.
set -u
# Pipestatus: the return value of a pipeline is the status of the last command to exit with a non-zero status,
# or zero if no command exited with a non-zero status.
set -o pipefail

# --- Configuration ---
GITHUB_API_URL="https://api.github.com/repos/paritytech/polkajam-releases/releases"

# User-configurable: Base path for Jam installation (MUST be set as environment variable)
if [ -z "${JAM_PATH:-}" ]; then
    echo "Error: Environment variable JAM_PATH is not set." >&2
    echo "Please set it to your Jam installation directory, e.g.:" >&2
    echo "  export JAM_PATH=\"\${HOME}/.jam\"" >&2
    exit 1
fi
echo "Using JAM_PATH: $JAM_PATH"

# User-configurable: Target directory for the final binaries
# Defaults to $JAM_PATH/bin, can be overridden by JAM_TARGET_BIN_DIR
TARGET_DIR="${JAM_TARGET_BIN_DIR:-${JAM_PATH}/bin}"

# User-configurable: Persistent directory to store all downloaded releases (MUST be set as environment variable)
if [ -z "${POLKAJAM_RELEASES_PATH:-}" ]; then
    echo "Error: Environment variable POLKAJAM_RELEASES_PATH is not set." >&2
    echo "Please set it to your desired archive directory for Polkadot JAM releases, e.g.:" >&2
    echo "  export POLKAJAM_RELEASES_PATH=\"\${HOME}/.polkajam_releases_archive\"" >&2
    exit 1
fi
JAM_RELEASES_ARCHIVE_DIR="${POLKAJAM_RELEASES_PATH}"
echo "Using POLKAJAM_RELEASES_PATH for archive: $JAM_RELEASES_ARCHIVE_DIR"


# User-configurable: Base directory for temporary downloads (during a fetch operation)
DOWNLOAD_BASE_DIR="${JAM_DOWNLOAD_DIR:-/tmp}"
# User-configurable: Base directory for temporary extractions (during a fetch operation)
EXTRACTION_BASE_DIR="${JAM_EXTRACTION_DIR:-/tmp}"

# Platform: Dynamically determined or overridden by JAM_PLATFORM
PLATFORM="" # Will be set by determine_platform_or_use_env

# Generated: Unique directory for this script run's temporary downloads
RUN_SPECIFIC_DOWNLOAD_DIR="" # Will be set in fetch_and_store_release
# Generated: Unique directory for this script run's temporary extractions
RUN_SPECIFIC_EXTRACTION_DIR="" # Will be set in fetch_and_store_release


# Indexed arrays for mapping source binary names to target binary names
# These will be populated by initialize_platform_config
SOURCE_BINARY_NAMES_IN_ARCHIVE=()
TARGET_BINARY_NAMES_IN_DEPLOY_DIR=()

# Will be populated after platform is determined
SIMPLIFIED_PLATFORM_ASSET_SUFFIX="" # For asset name construction

# --- Helper Functions ---

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


# Function to clean up run-specific temporary directories
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

# Register the cleanup function to be called on script exit
trap cleanup_temp_dirs EXIT

# Sets SIMPLIFIED_PLATFORM_ASSET_SUFFIX
# and populates SOURCE_BINARY_NAMES_IN_ARCHIVE & TARGET_BINARY_NAMES_IN_DEPLOY_DIR arrays
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

    # Define the mapping for deployment phase
    # Source names as they are expected to be in the archive
    SOURCE_BINARY_NAMES_IN_ARCHIVE=(
        "polkajam"
        "jamt"
        "jamtop"
        "polkajam-repl"
        "polkajam-testnet"
    )
    # Corresponding target names in the final $TARGET_DIR
    # polkajam from archive is deployed as polkajam
    TARGET_BINARY_NAMES_IN_DEPLOY_DIR=(
        "polkajam"
        "jamt"
        "jamtop"
        "polkajam-repl"
        "polkajam-testnet"
    )
}


# --- Core Functions ---

# Fetches and stores a release.
# Returns 0 if release is successfully fetched and stored OR if it already exists in archive.
# Returns 1 on failure.
# Sets global variable PROCESSED_RELEASE_TAG with the tag name of the release.
PROCESSED_RELEASE_TAG=""
fetch_and_store_release() {
    local release_tag_to_fetch="$1" # If empty, fetches latest nightly
    local release_json
    local asset_name_filter
    local asset_url
    local asset_filename
    local download_path
    local extracted_binaries_path
    local archive_storage_path_for_release

    PROCESSED_RELEASE_TAG="" # Reset global var

    # Setup temporary directories for this fetch operation
    RUN_SPECIFIC_DOWNLOAD_DIR="${DOWNLOAD_BASE_DIR}/polkajam_dl_$(date +%s)_$$"
    RUN_SPECIFIC_EXTRACTION_DIR="${EXTRACTION_BASE_DIR}/polkajam_extract_$(date +%s)_$$"
    mkdir -p "$RUN_SPECIFIC_DOWNLOAD_DIR"
    mkdir -p "$RUN_SPECIFIC_EXTRACTION_DIR"

    echo "Fetching release information from GitHub ($GITHUB_API_URL)..."
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
            jq -r '[.[] | select((.tag_name | ascii_downcase | contains("nightly")))] | sort_by(.published_at) | .[-1]')
        if [ -z "$release_json" ] || [ "$release_json" == "null" ]; then
            echo "Error: Could not find the latest nightly release (tag_name contains 'nightly')." >&2
            echo "Current releases found (tag_name, prerelease, published_at):" >&2
            echo "$ALL_RELEASES_JSON" | jq -r '.[] | "\(.tag_name) | Prerelease: \(.prerelease) | Published: \(.published_at)"' >&2
            return 1
        fi
        PROCESSED_RELEASE_TAG=$(echo "$release_json" | jq -r '.tag_name')
        echo "Found latest nightly release: $PROCESSED_RELEASE_TAG"
    fi

    archive_storage_path_for_release="${JAM_RELEASES_ARCHIVE_DIR}/${PROCESSED_RELEASE_TAG}"
    if [ -d "$archive_storage_path_for_release" ]; then
        echo "Info: Release '$PROCESSED_RELEASE_TAG' already exists in archive at '$archive_storage_path_for_release'. Fetch and store steps skipped."
        return 0 
    fi

    asset_name_filter="polkajam-${PROCESSED_RELEASE_TAG}-${SIMPLIFIED_PLATFORM_ASSET_SUFFIX}.tgz"
    echo "Constructed Asset Name Filter: $asset_name_filter"

    asset_url=$(echo "$release_json" | jq -r --arg FILTER "$asset_name_filter" '.assets[] | select(.name == $FILTER) | .browser_download_url')
    asset_filename=$(echo "$release_json" | jq -r --arg FILTER "$asset_name_filter" '.assets[] | select(.name == $FILTER) | .name')

    if [ -z "$asset_url" ] || [ "$asset_url" == "null" ]; then
        echo "Error: Could not find asset URL for '$asset_name_filter' in release '$PROCESSED_RELEASE_TAG'." >&2
        echo "Available assets in this release ($PROCESSED_RELEASE_TAG):" >&2
        echo "$release_json" | jq -r '.assets[].name' >&2
        PROCESSED_RELEASE_TAG="" 
        return 1
    fi
    echo "Asset to download: $asset_filename (URL: $asset_url)"

    download_path="$RUN_SPECIFIC_DOWNLOAD_DIR/$asset_filename"
    echo "Downloading $asset_filename to $download_path..."
    curl -L --progress-bar "$asset_url" -o "$download_path" || { echo "Error: Download failed." >&2; PROCESSED_RELEASE_TAG=""; return 1; }
    echo "Download complete."

    echo "Extracting $asset_filename to $RUN_SPECIFIC_EXTRACTION_DIR..."
    tar -xzf "$download_path" -C "$RUN_SPECIFIC_EXTRACTION_DIR" || { echo "Error: Extraction failed." >&2; PROCESSED_RELEASE_TAG=""; return 1; }
    echo "Extraction complete."

    local extracted_subfolder_name
    extracted_subfolder_name=$(basename "$asset_filename" .tgz) 
    extracted_binaries_path="$RUN_SPECIFIC_EXTRACTION_DIR/$extracted_subfolder_name"

    if [ ! -d "$extracted_binaries_path" ]; then
        echo "Info: Did not find expected subfolder '$extracted_binaries_path'." >&2
        echo "Assuming binaries were extracted directly into '$RUN_SPECIFIC_EXTRACTION_DIR'." >&2
        extracted_binaries_path="$RUN_SPECIFIC_EXTRACTION_DIR"
        local first_binary_source_name="${SOURCE_BINARY_NAMES_IN_ARCHIVE[0]}"
        if [ -n "$first_binary_source_name" ] && [ ! -f "$extracted_binaries_path/$first_binary_source_name" ]; then
             echo "Warning: Could not find binaries in assumed paths." >&2
             ls -lA "$RUN_SPECIFIC_EXTRACTION_DIR" >&2
             local possible_subfolders=("$RUN_SPECIFIC_EXTRACTION_DIR"/*/)
             if [ ${#possible_subfolders[@]} -eq 1 ] && [ -d "${possible_subfolders[0]}" ]; then
                 extracted_binaries_path=$(realpath "${possible_subfolders[0]}")
                 echo "Update: Found a single subdirectory, using: $extracted_binaries_path"
             else
                 echo "Could not reliably determine the extracted binaries path. Storing may fail." >&2
             fi
        fi
    fi
    echo "Binaries sourced from: $extracted_binaries_path"

    echo "Storing binaries for release '$PROCESSED_RELEASE_TAG' in '$archive_storage_path_for_release'..."
    mkdir -p "$archive_storage_path_for_release"

    # Store binaries with their original names from the archive
    for source_name_in_archive in "${SOURCE_BINARY_NAMES_IN_ARCHIVE[@]}"; do
        local source_file_path="$extracted_binaries_path/$source_name_in_archive"
        local storage_file_path="$archive_storage_path_for_release/$source_name_in_archive" 

        if [ -f "$source_file_path" ]; then
            echo "  Storing '$source_name_in_archive' to '$storage_file_path'..."
            cp "$source_file_path" "$storage_file_path"
            chmod +x "$storage_file_path" 
        else
            if [ "$source_name_in_archive" == "polkajam" ]; then
                 echo "  ERROR: Main source binary '$source_name_in_archive' not found in '$extracted_binaries_path'. Cannot store release." >&2
                 PROCESSED_RELEASE_TAG=""
                 return 1 
            else
                echo "  Warning: Auxiliary binary '$source_name_in_archive' not found in '$extracted_binaries_path'. Skipping storage for this file." >&2
            fi
        fi
    done
    echo "Release '$PROCESSED_RELEASE_TAG' successfully fetched and stored in archive."
    return 0 # Indicate success
}

list_stored_releases_and_deploy() {
    echo "Available stored releases in '$JAM_RELEASES_ARCHIVE_DIR':"
    if [ ! -d "$JAM_RELEASES_ARCHIVE_DIR" ] || [ -z "$(ls -A "$JAM_RELEASES_ARCHIVE_DIR")" ]; then
        echo "  No releases found in archive."
        return 1
    fi
    
    local release_options=()
    local OLD_COLUMNS=${COLUMNS:-80} # Default to 80 if COLUMNS is unset
    COLUMNS=1 # Force single column for select
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
        COLUMNS=$OLD_COLUMNS # Restore COLUMNS as soon as select is done or broken from
        if [[ "$REPLY" == "q" || "$REPLY" == "Q" || "$selected_tag" == "Quit" ]]; then
             echo "Deployment cancelled."
             return 1
        elif [ -n "$selected_tag" ]; then
            deploy_release "$selected_tag"
            return $? 
        else
            echo "Invalid selection. Please try again."
            # To re-display the menu correctly if select continues
            COLUMNS=1 
        fi
    done
    COLUMNS=$OLD_COLUMNS # Fallback restoration
    return 1 
}

# Fetches/stores a specific release and then deploys it.
# Returns 0 if both fetch/store AND deploy are successful.
# Returns 1 otherwise.
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


list_remote_nightly_releases_and_select_for_processing() {
    echo "Fetching available nightly releases from GitHub..."
    ALL_RELEASES_JSON=$(curl -s "$GITHUB_API_URL")
    
    local nightly_tags=()
    local OLD_COLUMNS=${COLUMNS:-80} # Default to 80 if COLUMNS is unset
    COLUMNS=1 # Force single column for select
    while IFS= read -r line; do
        if [ -n "$line" ]; then
            nightly_tags+=("$line")
        fi
    done < <(echo "$ALL_RELEASES_JSON" | jq -r '[.[] | select((.tag_name | ascii_downcase | contains("nightly")))] | sort_by(.published_at) | reverse | .[].tag_name')
    


    if [ ${#nightly_tags[@]} -eq 0 ]; then
        echo "  No nightly releases found on GitHub."
        COLUMNS=$OLD_COLUMNS
        return 1 
    fi

    echo "Available nightly releases on GitHub (newest first):"
    PS3="Select a release to fetch, store, and deploy (or 'm' to enter manually, 'q' to quit): "
    select selected_tag_option in "${nightly_tags[@]}" "Enter tag manually" "Quit"; do
        COLUMNS=$OLD_COLUMNS # Restore COLUMNS as soon as select is done or broken from
        if [[ "$REPLY" == "q" || "$REPLY" == "Q" || "$selected_tag_option" == "Quit" ]]; then 
            echo "Operation cancelled."
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
            process_and_deploy_specific_tag "$selected_tag_option"
            return $? 
        else
            echo "Invalid selection. Please try again."
            # To re-display the menu correctly if select continues
            COLUMNS=1 
        fi
    done
    COLUMNS=$OLD_COLUMNS # Fallback restoration
    return 1 
}


deploy_release() {
    local selected_tag_name="$1"
    local stored_release_path="${JAM_RELEASES_ARCHIVE_DIR}/${selected_tag_name}"

    if [ ! -d "$stored_release_path" ]; then
        echo "Error: Stored release '$selected_tag_name' not found at '$stored_release_path'." >&2
        return 1
    fi

    echo "Deploying release '$selected_tag_name' to '$TARGET_DIR'..."
    mkdir -p "$TARGET_DIR"

    echo "  Removing previous release indicator files from $TARGET_DIR..."
    # Remove old style indicators first
    find "$TARGET_DIR" -maxdepth 1 -type f -name 'ACTIVE_RELEASE_IS_*.txt' -delete 2>/dev/null || true
    # Remove new style indicators (files named exactly like other archived tags)
    if [ -d "$JAM_RELEASES_ARCHIVE_DIR" ]; then
        for item_in_archive in "$JAM_RELEASES_ARCHIVE_DIR"/*; do
            if [ -d "$item_in_archive" ]; then
                local archived_tag_name
                archived_tag_name=$(basename "$item_in_archive")
                if [ "$archived_tag_name" != "$selected_tag_name" ] && [ -f "${TARGET_DIR}/${archived_tag_name}" ]; then
                    echo "    Removing previous indicator file: ${TARGET_DIR}/${archived_tag_name}"
                    rm -f "${TARGET_DIR}/${archived_tag_name}"
                fi
            fi
        done
    fi


    for i in "${!SOURCE_BINARY_NAMES_IN_ARCHIVE[@]}"; do
        local source_name_in_archive="${SOURCE_BINARY_NAMES_IN_ARCHIVE[i]}"
        local target_name="${TARGET_BINARY_NAMES_IN_DEPLOY_DIR[i]}"
        local source_file_path="$stored_release_path/$source_name_in_archive"
        local target_file_path="$TARGET_DIR/$target_name"

        if [ -f "$source_file_path" ]; then
            echo "  Copying '$source_name_in_archive' (from archive) to '$target_file_path'..."
            cp "$source_file_path" "$target_file_path"
            chmod +x "$target_file_path"
        else
            echo "  Warning: Source binary '$source_name_in_archive' not found in stored release '$selected_tag_name'. Skipping deployment for this file." >&2
        fi
    done

    echo "  Creating new release indicator file: ${TARGET_DIR}/${selected_tag_name}"
    touch "${TARGET_DIR}/${selected_tag_name}" # New indicator filename

    echo "Deployment of '$selected_tag_name' complete."
    echo "Binaries in target directory ($TARGET_DIR):"
    ls -lt "$TARGET_DIR"
    return 0 # Indicate success
}

show_menu() {
    echo ""
    echo "Polkadot JAM Nightly Release Manager"
    echo "------------------------------------"
    echo "Platform            : $PLATFORM" 
    echo "JAM Base Path       : $JAM_PATH"
    echo "Active Target Dir   : $TARGET_DIR"
    echo "Release Archive Dir : $JAM_RELEASES_ARCHIVE_DIR"
    echo "------------------------------------"
    echo "1. Fetch, store, and deploy latest nightly release"
    echo "2. Fetch, store, and deploy specific nightly release (select from list or enter tag)"
    echo "3. List stored releases and deploy one"
    # echo "4. Deploy specific stored release (by tag)" # Option 4 disabled as per user request
    echo "Q. Quit"
    echo ""
    read -r -p "Enter your choice: " MENU_CHOICE
}

# --- Main Execution ---

check_dependencies
determine_platform_or_use_env 
initialize_platform_config 

mkdir -p "$JAM_RELEASES_ARCHIVE_DIR"
mkdir -p "$TARGET_DIR"


while true; do
    show_menu
    case "$MENU_CHOICE" in
        1)
            echo "Determining latest nightly release tag..."
            ALL_RELEASES_JSON_FOR_LATEST=$(curl -s "$GITHUB_API_URL")
            LATEST_NIGHTLY_TAG=$(echo "$ALL_RELEASES_JSON_FOR_LATEST" | \
                jq -r '[.[] | select((.tag_name | ascii_downcase | contains("nightly")))] | sort_by(.published_at) | .[-1].tag_name')

            if [ -z "$LATEST_NIGHTLY_TAG" ] || [ "$LATEST_NIGHTLY_TAG" == "null" ]; then
                echo "Error: Could not determine the latest nightly release tag."
                read -r -n 1 -s -p "Press any key to return to menu..." && echo
            else
                echo "Latest nightly tag is: $LATEST_NIGHTLY_TAG"
                if process_and_deploy_specific_tag "$LATEST_NIGHTLY_TAG"; then
                    echo "Latest nightly release ($LATEST_NIGHTLY_TAG) processed and deployed. Exiting."
                    exit 0
                else
                    echo "Operation for latest nightly ($LATEST_NIGHTLY_TAG) did not complete successfully. Returning to menu."
                    read -r -n 1 -s -p "Press any key to return to menu..." && echo
                fi
            fi
            ;;
        2)
            if list_remote_nightly_releases_and_select_for_processing; then 
                echo "Selected nightly release processed and deployed. Exiting."
                exit 0
            else
                echo "Operation for specific nightly did not complete successfully or was cancelled. Returning to menu."
                read -r -n 1 -s -p "Press any key to return to menu..." && echo
            fi
            ;;
        3)
            list_stored_releases_and_deploy 
            read -r -n 1 -s -p "Press any key to return to menu..." && echo
            ;;
        # Option 4 is disabled
        # 4)
        #     read -r -p "Enter the exact tag of the stored release to deploy: " specific_tag_deploy
        #      if [ -n "$specific_tag_deploy" ]; then
        #         deploy_release "$specific_tag_deploy"
        #     else
        #         echo "No tag entered."
        #     fi
        #     read -r -n 1 -s -p "Press any key to return to menu..." && echo
        #     ;;
        [qQ])
            echo "Exiting."
            break
            ;;
        *)
            echo "Invalid choice. Please try again."
            read -r -n 1 -s -p "Press any key to return to menu..." && echo
            ;;
    esac
done

exit 0
