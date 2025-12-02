#!/bin/bash

# Compare container vs containerless mode
# This isolates container overhead from concurrency issues

APP_PATH="/Users/tsanders/Workspace/tackle2-ui"
RULESET="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive"
KANTRA="/Users/tsanders/Workspace/kantra/kantra"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Container vs Containerless Comparison${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Test 1: Container mode (default for nodejs provider)
echo -e "${YELLOW}Test 1: Container mode${NC}"
OUTPUT_CONTAINER="/tmp/test-container-$(date +%s)"
START_CONTAINER=$(date +%s)

cd /Users/tsanders/Workspace/kantra
./kantra analyze \
    --input "$APP_PATH" \
    --output "$OUTPUT_CONTAINER" \
    --rules "$RULESET" \
    --overwrite \
    > /tmp/container-mode-test.log 2>&1

CONTAINER_EXIT=$?
END_CONTAINER=$(date +%s)
CONTAINER_TIME=$((END_CONTAINER - START_CONTAINER))

if [ $CONTAINER_EXIT -ne 0 ]; then
    echo -e "${RED}Container test encountered errors (exit code: $CONTAINER_EXIT)${NC}"
    echo -e "${YELLOW}But timing was captured: ${CONTAINER_TIME} seconds${NC}"
else
    echo -e "${GREEN}Container mode completed in ${CONTAINER_TIME} seconds${NC}"
fi
echo ""

# Wait between tests
echo "Waiting 10 seconds before next test..."
sleep 10

# Test 2: Try to force containerless if possible
# Note: nodejs provider might not support this, but let's try
echo -e "${YELLOW}Test 2: Attempting containerless mode${NC}"
OUTPUT_CONTAINERLESS="/tmp/test-containerless-$(date +%s)"
START_CONTAINERLESS=$(date +%s)

cd /Users/tsanders/Workspace/kantra
./kantra analyze \
    --input "$APP_PATH" \
    --output "$OUTPUT_CONTAINERLESS" \
    --rules "$RULESET" \
    --run-local=true \
    --overwrite \
    > /tmp/containerless-mode-test.log 2>&1

CONTAINERLESS_EXIT=$?
END_CONTAINERLESS=$(date +%s)
CONTAINERLESS_TIME=$((END_CONTAINERLESS - START_CONTAINERLESS))

if [ $CONTAINERLESS_EXIT -ne 0 ]; then
    echo -e "${RED}Containerless test encountered errors (exit code: $CONTAINERLESS_EXIT)${NC}"
    echo -e "${YELLOW}But timing was captured: ${CONTAINERLESS_TIME} seconds${NC}"
else
    echo -e "${GREEN}Containerless mode completed in ${CONTAINERLESS_TIME} seconds${NC}"
fi
echo ""

# Calculate difference
SAVED_TIME=$((CONTAINER_TIME - CONTAINERLESS_TIME))
if [ $CONTAINER_TIME -gt 0 ]; then
    IMPROVEMENT=$(echo "scale=2; $CONTAINER_TIME / $CONTAINERLESS_TIME" | bc)
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
echo -e "Container mode:      ${CONTAINER_TIME} seconds"
echo -e "Containerless mode:  ${CONTAINERLESS_TIME} seconds"
echo ""
if [ "$SAVED_TIME" -gt 0 ]; then
    echo -e "${GREEN}Time saved: ${SAVED_TIME} seconds${NC}"
    echo -e "${GREEN}Speedup: ${IMPROVEMENT}x (${PERCENT}% faster)${NC}"
else
    echo -e "${RED}Containerless was slower by $((SAVED_TIME * -1)) seconds${NC}"
fi
echo ""

# Save results
cat > /tmp/containerless-comparison.txt <<EOF
Container vs Containerless Mode Comparison
Date: $(date)

Configuration:
- Input: $APP_PATH
- Rules: $RULESET
- Both tests use baseline (official) image with 1 worker

Results:
- Container time:      ${CONTAINER_TIME} seconds
- Containerless time:  ${CONTAINERLESS_TIME} seconds
- Time saved:          ${SAVED_TIME} seconds
- Speedup:             ${IMPROVEMENT}x (${PERCENT}% improvement)

Exit codes:
- Container:     ${CONTAINER_EXIT}
- Containerless: ${CONTAINERLESS_EXIT}

Detailed logs:
- Container:     /tmp/container-mode-test.log
- Containerless: /tmp/containerless-mode-test.log

Note: Check logs to verify if containerless mode actually worked for nodejs provider.
The --run-local flag primarily affects Java providers.

Output directories (preserved):
- Container:     ${OUTPUT_CONTAINER}
- Containerless: ${OUTPUT_CONTAINERLESS}
EOF

echo "Detailed results saved to: /tmp/containerless-comparison.txt"
echo ""
echo -e "${YELLOW}Important: Check the logs to verify both modes actually differ${NC}"
echo "  - grep 'provider containers' /tmp/container-mode-test.log"
echo "  - grep 'provider containers' /tmp/containerless-mode-test.log"
echo ""
echo -e "${YELLOW}Output directories preserved at:${NC}"
echo "  - $OUTPUT_CONTAINER"
echo "  - $OUTPUT_CONTAINERLESS"
