#!/bin/bash

# Generate test runner script for all fuzz trace test cases

TRACES_DIR="/Users/sourabhniyogi/Desktop/jam-conformance/fuzz-reports/0.7.1/traces"
OUTPUT_DIR="$HOME/Desktop/jamtestnet/jam-conformance/0.7.1"
SCRIPT_FILE="run_all_fuzz_tests.sh"

# Create output script
echo "#!/bin/bash" > $SCRIPT_FILE
echo "" >> $SCRIPT_FILE
echo "# Auto-generated script to run all fuzz trace tests" >> $SCRIPT_FILE
echo "# Generated on $(date)" >> $SCRIPT_FILE
echo "" >> $SCRIPT_FILE

# Generate test commands for each subdirectory and bin file
for subdir in "$TRACES_DIR"/*; do
    if [ -d "$subdir" ]; then
        dirname=$(basename "$subdir")
        echo "mkdir -p $OUTPUT_DIR/$dirname" >> $SCRIPT_FILE
        echo "" >> $SCRIPT_FILE
        echo "# Processing $dirname directory" >> $SCRIPT_FILE
        for binfile in "$subdir"/*.bin; do
            if [ -f "$binfile" ]; then
                filename=$(basename "$binfile")
                echo "echo \"Running test for $dirname/$filename...\"" >> $SCRIPT_FILE
                echo "go test -run=\"TestFuzzTrace/fuzz-reports/0.7.1/traces/$dirname/$filename\$\" 2>&1 | gzip > $OUTPUT_DIR/$dirname/${filename%.bin}.log.gz" >> $SCRIPT_FILE
            fi
        done
        echo "" >> $SCRIPT_FILE
    fi
done

echo "echo \"All fuzz tests completed!\"" >> $SCRIPT_FILE

chmod +x $SCRIPT_FILE
echo "Generated $SCRIPT_FILE"
