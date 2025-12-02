#!/bin/bash

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Testing Prepare Progress with Kantra                     ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""

KANTRA_DIR="/Users/tsanders/Workspace/kantra"
RULESET_DIR="/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive"
TACKLE2_UI_DIR="/Users/tsanders/Workspace/tackle2-ui"

# Check kantra exists
if [ ! -f "$KANTRA_DIR/kantra" ]; then
    echo -e "${RED}✗ Kantra binary not found${NC}"
    echo -e "${YELLOW}  Building kantra...${NC}"
    cd "$KANTRA_DIR"
    if ! go build -o kantra .; then
        echo -e "${RED}✗ Failed to build kantra${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Kantra built${NC}"
fi

echo -e "${GREEN}✓ Found kantra binary${NC}"
echo -e "${GREEN}✓ Ruleset: $RULESET_DIR${NC}"
echo -e "${GREEN}✓ Source: $TACKLE2_UI_DIR${NC}"
echo ""

echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}WATCH FOR: 'Preparing nodejs provider... X/Y files (Z%)'${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo ""

# Run kantra analyze with progress reporting (enabled by default)
"$KANTRA_DIR/kantra" analyze \
  --input="$TACKLE2_UI_DIR" \
  --rules="$RULESET_DIR" \
  --output=/tmp/kantra-output \
  --overwrite

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Analysis complete!${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "Output: /tmp/kantra-output/output.yaml"
echo ""
