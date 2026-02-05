# MCP Test Data

This directory contains test fixtures for the MCP server tests.

## Structure

```
testdata/
├── rules/
│   ├── test_rules.yaml       # Valid test rules for parsing and validation
│   └── invalid_rules.yaml    # Invalid rules for testing validation errors
├── results/
│   └── sample_output.yaml    # Sample analysis output for incident testing
├── target/
│   └── Sample.java            # Sample Java file for analysis
├── provider_settings.json     # Test provider configuration
└── README.md                  # This file
```

## Files

### rules/test_rules.yaml
Contains two valid test rules:
- `test-001`: Rule for finding Java files (mandatory, effort 1)
- `test-002`: Rule for finding XML files (optional, effort 3)

Used for testing:
- Rule listing
- Rule validation (valid case)
- Analysis execution
- Label filtering

### rules/invalid_rules.yaml
Contains rules with missing required fields (no ruleID).

Used for testing:
- Rule validation (invalid case)
- Error handling

### results/sample_output.yaml
Sample analysis output in konveyor format with:
- Two violations (test-001, test-002)
- Sample incidents with code snippets
- Labels and metadata

Used for testing:
- Incident querying
- Filtering by rule ID
- Limiting results
- Output parsing (YAML and JSON)

### target/Sample.java
Minimal Java source file for analysis.

Used for testing:
- Analysis execution
- Provider initialization
- File pattern matching

### provider_settings.json
Minimal provider configuration with builtin provider only.

Used for testing:
- Provider initialization
- Provider listing
- Analysis execution
- Dependency extraction

## Test Coverage

The test fixtures support testing:
- ✅ Input validation (missing/invalid parameters)
- ✅ File path validation (non-existent files)
- ✅ Rule parsing and validation
- ✅ Output format handling (JSON/YAML)
- ✅ Label filtering
- ✅ Provider listing and capabilities
- ✅ Incident querying and filtering
- ✅ Error handling and edge cases
