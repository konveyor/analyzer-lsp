# VHS tape file for recording the progress reporting demo
# Install VHS: https://github.com/charmbracelet/vhs
# Run: vhs demo.tape

Output demo-progress.mp4

Set FontSize 14
Set Width 1400
Set Height 700
Set Theme "Catppuccin Mocha"

Type "# Analyzer Progress Reporting Demo" Sleep 500ms Enter
Sleep 1s

Type "# Running live progress demo..." Sleep 500ms Enter
Sleep 500ms

Type "/tmp/progress-demo" Sleep 500ms Enter

# Wait for demo to complete (it takes about 6 seconds)
Sleep 8s

# Show the final state
Sleep 2s

Type ""
Sleep 1s

Type "# Now let's see it with real rules and providers!" Sleep 500ms Enter
Sleep 1s

Type "# Example command:" Sleep 300ms Enter
Type "# analyzer --rules patternfly-v5-to-v6.yaml \\" Sleep 300ms Enter
Type "#   --provider-settings settings.json \\" Sleep 300ms Enter
Type "#   --progress-output=stderr --progress-format=text" Sleep 500ms Enter
Sleep 500ms
Type "" Sleep 100ms Enter
Type "# Output:" Sleep 300ms Enter
Type "# [17:06:14] Provider: Initializing nodejs provider" Sleep 100ms Enter
Type "# [17:06:17] Provider: Provider nodejs ready (3.0s)" Sleep 100ms Enter
Type "# [17:06:17] Provider: Initializing builtin provider" Sleep 100ms Enter
Type "# [17:06:17] Provider: Provider builtin ready" Sleep 100ms Enter
Type "# [17:06:22] Rule: Starting rule execution: 10 rules" Sleep 100ms Enter
Type "# [17:06:22] Rule: patternfly-v5-to-v6-charts-00000" Sleep 100ms Enter
Type "# [17:06:22] Rule: patternfly-v5-to-v6-css-variables-00000" Sleep 100ms Enter
Type "# ..." Sleep 100ms Enter
Type "# [17:06:26] Analysis complete!" Sleep 300ms Enter

Sleep 2s

Type ""
Sleep 1s

Type "# Progress reporting features:" Sleep 500ms Enter
Type "# ✓ Provider initialization tracking (NEW!)" Sleep 300ms Enter
Type "# ✓ Real-time progress bar" Sleep 300ms Enter
Type "# ✓ Rule-by-rule tracking" Sleep 300ms Enter
Type "# ✓ Multiple output formats (JSON, Text, Channel)" Sleep 300ms Enter
Type "# ✓ Zero overhead when disabled" Sleep 300ms Enter

Sleep 3s
