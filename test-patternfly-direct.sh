#!/bin/bash

#############################################################################
# PatternFly Migration Analysis Test Script - Direct Approach
#############################################################################
# This script tests the improved nodejs.referenced implementation using
# the PatternFly 5→6 migration ruleset against the tackle2-ui codebase.
#
# Uses the "direct approach" recommended for testing:
# 1. Starts local provider server with your new changes
# 2. Runs analyzer directly (not through kantra containers)
#############################################################################

set -e  # Exit on error

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Paths
ANALYZER_LSP_DIR="/Users/tsanders/Workspace/analyzer-lsp"
TACKLE2_UI_DIR="/Users/tsanders/Workspace/tackle2-ui"
#RULESET_DIR="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-improved-detection"
RULESET_DIR="/tmp/patternfly-cleaned"
OUTPUT_DIR="/tmp/patternfly-test-$(date +%Y%m%d-%H%M%S)"

# Binaries
ANALYZER_BIN="${ANALYZER_LSP_DIR}/build/konveyor-analyzer"
PROVIDER_BIN="${ANALYZER_LSP_DIR}/build/generic-external-provider"

# Test configuration
PROVIDER_PORT=14654
PROVIDER_SETTINGS="/tmp/patternfly-provider-settings.json"
LOG_FILE="${OUTPUT_DIR}/analysis.log"
PROVIDER_LOG="${OUTPUT_DIR}/provider.log"
OUTPUT_FILE="${OUTPUT_DIR}/output.yaml"

echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  PatternFly 5→6 Migration Analysis - Direct Testing Approach  ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

#############################################################################
# Step 1: Pre-flight Checks
#############################################################################
echo -e "${YELLOW}[1/7]${NC} Running pre-flight checks..."

# Check binaries exist
if [ ! -f "${ANALYZER_BIN}" ]; then
    echo -e "${RED}ERROR: Analyzer binary not found at ${ANALYZER_BIN}${NC}"
    echo "Please run: cd ${ANALYZER_LSP_DIR} && make build"
    exit 1
fi

if [ ! -f "${PROVIDER_BIN}" ]; then
    echo -e "${RED}ERROR: Provider binary not found at ${PROVIDER_BIN}${NC}"
    echo "Please run: cd ${ANALYZER_LSP_DIR} && make build"
    exit 1
fi

# Check ruleset exists
if [ ! -d "${RULESET_DIR}" ]; then
    echo -e "${RED}ERROR: Ruleset directory not found at ${RULESET_DIR}${NC}"
    exit 1
fi

