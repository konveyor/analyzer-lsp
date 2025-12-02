#!/bin/bash

# Compare baseline vs corrected worker pool optimization (v2)
# This tests the fixed version with global LSP rate limiting

APP_PATH="/Users/tsanders/Workspace/tackle2-ui"
RULESET="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive"
KANTRA="/Users/tsanders/Workspace/kantra/kantra"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Performance Comparison Test - Corrected Optimization${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Test 1: Baseline (official quay.io image - 1 worker)
echo -e "${YELLOW}Test 1: Baseline (official image - 1 worker)${NC}"
OUTPUT_BASELINE="/tmp/test-baseline-$(date +%s)"
START_BASELINE=$(date +%s)

cd /Users/tsanders/Workspace/kantra
./kantra analyze \
    --input "$APP_PATH" \
    --output "$OUTPUT_BASELINE" \
    --rules "$RULESET" \
    --overwrite \
    > /tmp/baseline-test-v2.log 2>&1

BASELINE_EXIT=$?
END_BASELINE=$(date +%s)
BASELINE_TIME=$((END_BASELINE - START_BASELINE))

if [ $BASELINE_EXIT -ne 0 ]; then
    echo -e "${RED}Baseline test encountered errors (exit code: $BASELINE_EXIT)${NC}"
    echo -e "${YELLOW}But timing was captured: ${BASELINE_TIME} seconds${NC}"
else
    echo -e "${GREEN}Baseline completed successfully in ${BASELINE_TIME} seconds${NC}"
fi
echo ""

# Wait between tests
echo "Waiting 10 seconds before next test..."
sleep 10

# Test 2: Optimized-v2 (localhost image with corrected worker pool + global LSP rate limiting)
echo -e "${YELLOW}Test 2: Optimized-v2 (10 workers + global LSP rate limiting)${NC}"
OUTPUT_OPTIMIZED="/tmp/test-optimized-v2-$(date +%s)"
START_OPTIMIZED=$(date +%s)

cd /Users/tsanders/Workspace/kantra
GENERIC_PROVIDER_IMG=localhost/generic-provider:optimized-v2 ./kantra analyze \
    --input "$APP_PATH" \
    --output "$OUTPUT_OPTIMIZED" \
    --rules "$RULESET" \
    --overwrite \
    > /tmp/optimized-test-v2.log 2>&1

OPTIMIZED_EXIT=$?
END_OPTIMIZED=$(date +%s)
OPTIMIZED_TIME=$((END_OPTIMIZED - START_OPTIMIZED))

if [ $OPTIMIZED_EXIT -ne 0 ]; then
    echo -e "${RED}Optimized-v2 test encountered errors (exit code: $OPTIMIZED_EXIT)${NC}"
    echo -e "${YELLOW}But timing was captured: ${OPTIMIZED_TIME} seconds${NC}"
else
    echo -e "${GREEN}Optimized-v2 completed successfully in ${OPTIMIZED_TIME} seconds${NC}"
fi
echo ""

# Calculate difference
SAVED_TIME=$((BASELINE_TIME - OPTIMIZED_TIME))
if [ $BASELINE_TIME -gt 0 ]; then
    IMPROVEMENT=$(echo "scale=2; $BASELINE_TIME / $OPTIMIZED_TIME" | bc)
    PERCENT=$(echo "scale=1; ($IMPROVEMENT - 1) * 100" | bc)
else
    IMPROVEMENT="N/A"
    PERCENT="N/A"
fi

# Results
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Results${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "Baseline (1 worker):     ${BASELINE_TIME} seconds"
echo -e "Optimized-v2 (10 workers + global LSP limit): ${OPTIMIZED_TIME} seconds"
echo ""
if [ "$SAVED_TIME" -gt 0 ]; then
    echo -e "${GREEN}Time saved: ${SAVED_TIME} seconds${NC}"
    echo -e "${GREEN}Speedup: ${IMPROVEMENT}x (${PERCENT}% faster)${NC}"
else
    echo -e "${RED}Optimized-v2 was slower by $((SAVED_TIME * -1)) seconds${NC}"
    echo -e "${RED}Slowdown: ${IMPROVEMENT}x (${PERCENT}% change)${NC}"
fi
echo ""

# Save results
cat > /tmp/performance-comparison-v2.txt <<EOF
Performance Comparison: Corrected Worker Pool Optimization (v2)
Date: $(date)

Configuration:
- Input: $APP_PATH
- Rules: $RULESET
- Baseline: quay.io image (1 worker)
- Optimized-v2: localhost/generic-provider:optimized-v2 (10 workers + global LSP rate limiting)

Implementation:
- 10 concurrent workers for file processing (I/O, text search, cache)
- Global LSP semaphore limiting total concurrent LSP calls to 15
- No per-file semaphore to avoid concurrency multiplication

Results:
- Baseline time:  ${BASELINE_TIME} seconds
- Optimized time: ${OPTIMIZED_TIME} seconds
- Time saved:     ${SAVED_TIME} seconds
- Speedup:        ${IMPROVEMENT}x (${PERCENT}% improvement)

Exit codes:
- Baseline:  ${BASELINE_EXIT}
- Optimized: ${OPTIMIZED_EXIT}

Detailed logs:
- Baseline:  /tmp/baseline-test-v2.log
- Optimized: /tmp/optimized-test-v2.log

Output directories (preserved for inspection):
- Baseline:  ${OUTPUT_BASELINE}
- Optimized: ${OUTPUT_OPTIMIZED}
EOF

echo "Detailed results saved to: /tmp/performance-comparison-v2.txt"
echo ""
echo -e "${YELLOW}Note: Output directories preserved at:${NC}"
echo "  - $OUTPUT_BASELINE"
echo "  - $OUTPUT_OPTIMIZED"
echo ""
echo -e "${YELLOW}Clean up manually when done: rm -rf $OUTPUT_BASELINE $OUTPUT_OPTIMIZED${NC}"
