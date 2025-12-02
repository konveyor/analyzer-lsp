# Chain Template Enhancements for PatternFly Combo Rules

## Overview

This document describes how to enhance PatternFly migration rules to use **chain templates** with filepath filtering for improved performance and accuracy.

## What Are Chain Templates?

Chain templates allow rules to pass context (especially filepaths) between conditions in an `and:` block. This enables:
1. **More efficient searches** - Only check files that match previous conditions
2. **Better accuracy** - Ensure all conditions check the same files
3. **Reduced false positives** - Tighter coupling between import detection and usage detection

## Syntax

```yaml
when:
  and:
  - as: <variableName>           # Store filepaths from this condition
    <condition>
  - from: <variableName>         # Reference the stored context
    <condition>:
      filepaths: '{{<variableName>.filepaths}}'  # Only search these files
```

## Example Enhancement

### Original Combo Rule (without chain templates)

```yaml
- ruleID: patternfly-v5-to-patternfly-v6-toolbar-00000
  when:
    and:
    - nodejs.referenced:
        pattern: ToolbarGroup
    - builtin.filecontent:
        pattern: <ToolbarGroup[^>]*\balign[^>]*alignLeft\b
        filePattern: \.(j|t)sx?$
```

**Problems:**
- `nodejs.referenced` only checks if ToolbarGroup exists in node_modules
- `builtin.filecontent` searches ALL .tsx/.jsx files (inefficient)
- No guarantee both conditions check the same files

### Enhanced Rule (with chain templates)

```yaml
- ruleID: patternfly-v5-to-patternfly-v6-toolbar-00000
  when:
    and:
    - as: toolbarGroupFiles
      builtin.filecontent:
        pattern: import.*\{[^}]*\bToolbarGroup\b[^}]*\}.*from ['"]@patternfly/react-core['"]
        filePattern: \.(j|t)sx?$
    - from: toolbarGroupFiles
      builtin.filecontent:
        pattern: <ToolbarGroup[^>]*\balign[^>]*alignLeft\b
        filepaths: '{{toolbarGroupFiles.filepaths}}'
```

**Benefits:**
- First condition finds files that import ToolbarGroup
- Second condition ONLY searches those files (faster, more accurate)
- Guaranteed to find actual usage in files that import the component

## Enhanced Toolbar Rules

The following rules have been enhanced in `patternfly-v5-to-patternfly-v6-toolbar-enhanced.yaml`:

### 1. ToolbarGroup align="alignLeft" → align="alignStart"
- **Rule ID:** patternfly-v5-to-patternfly-v6-toolbar-00000
- **Chain variable:** `toolbarGroupFiles`

### 2. ToolbarItem align="alignLeft" → align="alignStart"
- **Rule ID:** patternfly-v5-to-patternfly-v6-toolbar-00010
- **Chain variable:** `toolbarItemFiles`

### 3. ToolbarToggleGroup alignment="alignLeft" → alignment="alignStart"
- **Rule ID:** patternfly-v5-to-patternfly-v6-toolbar-00020
- **Chain variable:** `toolbarToggleFiles`

## How Chain Templates Work (Implementation)

### 1. Template Storage (engine/conditions.go)
When a condition has `as:`, filepaths are stored:
```go
if c.As != "" {
    condCtx.Template[c.As] = ChainTemplate{
        Filepaths: incidentsToFilepaths(response.Incidents),
        Extras:    response.TemplateContext,
    }
}
```

### 2. Template Rendering (provider/provider.go)
The `{{name.filepaths}}` syntax is rendered using mustache:
```go
func templateCondition(condition []byte, ctx map[string]engine.ChainTemplate) ([]byte, error) {
    // Convert context to format mustache can render
    yamlCtx := make(map[string]interface{})
    for key, template := range ctx {
        if len(template.Filepaths) > 0 {
            yamlCtx[key] = map[string]interface{}{
                "filepaths": template.Filepaths,
            }
        }
    }
    // Render template
    s, err := mustache.RenderRaw(s, true, yamlCtx)
    return []byte(s), nil
}
```

### 3. Filepath Filtering (provider/internal/builtin/service_client.go)
The builtin provider uses the filepaths:
```go
filePaths, err := fileSearcher.Search(provider.SearchCriteria{
    Patterns:           patterns,
    ConditionFilepaths: c.Filepaths,  // Only search these files!
})
```

## Testing

Test the enhanced rules:
```bash
go run ./cmd/analyzer \
  --provider-settings /tmp/test-provider-settings-builtin.json \
  --rules patternfly-v5-to-patternfly-v6-toolbar-enhanced.yaml \
  --output-file /tmp/test-output.yaml
```

## When to Use Chain Templates

**Use chain templates when:**
✅ You have import detection + usage detection pattern
✅ The first condition finds a subset of files
✅ Subsequent conditions should only check those files
✅ You want to guarantee consistency between conditions

**Don't use chain templates when:**
❌ Conditions are independent (should check different files)
❌ First condition matches almost all files (no performance benefit)
❌ Simple single-condition rules

## Performance Impact

For a codebase with:
- 1,000 total .tsx/.jsx files
- 10 files importing ToolbarGroup

**Without chain templates:**
- Condition 1: Check dependencies (fast)
- Condition 2: Search 1,000 files
- **Total:** ~1,000 file searches

**With chain templates:**
- Condition 1: Search 1,000 files for imports
- Condition 2: Search 10 files for usage
- **Total:** ~1,010 file searches, but condition 2 is 100x faster

**Net result:** Faster overall execution, especially for rare components.

## Next Steps

To enhance all combo-final rules:
1. Identify rules with import detection patterns
2. Convert first condition to find import statements
3. Add `as:` clause to first condition
4. Add `from:` and `filepaths:` to second condition
5. Test to ensure no regressions

## Related Files

- **Enhanced rules:** `patternfly-v5-to-patternfly-v6-toolbar-enhanced.yaml`
- **Original rules:** `/Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/combo-final/`
- **Test rule:** `test-filepath-chain.yaml`
- **Implementation:**
  - `engine/conditions.go` (ChainTemplate storage)
  - `provider/provider.go` (template rendering)
  - `provider/internal/builtin/service_client.go` (filepath filtering)