RULE_COUNT=$(ls -1 "${RULESET_DIR}"/*.yaml 2>/dev/null | grep -v ruleset.yaml | wc -l | tr -d ' ')
echo -e "  ${GREEN}✓${NC} Found analyzer binary: ${ANALYZER_BIN}"
echo -e "  ${GREEN}✓${NC} Found provider binary: ${PROVIDER_BIN}"
echo -e "  ${GREEN}✓${NC} Found ruleset with ${RULE_COUNT} rule files"
echo -e "  ${GREEN}✓${NC} Target codebase: ${TACKLE2_UI_DIR}"
echo ""

#############################################################################
# Step 2: Create Output Directory
#############################################################################
echo -e "${YELLOW}[2/7]${NC} Creating output directory..."
mkdir -p "${OUTPUT_DIR}"
echo -e "  ${GREEN}✓${NC} Output directory: ${OUTPUT_DIR}"
echo ""

#############################################################################
# Step 3: Create Provider Settings
#############################################################################
echo -e "${YELLOW}[3/7]${NC} Creating provider settings..."

cat > "${PROVIDER_SETTINGS}" << EOF
[
  {
    "name": "nodejs",
    "address": "localhost:${PROVIDER_PORT}",
    "initConfig": [
      {
        "location": "${TACKLE2_UI_DIR}",
        "analysisMode": "full",
        "providerSpecificConfig": {
          "lspServerName": "nodejs",
          "lspServerPath": "/opt/homebrew/bin/typescript-language-server",
          "lspServerArgs": ["--stdio"],
          "lspServerInitializationOptions": "",
          "workspaceFolders": ["file://${TACKLE2_UI_DIR}"],
          "dependencyFolders": [""],
          "dependencyProviderPath": ""
        }
      }
    ]
  }
]
EOF

echo -e "  ${GREEN}✓${NC} Provider settings: ${PROVIDER_SETTINGS}"
echo -e "  ${GREEN}✓${NC} Provider port: ${PROVIDER_PORT}"
echo -e "  ${GREEN}✓${NC} Workspace: ${TACKLE2_UI_DIR}"
echo ""

#############################################################################
# Step 4: Stop Any Existing Provider
#############################################################################
echo -e "${YELLOW}[4/7]${NC} Checking for existing provider processes..."

EXISTING_PIDS=$(lsof -ti:${PROVIDER_PORT} 2>/dev/null || true)
if [ -n "${EXISTING_PIDS}" ]; then
    echo -e "  ${YELLOW}⚠${NC} Found existing process on port ${PROVIDER_PORT}"
    for PID in ${EXISTING_PIDS}; do
        echo -e "  ${YELLOW}⚠${NC} Killing PID ${PID}..."
        kill -9 ${PID} 2>/dev/null || true
    done
    sleep 2
fi

# Double check it's dead
if lsof -ti:${PROVIDER_PORT} >/dev/null 2>&1; then
    echo -e "${RED}ERROR: Could not kill process on port ${PROVIDER_PORT}${NC}"
    exit 1
fi

echo -e "  ${GREEN}✓${NC} Port ${PROVIDER_PORT} is available"
echo ""

#############################################################################
# Step 5: Start Provider Server
#############################################################################
echo -e "${YELLOW}[5/7]${NC} Starting provider server with your new changes..."

"${PROVIDER_BIN}" --port ${PROVIDER_PORT} > "${PROVIDER_LOG}" 2>&1 &
PROVIDER_PID=$!

echo -e "  ${GREEN}✓${NC} Provider started (PID: ${PROVIDER_PID})"
echo -e "  ${GREEN}✓${NC} Provider log: ${PROVIDER_LOG}"

# Wait for provider to be ready
echo -n "  Waiting for provider to be ready"
for i in {1..10}; do
    if lsof -ti:${PROVIDER_PORT} >/dev/null 2>&1; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 1
    if [ $i -eq 10 ]; then
        echo -e " ${RED}✗${NC}"
        echo -e "${RED}ERROR: Provider failed to start${NC}"
        echo "Provider log:"
        cat "${PROVIDER_LOG}"
        exit 1
    fi
done
echo ""

#############################################################################
# Step 6: Run Analysis
#############################################################################
echo -e "${YELLOW}[6/7]${NC} Running analysis with ${RULE_COUNT} PatternFly migration rules..."
echo -e "  ${BLUE}ℹ${NC} This may take a few minutes depending on rule complexity"
echo ""

START_TIME=$(date +%s)

"${ANALYZER_BIN}" \
  --rules="${RULESET_DIR}" \
  --provider-settings="${PROVIDER_SETTINGS}" \
  --output-file="${OUTPUT_FILE}" \
  --verbose=5 2>&1 | tee "${OUTPUT_DIR}/console.log"

EXIT_CODE=$?
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
if [ ${EXIT_CODE} -eq 0 ]; then
    echo -e "  ${GREEN}✓${NC} Analysis completed successfully in ${DURATION} seconds"
else
    echo -e "  ${RED}✗${NC} Analysis failed with exit code ${EXIT_CODE}"
    echo "Check logs at: ${LOG_FILE}"
    kill ${PROVIDER_PID} 2>/dev/null || true
    exit ${EXIT_CODE}
fi
echo ""

#############################################################################
# Step 7: Generate Summary
#############################################################################
echo -e "${YELLOW}[7/7]${NC} Generating analysis summary..."
echo ""

# Stop provider
kill ${PROVIDER_PID} 2>/dev/null || true

# Count violations
if [ -f "${OUTPUT_FILE}" ]; then
    VIOLATION_COUNT=$(grep -c "ruleID:" "${OUTPUT_FILE}" 2>/dev/null || echo "0")
    INCIDENT_COUNT=$(grep -c "uri: file://" "${OUTPUT_FILE}" 2>/dev/null || echo "0")

    echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║                      ANALYSIS SUMMARY                          ║${NC}"
    echo -e "${BLUE}╠════════════════════════════════════════════════════════════════╣${NC}"
    echo -e "${BLUE}║${NC} Duration:           ${DURATION} seconds"
    echo -e "${BLUE}║${NC} Rules analyzed:     ${RULE_COUNT}"
    echo -e "${BLUE}║${NC} Violations found:   ${VIOLATION_COUNT}"
    echo -e "${BLUE}║${NC} Total incidents:    ${INCIDENT_COUNT}"
    echo -e "${BLUE}╠════════════════════════════════════════════════════════════════╣${NC}"
    echo -e "${BLUE}║${NC} Output files:"
    echo -e "${BLUE}║${NC}   - Analysis results: ${OUTPUT_FILE}"
    echo -e "${BLUE}║${NC}   - Provider log:     ${PROVIDER_LOG}"
    echo -e "${BLUE}║${NC}   - Console output:   ${OUTPUT_DIR}/console.log"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Show top 5 violations
    if [ ${VIOLATION_COUNT} -gt 0 ]; then
        echo -e "${GREEN}Top violations found:${NC}"
        grep "ruleID:" "${OUTPUT_FILE}" | head -5 | sed 's/^/  /'
        echo ""
    fi

    # Check provider log for performance metrics
    echo -e "${GREEN}Provider performance metrics:${NC}"
    grep -E "(import-based search complete|Scanning for import|Found imports)" "${PROVIDER_LOG}" | tail -10 | sed 's/^/  /'
    echo ""

    echo -e "${GREEN}✓ Test completed successfully!${NC}"
    echo ""
    echo -e "To view full results:"
    echo -e "  ${BLUE}cat ${OUTPUT_FILE}${NC}"
    echo ""
    echo -e "To view provider performance details:"
    echo -e "  ${BLUE}grep 'import-based search complete' ${PROVIDER_LOG}${NC}"
    echo ""
else
    echo -e "${RED}ERROR: Output file not found at ${OUTPUT_FILE}${NC}"
    echo "Check logs:"
    echo "  Provider log: ${PROVIDER_LOG}"
    echo "  Console log: ${OUTPUT_DIR}/console.log"
    exit 1
fi

exit 0
