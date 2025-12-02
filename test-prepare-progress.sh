#!/bin/bash

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Configuration
ANALYZER_DIR="/Users/tsanders/Workspace/analyzer-lsp"
RULESET_DIR="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive"
TACKLE2_UI_DIR="/Users/tsanders/Workspace/tackle2-ui"
TS_LSP_PATH="/opt/homebrew/bin/typescript-language-server"

echo -e "${BLUE}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Testing Prepare Progress Reporting Feature              ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""

# Step 1: Verify directories exist
echo -e "${YELLOW}[1/6]${NC} Verifying directories and dependencies..."
if [ ! -d "$ANALYZER_DIR" ]; then
    echo -e "${RED}✗ Analyzer directory not found: $ANALYZER_DIR${NC}"
    exit 1
fi

if [ ! -d "$RULESET_DIR" ]; then
    echo -e "${RED}✗ Ruleset directory not found: $RULESET_DIR${NC}"
    exit 1
fi

if [ ! -d "$TACKLE2_UI_DIR" ]; then
    echo -e "${RED}✗ Tackle2-UI directory not found: $TACKLE2_UI_DIR${NC}"
    exit 1
fi

if [ ! -f "$TS_LSP_PATH" ]; then
    echo -e "${RED}✗ TypeScript language server not found: $TS_LSP_PATH${NC}"
    echo -e "${YELLOW}  Install it with: npm install -g typescript-language-server${NC}"
    exit 1
fi

echo -e "${GREEN}✓ All directories and dependencies found${NC}"
echo ""

# Step 2: Count rules
echo -e "${YELLOW}[2/6]${NC} Counting rules in ruleset..."
RULE_COUNT=$(find "$RULESET_DIR" -name "*.yaml" -type f | wc -l | tr -d ' ')
echo -e "${GREEN}✓ Found $RULE_COUNT rule files${NC}"
echo ""

# Step 3: Build analyzer
echo -e "${YELLOW}[3/6]${NC} Building analyzer..."
cd "$ANALYZER_DIR"
if go build ./cmd/analyzer; then
    echo -e "${GREEN}✓ Analyzer built successfully${NC}"
else
    echo -e "${RED}✗ Failed to build analyzer${NC}"
    exit 1
fi
echo ""

# Step 4: Create temporary provider settings
echo -e "${YELLOW}[4/6]${NC} Creating provider settings..."
SETTINGS_FILE=$(mktemp /tmp/analyzer-settings.XXXXXX)
GENERIC_PROVIDER="${ANALYZER_DIR}/generic-external-provider"

if [ ! -f "$GENERIC_PROVIDER" ]; then
    echo -e "${RED}✗ Generic external provider not found: $GENERIC_PROVIDER${NC}"
    echo -e "${YELLOW}  Building it now...${NC}"
    cd "$ANALYZER_DIR/external-providers/generic-external-provider"
    if ! go build -o "$GENERIC_PROVIDER" .; then
        echo -e "${RED}✗ Failed to build generic-external-provider${NC}"
        exit 1
    fi
    cd "$ANALYZER_DIR"
    echo -e "${GREEN}✓ Built generic-external-provider${NC}"
fi

cat > "$SETTINGS_FILE" <<EOF
- name: nodejs
  binaryPath: $GENERIC_PROVIDER
  initConfig:
    - location: $TACKLE2_UI_DIR
      providerSpecificConfig:
        lspServerPath: $TS_LSP_PATH
        lspServerArgs:
          - "--stdio"
        lspServerName: nodejs
EOF
echo -e "${GREEN}✓ Settings created: $SETTINGS_FILE${NC}"
echo ""

# Step 5: Create output directory
echo -e "${YELLOW}[5/6]${NC} Preparing output..."
OUTPUT_FILE=$(mktemp /tmp/analyzer-output.XXXXXX)
echo -e "${GREEN}✓ Output will be written to: $OUTPUT_FILE${NC}"
echo ""

# Step 6: Run analyzer with progress monitoring
echo -e "${YELLOW}[6/6]${NC} Running analyzer..."
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Watch for progress updates during Prepare() phase:${NC}"
echo -e "${BLUE}  - 'Preparing nodejs provider... X/Y files (Z%)'${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo ""

# Run the analyzer
"$ANALYZER_DIR/analyzer" \
  --rules="$RULESET_DIR" \
  --provider-settings="$SETTINGS_FILE" \
  --output-file="$OUTPUT_FILE" \
  --progress-format=text \
  --progress-output=stderr \
  --verbose=5

ANALYZER_EXIT_CODE=$?

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"

if [ $ANALYZER_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ Analyzer completed successfully!${NC}"
    echo ""
    echo -e "${BLUE}Summary:${NC}"
    echo -e "  Ruleset:  $RULESET_DIR ($RULE_COUNT rules)"
    echo -e "  Codebase: $TACKLE2_UI_DIR"
    echo -e "  Output:   $OUTPUT_FILE"
    echo -e "  Settings: $SETTINGS_FILE"
    echo ""

    # Show incident count if available
    if [ -f "$OUTPUT_FILE" ]; then
        INCIDENT_COUNT=$(grep -c "uri:" "$OUTPUT_FILE" 2>/dev/null || echo "0")
        echo -e "${GREEN}  Incidents found: $INCIDENT_COUNT${NC}"
    fi

    echo ""
    echo -e "${YELLOW}Note: Temporary files created:${NC}"
    echo -e "  - $SETTINGS_FILE"
    echo -e "  - $OUTPUT_FILE"
    echo -e "${YELLOW}  Clean up with: rm $SETTINGS_FILE $OUTPUT_FILE${NC}"
else
    echo -e "${RED}✗ Analyzer failed with exit code: $ANALYZER_EXIT_CODE${NC}"
    echo ""
    echo -e "${YELLOW}Temporary files for debugging:${NC}"
    echo -e "  Settings: $SETTINGS_FILE"
    echo -e "  Output:   $OUTPUT_FILE"
    exit $ANALYZER_EXIT_CODE
fi

echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}        Progress Reporting Test Complete!              ${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
