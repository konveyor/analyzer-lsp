---
- message: this rule is in a markdown file and should be skipped
  ruleID: md-rule-001
  when:
    builtin.file: "*.md"
