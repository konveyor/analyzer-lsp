# Code Review Response Plan - nodejs.referenced Import Search

This document outlines the plan to address code review feedback from shawn-hurley on PR #976.

## Review Comments Summary

1. Handling the '*' import cases
2. Normalize function could use helper methods instead of iterating on characters
3. Look ahead in a loop can be simplified
4. Use `os.LineSeparator` instead of '/r' or '/n' directly
5. Public methods vs private methods
6. Plan for adding back ability to search for things that are not imported in rules

---

## 1. Handle '*' import cases and mixed imports

**Location:** `findImportStatements` function (lines 507-603)

**Current Issue:**
The regex only matches:
- Named imports: `import { Card } from "package"`
- Default imports: `import Card from "package"`

But does NOT match:
- Namespace imports: `import * as Card from "package"`
- Mixed default + named: `import React, { useState } from "react"`
- Mixed default + namespace: `import React, * as ReactAll from "react"`

**Additional patterns that DON'T need handling:**
- Side-effect imports: `import "@patternfly/react-core/dist/styles/base.css"` - No symbols to search, safe to ignore
- `import * from "package"` - Invalid JavaScript syntax (must use `as`)

**Current Regex:**
```go
importRegex := regexp.MustCompile(
    `import\s+(?:\{([^}]*)\}|(\w+))\s+from\s+['"]([^'"]+)['"]`,
)
```

**Planned Changes:**

1. Update regex to handle all valid import patterns:
```go
// Matches:
// - import { Card } from "pkg"          (named only)
// - import Card from "pkg"              (default only)
// - import * as Card from "pkg"         (namespace only)
// - import React, { useState } from "pkg"  (default + named)
// - import React, * as All from "pkg"   (default + namespace - rare but valid)
importRegex := regexp.MustCompile(
    `import\s+(?:(\w+)\s*,\s*)?(?:\{([^}]*)\}|(\w+)|\*\s+as\s+(\w+))(?:\s*,\s*\*\s+as\s+(\w+))?\s+from\s+['"]([^'"]+)['"]`,
)
```

**Capture Groups:**
- Group 1: Default import (when mixed with named/namespace)
- Group 2: Named imports (braced list)
- Group 3: Default import (when alone)
- Group 4: Namespace import
- Group 5: Namespace import (when mixed with default - rare)
- Group 6: Package name

2. Update the pattern matching logic (lines 536-555) to check all groups:
```go
var defaultImport string
var namedImports string
var namespaceImport string

// Extract default import (group 1 for mixed, group 3 for standalone)
if matchIdx[2] != -1 && matchIdx[3] != -1 {
    defaultImport = normalized[matchIdx[2]:matchIdx[3]]
} else if matchIdx[6] != -1 && matchIdx[7] != -1 {
    defaultImport = normalized[matchIdx[6]:matchIdx[7]]
}

// Extract named imports (group 2)
if matchIdx[4] != -1 && matchIdx[5] != -1 {
    namedImports = normalized[matchIdx[4]:matchIdx[5]]
}

// Extract namespace import (group 4 or group 5 for mixed)
if matchIdx[8] != -1 && matchIdx[9] != -1 {
    namespaceImport = normalized[matchIdx[8]:matchIdx[9]]
} else if matchIdx[10] != -1 && matchIdx[11] != -1 {
    namespaceImport = normalized[matchIdx[10]:matchIdx[11]]
}

// Check if pattern matches any import type
patternFound := false
if defaultImport == pattern {
    patternFound = true
} else if namedImports != "" && strings.Contains(namedImports, pattern) {
    patternFound = true
} else if namespaceImport == pattern {
    patternFound = true
}
```

**Test Cases to Add:**
- `import * as PatternFly from "@patternfly/react-core"` - should match "PatternFly"
- `import React, { useState } from "react"` - should match "React" and "useState"
- `import Card, { CardBody } from "@patternfly/react-core"` - should match "Card" and "CardBody"
- `import React, * as ReactAll from "react"` - should match "React" and "ReactAll"
- `import "@patternfly/styles.css"` - should NOT match any pattern (side-effect only)
- Should NOT match pattern if it's not the exact identifier

---

## 2. Refactor normalize function to use helper methods

**Location:** `normalizeMultilineImports` function (lines 629-730)

**Current Issue:**
The function is ~100 lines long and handles multiple concerns:
- String delimiter detection
- Escape sequence handling
- Brace depth tracking
- Import statement end detection

