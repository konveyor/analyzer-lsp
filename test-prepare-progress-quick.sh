#!/bin/bash

# Quick test for prepare progress reporting
# This runs a minimal test with just one rule to verify progress appears

set -e

echo "🚀 Quick Progress Test"
echo "====================="
echo ""

# Build analyzer
echo "Building analyzer..."
go build ./cmd/analyzer
echo "✓ Built"
echo ""

# Create minimal rule
echo "Creating test rule..."
cat > /tmp/test-rule.yaml <<EOF
- ruleID: test-button-usage
  when:
    nodejs.referenced:
      pattern: Button
  message: Found Button usage
EOF

# Create settings
ANALYZER_DIR="$(pwd)"
cat > /tmp/test-settings.yaml <<EOF
- name: nodejs
  binaryPath: ${ANALYZER_DIR}/generic-external-provider
  initConfig:
    - location: /Users/tsanders/Workspace/tackle2-ui
      providerSpecificConfig:
        lspServerPath: /opt/homebrew/bin/typescript-language-server
        lspServerArgs:
          - "--stdio"
        lspServerName: nodejs
EOF

echo "✓ Rule and settings created"
echo ""
echo "Running analyzer..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "WATCH FOR: 'Preparing nodejs provider... X/Y files'"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

./analyzer \
  --rules=/tmp/test-rule.yaml \
  --provider-settings=/tmp/test-settings.yaml \
  --output-file=/tmp/test-output.yaml \
  --progress-format=text \
  --progress-output=stderr \
  --verbose=5

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✓ Test complete!"
echo ""
echo "If you saw 'Preparing nodejs provider...' updates,"
echo "then progress reporting is working! 🎉"
echo ""
echo "Cleanup: rm /tmp/test-{rule,settings,output}.yaml"
