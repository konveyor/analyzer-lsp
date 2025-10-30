# Progress Reporting - Quick Start

Get up and running with progress reporting in 5 minutes.

## Enable Progress Reporting

Add two flags to your analyzer command:

```bash
analyzer \
  --rules rules.yaml \
  --provider-settings settings.json \
  --progress-output=stderr \
  --progress-format=text
```

That's it! You'll see real-time progress updates like:

```
[15:04:05] Initializing...
[15:04:05] Provider: java-external-provider
[15:04:06] Loaded 45 rules
[15:04:07] Processing rules: 15/45 (33.3%)
[15:04:08] Processing rules: 30/45 (66.7%)
[15:04:09] Analysis complete!
```

## Common Use Cases

### 1. Terminal Progress Bar

For interactive terminal use:

```bash
analyzer \
  --rules rules.yaml \
  --progress-output=stderr \
  --progress-format=text
```

### 2. Log to File

For debugging or auditing:

```bash
analyzer \
  --rules rules.yaml \
  --progress-output=progress.json \
  --progress-format=json
```

### 3. CI/CD Pipeline

For automated builds:

```bash
analyzer \
  --rules rules.yaml \
  --progress-output=stdout \
  --progress-format=json \
  --output-file=results.yaml 2>/dev/null | \
  jq -r 'select(.stage=="rule_execution") | "\(.percent | floor)% complete"'
```

### 4. Custom UI Integration

For building dashboards or UIs:

```go
import "github.com/konveyor/analyzer-lsp/progress"

reporter := progress.NewChannelReporter()
defer reporter.Close()

eng := engine.CreateRuleEngine(ctx, 10, log,
    engine.WithProgressReporter(reporter),
)

go func() {
    for event := range reporter.Events() {
        updateUI(event.Percent, event.Current, event.Total)
    }
}()

results := eng.RunRules(ctx, ruleSets)
```

## Flags Reference

| Flag | Values | Default | Description |
|------|--------|---------|-------------|
| `--progress-output` | `stderr`, `stdout`, `<file>` | none | Where to write progress |
| `--progress-format` | `text`, `json` | `text` | Output format |

## Event Stages

Progress events track these analysis stages:

1. **init** - Analysis starting
2. **provider_init** - Initializing providers
3. **rule_parsing** - Loading rules
4. **rule_execution** - Processing rules ← *Most updates here*
5. **complete** - Analysis finished

## JSON Event Format

Each event is a JSON object on a single line:

```json
{
  "timestamp": "2024-10-29T15:04:07Z",
  "stage": "rule_execution",
  "current": 15,
  "total": 45,
  "percent": 33.3,
  "message": "java-spring-boot-001"
}
```

Parse with standard JSON tools:
- **Shell**: `jq`
- **Python**: `json.loads(line)`
- **JavaScript**: `JSON.parse(line)`
- **Go**: `json.Unmarshal([]byte(line), &event)`

## Next Steps

- [Full Documentation](./progress-reporting.md) - Complete API reference and examples
- [Demo Video](../demo-progress.mp4) - See it in action
- [Implementation Guide](./PROGRESS_REPORTING_PLAN.md) - Architecture and design

## Tips

✅ **Do**:
- Use `stderr` for progress, keep `stdout` for data
- Use JSON format for scripting and automation
- Process events in a separate goroutine

❌ **Don't**:
- Block on progress event handling
- Mix progress and normal output on stdout
- Rely on exact event timing (varies by rule complexity)

---

Need help? Check the [full documentation](./progress-reporting.md) or open an issue.