This makes it hard to read, test, and maintain.

**Planned Changes:**

Create the following helper methods:

### Helper Method 1: String Handling
```go
// isStringDelimiter checks if a character is a string delimiter
func isStringDelimiter(ch byte) bool {
    return ch == '"' || ch == '\'' || ch == '`'
}

// isEscapedQuote determines if a quote at position is escaped
func isEscapedQuote(content string, pos int) bool {
    escapeCount := 0
    for j := pos - 1; j >= 0 && content[j] == '\\'; j-- {
        escapeCount++
    }
    // If odd number of backslashes, quote is escaped
    return escapeCount%2 == 1
}
```

### Helper Method 2: Import Statement End Detection
```go
// hasCompletedImportStatement checks if we've seen a complete import with "from" clause
// within the current import statement scope
func (sc *NodeServiceClient) hasCompletedImportStatement(content string, importStart, currentPos int) bool {
    if currentPos <= importStart+6 {
        return false
    }

    // Look at just the current import statement
    snippet := content[importStart:min(currentPos+1, len(content))]
    if len(snippet) <= 10 {
        return false
    }

    // Check the last ~50 chars for "from" keyword followed by a quote
    start := len(snippet) - min(50, len(snippet))
    last50 := snippet[start:]

    return fromKeywordRegex.MatchString(last50)
}
```

### Refactored Main Function
The `normalizeMultilineImports` function will become much cleaner:

```go
func (sc *NodeServiceClient) normalizeMultilineImports(content string) string {
    var result strings.Builder
    result.Grow(len(content))

    i := 0
    for i < len(content) {
        // Look for "import" keyword
        if i+6 <= len(content) && content[i:i+6] == "import" {
            if i > 0 && isIdentifierChar(rune(content[i-1])) {
                result.WriteByte(content[i])
                i++
                continue
            }

            importStart := i
            result.WriteString("import")
            i += 6

            i = sc.normalizeImportStatement(&result, content, i, importStart)
        } else {
            result.WriteByte(content[i])
            i++
        }
    }

    return result.String()
}

func (sc *NodeServiceClient) normalizeImportStatement(result *strings.Builder, content string, pos, importStart int) int {
    braceDepth := 0
    inString := false
    stringChar := byte(0)
    i := pos

    for i < len(content) {
        ch := content[i]

        // Handle string entry/exit
        if !inString && isStringDelimiter(ch) {
            inString = true
            stringChar = ch
            result.WriteByte(ch)
            i++
            continue
        } else if inString && ch == stringChar && !isEscapedQuote(content, i) {
            inString = false
            result.WriteByte(ch)
            i++
            continue
        } else if inString {
            result.WriteByte(ch)
            i++
            continue
        }

        // Handle import statement structure (not in strings)
        switch ch {
        case '{':
            braceDepth++
            result.WriteByte(ch)
            i++
        case '}':
            braceDepth--
            result.WriteByte(ch)
            i++
        case '\n', '\r':
            if braceDepth == 0 && sc.hasCompletedImportStatement(content, importStart, i) {
                result.WriteByte('\n')
                i++
                return i // Import complete
            }
            result.WriteByte(' ')
            i++
        case ';':
            result.WriteByte(ch)
            i++
            return i // Import complete
        default:
            result.WriteByte(ch)
            i++
        }
    }

    return i
}
```

**Benefits:**
- Each helper is testable independently
- Main function logic is clearer
- Easier to understand and modify
- Better separation of concerns

---

## 3. Simplify look-ahead in loop logic

**Location:** Lines 694-710 in `normalizeMultilineImports`

**Current Issue:**
The "from" keyword detection is complex and deeply nested:

```go
if braceDepth == 0 && i > importStart+6 {
    snippet := content[importStart:min(i+1, len(content))]
    if len(snippet) > 10 {
        start := len(snippet) - min(50, len(snippet))
        last50 := snippet[start:]
        if fromKeywordRegex.MatchString(last50) {
            // Import statement is complete
            result.WriteByte('\n')
            i++
            break
        }
    }
}
```

**Planned Changes:**

This is addressed by refactoring #2 above. The `hasCompletedImportStatement` helper method simplifies this logic by:
1. Extracting it to a named function with clear purpose
2. Early returns for invalid states
3. Clear variable names
4. Single responsibility

The main loop becomes a simple switch statement with a call to the helper.

---

## 4. Use appropriate line separator handling

**Location:** Lines 694, 712 (newline handling in `normalizeMultilineImports`)

**Current Issue:**
Hardcoded checks for `\n` and `\r` aren't explicitly documented as cross-platform.

**Investigation:**
Go doesn't have `os.LineSeparator` constant. The standard library approach is to explicitly handle both `\n` (Unix/Linux/Mac) and `\r\n` (Windows), with `\r` (old Mac) as a fallback.

**Planned Changes:**

Since we're processing JavaScript/TypeScript source files which use `\n` as the standard line ending (even on Windows, most modern editors normalize to `\n`), the current approach is actually correct. However, we should:

1. Add a comment explaining the line ending handling:

```go
case '\n', '\r':
    // Handle line breaks - we check both \n (Unix/Mac) and \r (Windows/old Mac)
    // JavaScript/TypeScript files typically use \n, but we handle both for robustness
    if braceDepth == 0 && sc.hasCompletedImportStatement(content, importStart, i) {
        result.WriteByte('\n')
        i++
        return i // Import complete
    }
    result.WriteByte(' ') // Replace line breaks with spaces within import statements
    i++
