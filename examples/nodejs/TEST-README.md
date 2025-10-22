# Test Files for TypeScript/React Support PR

This directory contains test rules and applications to verify the TypeScript/React support fixes.

## Quick Test

### Prerequisites
```bash
# Build the analyzer
make build

# Install TypeScript language server (optional, only needed for typescript.referenced tests)
npm install -g typescript-language-server typescript
```

### Test 1: Verify .tsx File Support

```bash
# Create test app
mkdir -p /tmp/test-tsx-app/src
cat > /tmp/test-tsx-app/src/Component.tsx <<'EOF'
import React from 'react';
export const MyComponent: React.FC = () => <div>Hello</div>;
EOF

# Create provider settings
cat > /tmp/provider-settings.json <<'EOF'
[{"name":"builtin","binaryPath":"","initConfig":[{"location":"/tmp/test-tsx-app","analysisMode":"full"}]}]
EOF

# Run test
./konveyor-analyzer \
  --provider-settings=/tmp/provider-settings.json \
  --rules=test/test-tsx-support.yaml \
  --output-file=/tmp/output.yaml

# Verify success
grep -q "violations:" /tmp/output.yaml && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected:** Should find violations in `Component.tsx`

### Test 2: Verify Brace Expansion

```bash
# Add more test files
echo "import React from 'react';" > /tmp/test-tsx-app/src/test.ts
echo "import React from 'react';" > /tmp/test-tsx-app/src/test.js
echo "import React from 'react';" > /tmp/test-tsx-app/src/test.jsx
echo ".example { color: var(--pf-global); }" > /tmp/test-tsx-app/src/test.css

# Run test
./konveyor-analyzer \
  --provider-settings=/tmp/provider-settings.json \
  --rules=test/test-brace-expansion.yaml \
  --output-file=/tmp/output.yaml

# Count matches (should be 5: .tsx, .ts, .js, .jsx files + 1 CSS)
echo "Violations found: $(grep -c 'uri: file://' /tmp/output.yaml)"
```

**Expected:** Should find violations in ALL file types

### Test 3: Verify node_modules is Skipped

```bash
# Add node_modules with many files
mkdir -p /tmp/test-tsx-app/node_modules
for i in {1..100}; do
  echo "import React from 'react';" > /tmp/test-tsx-app/node_modules/lib$i.tsx
done

# Time the analysis (should be fast)
time ./konveyor-analyzer \
  --provider-settings=/tmp/provider-settings.json \
  --rules=test/test-tsx-support.yaml \
  --output-file=/tmp/output.yaml
```

**Expected:**
- Completes in < 10 seconds
- Does NOT find violations in node_modules files
- Only finds violations in src/ directory

## Test Files

- `test-tsx-support.yaml` - Tests that `.tsx` files are detected
- `test-brace-expansion.yaml` - Tests brace expansion patterns like `*.{ts,tsx}`
- `README.md` - This file

## Performance Benchmark

### Before Fixes
- ❌ 0 `.tsx` files detected
- ❌ Scans node_modules (15+ minutes)
- ❌ Brace expansion doesn't work

### After Fixes
- ✅ `.tsx` and `.jsx` files detected
- ✅ Skips node_modules (< 10 seconds)
- ✅ Brace expansion works for all patterns

## Success Criteria

- [ ] `.tsx` files are scanned
- [ ] `.jsx` files are scanned
- [ ] `node_modules` is skipped
- [ ] Analysis completes in < 10 seconds
- [ ] Pattern `*.{ts,tsx}` matches both file types
- [ ] Pattern `*.{css,scss}` matches both file types
- [ ] Existing `*.tsx` patterns still work (backward compatibility)
