# Provider E2E Testing

This directory contains end-to-end tests for each external provider. Each provider has its own `e2e-tests` directory with provider-specific test configurations.

## Directory Structure

```
external-providers/
├── java-external-provider/
│   └── e2e-tests/
│       ├── rule-example.yaml          # Java-specific test rules
│       ├── demo-output.yaml           # Expected output for Java tests
│       └── provider_settings.json     # Java provider configuration
├── golang-dependency-provider/
│   └── e2e-tests/
│       ├── rule-example.yaml          # Go-specific test rules
│       ├── demo-output.yaml           # Expected output for Go tests
│       └── provider_settings.json     # Go provider configuration
├── generic-external-provider/
│   └── e2e-tests/
│       ├── rule-example.yaml          # Python/Node.js-specific test rules
│       ├── demo-output.yaml           # Expected output for Python/Node.js tests
│       └── provider_settings.json     # Python/Node.js provider configuration
└── yq-external-provider/
    └── e2e-tests/
        ├── rule-example.yaml          # YAML provider-specific test rules
        ├── demo-output.yaml           # Expected output for YAML tests
        └── provider_settings.json     # YAML provider configuration
```

## Test Files

Each provider's `e2e-tests` directory contains three files:

1. **rule-example.yaml**: Test rules that use only that provider's capabilities
2. **demo-output.yaml**: Expected analysis output for the provider's test rules
3. **provider_settings.json**: Provider configuration including only the tested provider and builtin

## Running Tests

### Prerequisites

1. Build the provider images:
   ```bash
   make build-external
   ```

### Test Individual Providers

#### Java Provider
```bash
# Start Java provider pod
make run-java-provider-pod

# Run Java tests
make run-demo-java

# Stop Java provider pod
make stop-java-provider-pod
```

#### Go Provider
```bash
# Start Go provider pod
make run-go-provider-pod

# Run Go tests
make run-demo-go

# Stop Go provider pod
make stop-go-provider-pod
```

#### Generic Provider (Python/Node.js)
```bash
# Start Generic provider pod
make run-generic-provider-pod

# Run Generic tests
make run-demo-generic

# Stop Generic provider pod
make stop-generic-provider-pod
```

#### YAML Provider
```bash
# Start YAML provider pod
make run-yaml-provider-pod

# Run YAML tests
make run-demo-yaml

# Stop YAML provider pod
make stop-yaml-provider-pod
```

### Test All Providers (Original Behavior)

To test all providers together using the combined configuration:

```bash
# Start all provider pods
make run-external-providers-pod

# Run all tests
make run-demo-image

# Stop all provider pods
make stop-external-providers-pod
```

## Test Configuration Files

### rule-example.yaml

Contains rules specific to each provider:

- **Java**: Rules using `java.referenced`, `java.dependency`, annotation inspection
- **Go**: Rules using `go.referenced`, `go.dependency`
- **Generic**: Rules using `python.referenced`, `nodejs.referenced`
- **YAML**: Rules using `yaml.k8sResourceMatched`

### demo-output.yaml

Contains the expected violations for each provider's test rules. This file is used to validate that the analyzer produces correct output.

### provider_settings.json

Contains the provider initialization configuration:
- Provider name and address
- LSP server configuration
- Workspace folders and dependency folders
- Analysis mode (full, source-only, etc.)
- Builtin provider configuration for file-based rules

## Root Test Files

The root directory still contains combined test files for testing all providers together:

- `rule-example.yaml`: Multi-provider and builtin-only rules
- `demo-output.yaml`: Combined expected output for all providers
- `provider_pod_local_settings.json`: Configuration for all providers

## Adding New Tests

To add tests for a specific provider:

1. Add the rule to the provider's `e2e-tests/rule-example.yaml`
2. Add the expected output to the provider's `e2e-tests/demo-output.yaml`
3. Update `provider_settings.json` if new configuration is needed
4. Run the provider-specific test target to validate

## Troubleshooting

### Pod Already Exists
If you get an error about a pod already existing:
```bash
make stop-<provider>-provider-pod
```

### Volume Already Exists
If you get an error about test-data volume:
```bash
podman volume rm test-data
```

### Provider Not Starting
Check provider logs:
```bash
podman logs <provider-name>
```

For example:
```bash
podman logs java-provider
```