```

2. Consider adding a preprocessing step to normalize `\r\n` to `\n` at the start of the function for consistency:

```go
func (sc *NodeServiceClient) normalizeMultilineImports(content string) string {
    // Normalize Windows line endings to Unix for consistent processing
    content = strings.ReplaceAll(content, "\r\n", "\n")

    var result strings.Builder
    result.Grow(len(content))
    // ... rest of function
}
```

**Decision:** Add the preprocessing step to normalize `\r\n` to `\n`, then only check for `\n` in the switch statement. This is simpler and more maintainable.

---

## 5. Public methods vs private methods

**Location:** Lines 758-787 (test helper exports)

**Current Issue:**
The code exports public wrapper methods just for testing:
- `NormalizeMultilineImportsPublic`
- `FindImportStatementsPublic`
- `IsIdentifierCharPublic`
- `FileInfo` struct

This is a Go anti-pattern. Go's testing convention is that test files in the same package can access private (unexported) methods directly.

**Planned Changes:**

1. **Verify test file package declaration:**
   Check that `import_search_test.go` uses `package nodejs` (not `package nodejs_test`)

2. **Remove all public test wrappers:**
   Delete lines 758-787 entirely:
   - Remove `FileInfo` exported struct
   - Remove `NormalizeMultilineImportsPublic`
   - Remove `FindImportStatementsPublic`
   - Remove `IsIdentifierCharPublic`

3. **Update test file:**
   In `import_search_test.go`, update calls to use private methods directly:
   - `sc.NormalizeMultilineImportsPublic(...)` → `sc.normalizeMultilineImports(...)`
   - `sc.FindImportStatementsPublic(...)` → `sc.findImportStatements(...)`
   - `sc.IsIdentifierCharPublic(...)` → `isIdentifierChar(...)`
   - `FileInfo{...}` → `fileInfo{...}`

**Benefits:**
- Cleaner public API surface
- Follows Go conventions
- Reduces maintenance burden
- Makes it clear these are internal implementation details

---

## 6. Plan for adding non-import search capability

**Current Issue:**
The new import-based search algorithm only finds symbols that are explicitly imported. This means it cannot find:
- Global variables/functions (e.g., `window`, `document`, `console`)
- Built-in types (e.g., `Array`, `Object`, `Promise`)
- Symbols injected by frameworks without explicit imports
- Dynamic usage patterns that don't require imports

**Why This Might Be Needed:**
Some analysis rules may need to detect usage of globals or built-in APIs that aren't imported. For example:
- Detecting usage of deprecated global APIs
- Finding direct DOM manipulation with `document`
- Identifying console.log statements for cleanup

**Proposed Solutions:**

### Option A: Fallback Mechanism (Recommended for MVP)
Add a fallback to `workspace/symbol` when no imports are found:

```go
// In EvaluateReferenced, after line 242:
if len(importLocations) == 0 {
    sc.Log.Info("No imports found, trying workspace/symbol fallback",
        "query", query)

    // Fall back to workspace/symbol search
    symbols := []protocol.WorkspaceSymbol{}
    params := protocol.WorkspaceSymbolParams{Query: query}
    err = sc.Conn.Call(ctx, "workspace/symbol", params).Await(ctx, &symbols)

    if err != nil || len(symbols) == 0 {
        return resp{Matched: false}, nil
    }

    // Use existing EvaluateSymbols logic
    incidentsMap, err := sc.EvaluateSymbols(ctx, symbols)
    // ... convert to response
}
```

**Pros:**
- Best of both worlds: fast import-based search + comprehensive fallback
- Transparent to rule authors
- No API changes needed

**Cons:**
- Fallback may still miss symbols (original problem)
- Adds complexity
- May be slower for patterns that truly don't exist

### Option B: Configuration Flag
Add a config option to control behavior:

```yaml
# In provider config
nodejs:
  searchMode: "imports-only" # or "workspace-symbol" or "hybrid"
