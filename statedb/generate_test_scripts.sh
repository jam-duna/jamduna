#!/bin/bash

# Generate test runner script for all trace test cases

TRACES_DIR="/Users/sourabhniyogi/Desktop/jam-test-vectors/traces"
OUTPUT_DIR="$HOME/Desktop/jamtestnet/traces/0.7.1"
SCRIPT_FILE="run_all_trace_tests.sh"

# Directories to process
DIRS=("fuzzy" "fuzzy_light" "preimages" "preimages_light" "storage" "storage_light")

# Create output script
echo "#!/bin/bash" > $SCRIPT_FILE
echo "" >> $SCRIPT_FILE
echo "# Auto-generated script to run all trace tests" >> $SCRIPT_FILE
echo "# Generated on $(date)" >> $SCRIPT_FILE
echo "" >> $SCRIPT_FILE

# Create output directories
for dir in "${DIRS[@]}"; do
    echo "mkdir -p $OUTPUT_DIR/$dir" >> $SCRIPT_FILE
done

echo "" >> $SCRIPT_FILE

# Generate test commands for each directory and bin file
for dir in "${DIRS[@]}"; do
    if [ -d "$TRACES_DIR/$dir" ]; then
        echo "# Processing $dir directory" >> $SCRIPT_FILE
        for binfile in "$TRACES_DIR/$dir"/*.bin; do
            if [ -f "$binfile" ]; then
                filename=$(basename "$binfile")
                # Skip genesis files (00000000.bin and genesis.bin)
                if [ "$filename" != "00000000.bin" ] && [ "$filename" != "genesis.bin" ]; then
                    echo "echo \"Running test for $dir/$filename...\"" >> $SCRIPT_FILE
                    echo "go test -run=\"TestTracesGoInterpreter/$dir/$filename\$\" 2>&1 | gzip > $OUTPUT_DIR/$dir/${filename%.bin}.log.gz" >> $SCRIPT_FILE
                fi
            fi
        done
        echo "" >> $SCRIPT_FILE
    fi
done

echo "echo \"All tests completed!\"" >> $SCRIPT_FILE

chmod +x $SCRIPT_FILE
echo "Generated $SCRIPT_FILE"
