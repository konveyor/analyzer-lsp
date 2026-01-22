# Developer Documentation

Welcome to the analyzer-lsp developer documentation! This section contains guides and references for contributors working on the analyzer codebase.

## Getting Started

New contributors should read these in order:

1. [Architecture](architecture.md) - Understand the codebase structure and package organization
2. [Development Setup](setup.md) - Set up your development environment with required tools
3. [Testing](testing.md) - Learn how to run and write tests
4. See the main [CONTRIBUTING.md](../../CONTRIBUTING.md) for code style, PR process, and contribution guidelines

## Documentation Index

### Core Guides

- **[Architecture](architecture.md)** - Package overview, architecture diagrams, and data flow
  - Top-level package descriptions
  - Exposed types and interfaces
  - System architecture and component interaction

- **[Testing](testing.md)** - Comprehensive testing guide
  - Running unit tests with `go test`
  - Understanding `make test-all` and E2E tests
  - Writing tests for new rules and providers
  - Debugging test failures

- **[Development Setup](setup.md)** - Setting up your development environment
  - Installing Go and dependencies
  - Installing LSP servers for testing
  - Setting up debugging tools
  - IDE configuration

- **[Provider Development](provider_development.md)** - Creating new language providers
  - Provider architecture and interfaces
  - Implementing a new external provider
  - Testing and debugging providers
  - Best practices

## Quick Reference

### Common Commands

```bash
# Building
make build                  # Build all components
make analyzer              # Build main analyzer binary
make build-external        # Build all external providers

# Testing
go test ./...              # Run all unit tests
make test-all              # Run complete E2E test suite
make test-java             # Test Java provider
make test-generic          # Test Go/Python/Node.js providers
make test-analyzer         # Full integration test

# Development
make run-external-providers-pod    # Start provider containers for testing
make stop-external-providers-pod   # Stop provider containers
```

### Project Structure

```text
analyzer-lsp/
├── cmd/                   # CLI entry points
├── engine/                # Rule execution engine
├── provider/              # Provider abstraction and interfaces
├── parser/                # YAML rule parser
├── lsp/                   # LSP protocol types and client
├── external-providers/    # Language-specific providers
├── output/                # Output data structures
├── examples/              # Test data and sample projects
└── docs/                  # Documentation
    └── development/       # You are here!
```

### Additional Resources

- [Main Documentation](../README.md) - User-facing documentation index
- [Rules Documentation](../rules.md) - Rule syntax and examples
- [Provider Documentation](../providers.md) - Provider configuration
- [Output Format](../output.md) - Understanding analyzer output
- [Versioning](../../VERSIONING.md) - Version policy and releases

## Getting Help

- Check existing [GitHub Issues](https://github.com/konveyor/analyzer-lsp/issues)
- Review the [CONTRIBUTING.md](../../CONTRIBUTING.md) guide
- Join the Konveyor [community](https://github.com/konveyor/community) discussions

## Contributing to Documentation

Found something missing or unclear? Documentation improvements are always welcome! Please submit a PR or open an issue.