```

**Pros:**
- Explicit control over behavior
- Can optimize for specific use cases
- Easy to A/B test performance

**Cons:**
- More configuration surface area
- Users need to understand the trade-offs
- Rules may behave differently based on config

### Option C: Separate Capability
Create a new capability for non-import searches:

```yaml
# For imported symbols (fast, accurate)
when:
  nodejs.referenced:
    pattern: Card

# For globals/built-ins (slower, comprehensive)
when:
  nodejs.symbol:
    pattern: console
```

**Pros:**
- Clear separation of concerns
- Rule authors explicitly choose behavior
- Can optimize each path independently

**Cons:**
- More API surface area
- Need to document when to use which
- Migration burden for existing rules

### Option D: Text-based fallback
Use `builtin.filecontent` as documented fallback:

```yaml
# Document this pattern for globals
when:
  or:
  - nodejs.referenced:
      pattern: console
  - builtin.filecontent:
      pattern: \bconsole\b
```

**Pros:**
- No code changes needed
- Uses existing capabilities
- Explicit about the use case

**Cons:**
- Verbose for rule authors
- Not semantic (text matching)
- Defeats the purpose of removing combo rules

---

### Recommended Approach

**Phase 1 (Current PR):**
1. Document the limitation in code comments
2. Add "Known Limitations" section to `EvaluateReferenced` docstring:

```go
// Known Limitations:
// - This algorithm only finds symbols that are explicitly imported
// - Global variables (window, document, console) won't be found unless imported
// - Built-in types (Array, Object, Promise) require explicit imports to be detected
// - For finding globals, consider using builtin.filecontent as a fallback in rules
```

3. Add TODO comment suggesting future enhancement:

```go
// TODO: Consider adding fallback to workspace/symbol when no imports found
// This would enable detection of globals and built-ins at the cost of performance
// See CODE_REVIEW_RESPONSE_PLAN.md section 6 for detailed options
```

**Phase 2 (Future Enhancement):**
Implement Option A (fallback mechanism) based on real-world usage feedback:
- Track metrics on how often fallback would be needed
- Get feedback from rule authors on use cases
- Implement with feature flag to enable/disable
- Add configuration option if needed based on performance impact

**Phase 3 (If Needed):**
If fallback proves problematic, consider Option C (separate capability) to give rule authors fine-grained control.

---

## Implementation Order

1. **#5 - Remove Public test methods** (Cleanest, lowest risk)
   - Update test file package if needed
   - Remove wrapper methods
   - Update test calls
   - Run tests to verify

2. **#4 - Line separator handling** (Simple documentation)
   - Add `\r\n` to `\n` normalization
   - Add comments explaining line ending handling
   - Update switch statement

3. **#1 - Handle * imports** (Extends existing regex)
   - Update regex pattern
   - Add namespace import handling to matching logic
   - Add test cases

4. **#2 & #3 - Refactor normalize with helpers** (Larger refactor, do together)
   - Create helper methods for string handling
   - Create `hasCompletedImportStatement` helper
   - Create `normalizeImportStatement` helper
   - Refactor main function to use helpers
   - Run tests to verify behavior unchanged

5. **#6 - Document future plan** (Documentation only)
   - Add "Known Limitations" to docstring
   - Add TODO comments
   - Update PR description if needed

---

## Testing Strategy

For each change:
- Run existing unit tests in `import_search_test.go`
- Add new test cases for namespace imports
- Verify behavior on real codebase (tackle2-ui)
- Check that PatternFly detection still works
- Benchmark performance to ensure no regression

---

## Success Criteria

- [ ] All existing tests pass
- [ ] New tests added for namespace imports (`import * as X`)
- [ ] Code is more maintainable (helpers extracted)
- [ ] Line ending handling is explicit and documented
- [ ] No public test-only methods
- [ ] Future limitations documented
- [ ] No performance regression
- [ ] Review feedback addressed
