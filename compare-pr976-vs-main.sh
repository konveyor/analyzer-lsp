#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}PR #976 vs Main Branch - Performance Comparison${NC}"
echo -e "${BLUE}Use Case: PatternFly Migration (nodejs.referenced)${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Configuration
KANTRA_DIR="/Users/tsanders/Workspace/kantra"
ANALYZER_LSP_DIR="/Users/tsanders/Workspace/analyzer-lsp"
INPUT_DIR="/Users/tsanders/Workspace/tackle2-ui"
RULESET="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive"
OUTPUT_DIR_MAIN="/tmp/test-main-prepare-$(date +%s)"
OUTPUT_DIR_PR976="/tmp/test-pr976-import-search-$(date +%s)"

# Track timing
MAIN_TIME=""
PR976_TIME=""

cleanup() {
    echo -e "\n${YELLOW}Cleaning up containers...${NC}"
    podman ps -a --format "{{.Names}}" | grep -E "provider-|nodejs" | xargs -r podman rm -f 2>/dev/null || true
}

trap cleanup EXIT

# Function to build generic-provider image
build_image() {
    local branch=$1
    local tag=$2

    echo -e "${YELLOW}Building generic-provider image from branch: ${branch}${NC}"
    cd "$ANALYZER_LSP_DIR"

    # Stash any changes
    git stash push -m "Stashing before branch switch for benchmark" 2>/dev/null || true

    # Switch to branch
    git checkout "$branch"

    # Build image from root directory (build context needs to include all of analyzer-lsp)
    echo "Building image with tag: ${tag}"
    podman build -t "localhost/generic-provider:${tag}" -f external-providers/generic-external-provider/Dockerfile .
}

# Function to run kantra and measure time
run_kantra_test() {
    local image_tag=$1
    local output_dir=$2
    local test_name=$3

    echo -e "\n${YELLOW}Test: ${test_name}${NC}"

    # Clean up any existing containers
    cleanup

    # Run kantra
    cd "$KANTRA_DIR"

    local start_time=$(date +%s)

    if GENERIC_PROVIDER_IMG="localhost/generic-provider:${image_tag}" ./kantra analyze \
        --input "$INPUT_DIR" \
        --output "$output_dir" \
        --rules "$RULESET" \
        --overwrite 2>&1 | tee "${output_dir}.log"; then

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        echo -e "${GREEN}${test_name} completed successfully${NC}"
        echo -e "${YELLOW}Duration: ${duration} seconds${NC}"
        echo "$duration"
        return 0
    else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        echo -e "${RED}${test_name} encountered errors (exit code: $?)${NC}"
        echo -e "${YELLOW}But timing was captured: ${duration} seconds${NC}"
        echo "$duration"
        return 1
    fi
}

# Build images from both branches
echo -e "${YELLOW}Step 1: Building images${NC}"
build_image "main" "main-prepare"
build_image "fix/nodejs-referenced-import-search" "pr976-import"

# Return to PR branch
git checkout fix/nodejs-referenced-import-search

echo ""
echo -e "${YELLOW}Step 2: Running benchmarks${NC}"

# Test 1: Main branch with prepare method
MAIN_TIME=$(run_kantra_test "main-prepare" "$OUTPUT_DIR_MAIN" "Main Branch (with Prepare method)")
MAIN_EXIT=$?

echo ""
echo "Waiting 10 seconds before next test..."
sleep 10

# Test 2: PR #976 with import-based search
PR976_TIME=$(run_kantra_test "pr976-import" "$OUTPUT_DIR_PR976" "PR #976 (import-based search)")
PR976_EXIT=$?

# Display results
echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Results${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo "Main Branch (Prepare method):     ${MAIN_TIME} seconds"
echo "PR #976 (import-based search):    ${PR976_TIME} seconds"
echo ""

# Calculate speedup/slowdown
if [ -n "$MAIN_TIME" ] && [ -n "$PR976_TIME" ]; then
    if [ "$PR976_TIME" -lt "$MAIN_TIME" ]; then
        DIFF=$((MAIN_TIME - PR976_TIME))
        SPEEDUP=$(echo "scale=2; $MAIN_TIME / $PR976_TIME" | bc)
        PCT_CHANGE=$(echo "scale=2; (($MAIN_TIME - $PR976_TIME) / $MAIN_TIME) * 100" | bc)
        echo -e "${GREEN}PR #976 was faster by ${DIFF} seconds${NC}"
        echo -e "${GREEN}Speedup: ${SPEEDUP}x (+${PCT_CHANGE}% improvement)${NC}"
    else
        DIFF=$((PR976_TIME - MAIN_TIME))
        SLOWDOWN=$(echo "scale=2; $PR976_TIME / $MAIN_TIME" | bc)
        PCT_CHANGE=$(echo "scale=2; (($PR976_TIME - $MAIN_TIME) / $MAIN_TIME) * 100" | bc)
        echo -e "${RED}PR #976 was slower by ${DIFF} seconds${NC}"
        echo -e "${RED}Slowdown: ${SLOWDOWN}x (+${PCT_CHANGE}% slower)${NC}"
    fi
fi

# Save detailed results
RESULTS_FILE="/tmp/pr976-vs-main-comparison.txt"
cat > "$RESULTS_FILE" << EOF
Performance Comparison: PR #976 vs Main Branch
================================================

Test Configuration:
- Input: tackle2-ui codebase
- Ruleset: PatternFly v6 comprehensive
- Main Branch: Prepare method for nodejs provider
- PR #976: Import-based search for nodejs.referenced

Results:
- Main Branch (Prepare method):   ${MAIN_TIME} seconds (exit: ${MAIN_EXIT})
- PR #976 (import-based search):  ${PR976_TIME} seconds (exit: ${PR976_EXIT})

Output Directories:
- Main: ${OUTPUT_DIR_MAIN}
- PR #976: ${OUTPUT_DIR_PR976}

Logs:
- Main: ${OUTPUT_DIR_MAIN}.log
- PR #976: ${OUTPUT_DIR_PR976}.log
EOF

echo ""
echo "Detailed results saved to: $RESULTS_FILE"
echo ""
echo -e "${YELLOW}Note: Output directories preserved at:${NC}"
echo "  - $OUTPUT_DIR_MAIN"
echo "  - $OUTPUT_DIR_PR976"
echo ""
echo -e "${YELLOW}Clean up manually when done: rm -rf $OUTPUT_DIR_MAIN $OUTPUT_DIR_PR976${NC}"
echo ""
