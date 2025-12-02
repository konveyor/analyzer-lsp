#!/bin/bash

# Compare performance: baseline (1 worker) vs optimized (10 workers)
# This test runs the same analysis twice and measures the time difference

APP_PATH="/Users/tsanders/Workspace/tackle2-ui"
RULESET="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive"
KANTRA="/Users/tsanders/Workspace/kantra/kantra"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Performance Comparison Test${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Test 1: Baseline (no optimization, uses official quay.io image with 1 worker)
echo -e "${YELLOW}Test 1: Baseline (official image - 1 worker)${NC}"
OUTPUT_BASELINE="/tmp/test-baseline-$(date +%s)"
START_BASELINE=$(date +%s)

cd /Users/tsanders/Workspace/kantra
./kantra analyze \
    --input "$APP_PATH" \
    --output "$OUTPUT_BASELINE" \
    --rules "$RULESET" \
    --overwrite \
    > /tmp/baseline-test.log 2>&1

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

# Wait a bit between tests
echo "Waiting 10 seconds before next test..."
sleep 10

# Test 2: Optimized (local image with 10 workers)
echo -e "${YELLOW}Test 2: Optimized (10-worker pool)${NC}"
OUTPUT_OPTIMIZED="/tmp/test-optimized-$(date +%s)"
START_OPTIMIZED=$(date +%s)

cd /Users/tsanders/Workspace/kantra
GENERIC_PROVIDER_IMG=localhost/generic-provider:optimized ./kantra analyze \
    --input "$APP_PATH" \
    --output "$OUTPUT_OPTIMIZED" \
    --rules "$RULESET" \
    --overwrite \
    > /tmp/optimized-test.log 2>&1

OPTIMIZED_EXIT=$?
END_OPTIMIZED=$(date +%s)
OPTIMIZED_TIME=$((END_OPTIMIZED - START_OPTIMIZED))

if [ $OPTIMIZED_EXIT -ne 0 ]; then
    echo -e "${RED}Optimized test encountered errors (exit code: $OPTIMIZED_EXIT)${NC}"
    echo -e "${YELLOW}But timing was captured: ${OPTIMIZED_TIME} seconds${NC}"
else
    echo -e "${GREEN}Optimized completed successfully in ${OPTIMIZED_TIME} seconds${NC}"
fi
echo ""

# Calculate improvement
IMPROVEMENT=$(echo "scale=2; $BASELINE_TIME / $OPTIMIZED_TIME" | bc)
SAVED_TIME=$((BASELINE_TIME - OPTIMIZED_TIME))
PERCENT_IMPROVEMENT=$(echo "scale=1; ($IMPROVEMENT - 1) * 100" | bc)

# Results
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Results${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "Baseline (1 worker):     ${BASELINE_TIME} seconds"
echo -e "Optimized (10 workers):  ${OPTIMIZED_TIME} seconds"
echo -e ""
echo -e "${GREEN}Time saved: ${SAVED_TIME} seconds${NC}"
echo -e "${GREEN}Speedup: ${IMPROVEMENT}x (${PERCENT_IMPROVEMENT}% faster)${NC}"
echo ""

# Save results
cat > /tmp/performance-comparison.txt <<EOF
Performance Comparison: Worker Pool Optimization
Date: $(date)

Configuration:
- Input: $APP_PATH
- Rules: $RULESET
- Baseline: quay.io image (1 worker)
- Optimized: localhost/generic-provider:optimized (10 workers)

Results:
- Baseline time:  ${BASELINE_TIME} seconds
- Optimized time: ${OPTIMIZED_TIME} seconds
- Time saved:     ${SAVED_TIME} seconds
- Speedup:        ${IMPROVEMENT}x (${PERCENT_IMPROVEMENT}% improvement)

Exit codes:
- Baseline:  ${BASELINE_EXIT}
- Optimized: ${OPTIMIZED_EXIT}

Detailed logs:
- Baseline:  /tmp/baseline-test.log
- Optimized: /tmp/optimized-test.log

Output directories (preserved for inspection):
- Baseline:  ${OUTPUT_BASELINE}
- Optimized: ${OUTPUT_OPTIMIZED}
EOF

echo "Detailed results saved to: /tmp/performance-comparison.txt"
echo ""
echo -e "${YELLOW}Note: Output directories preserved at:${NC}"
echo "  - $OUTPUT_BASELINE"
echo "  - $OUTPUT_OPTIMIZED"
echo ""
echo -e "${YELLOW}Clean up manually when done: rm -rf $OUTPUT_BASELINE $OUTPUT_OPTIMIZED${NC}"
