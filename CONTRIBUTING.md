# Contributing to Konveyor Analyzer LSP

Welcome! This guide will help you contribute to the Konveyor Analyzer LSP project.

## Developer Documentation

**New to the project?** Start with our comprehensive developer documentation:

- **[Development Documentation Hub](docs/development/README.md)** - Start here
- **[Architecture Overview](docs/development/architecture.md)** - Understand the codebase
- **[Development Setup](docs/development/setup.md)** - Set up your environment
- **[Testing Guide](docs/development/testing.md)** - Run and write tests
- **[Provider Development](docs/development/provider_development.md)** - Create new providers

## Table of Contents

- [Quick Start](#quick-start)
- [Development Workflow](#development-workflow)
- [Adding New Rules](#adding-new-rules)
- [Adding New Language Support](#adding-new-language-support)
- [Pull Request Process](#pull-request-process)
- [Common Issues](#common-issues)
- [Getting Help](#getting-help)

## Quick Start

**Prerequisites:** Go 1.23+, Podman/Docker, Make

For detailed setup instructions, see **[Development Setup](docs/development/setup.md)**.

```bash
# 1. Clone repository
git clone https://github.com/konveyor/analyzer-lsp.git
cd analyzer-lsp

# 2. Build the project
make build

# 3. Run tests
go test ./...
make test-all

# 4. Make changes and test
# ... edit code ...
go test ./...
make test-all
```

## Development Workflow

### 1. Set Up Your Environment

See **[Development Setup](docs/development/setup.md)** for:
- Installing LSP servers (gopls, pylsp, typescript-language-server)
- IDE configuration (VS Code, GoLand, Vim)
- Debugging setup

### 2. Understand the Architecture

Read **[Architecture Overview](docs/development/architecture.md)** to learn about:
- Package organization and dependencies
- Provider system design
- Data flow through the analyzer

### 3. Make Your Changes

```bash
# Create feature branch
git checkout -b feature/my-feature

# Make changes
# ... edit code ...

# Build
make build

# Test
go test ./...
make test-all
```

### 4. Test Thoroughly

See **[Testing Guide](docs/development/testing.md)** for:
- Running unit tests
- Running E2E tests
- Understanding how `make test-all` works
- Debugging test failures

**Quick test commands:**
```bash
# Unit tests
go test ./...

# All E2E tests
make test-all

# Specific provider tests
make test-java
make test-generic
make test-yaml
```

## Adding New Rules

Rules are defined in YAML files (e.g., `rule-example.yaml`).

**For comprehensive documentation on rule syntax, provider capabilities, and advanced features, see [docs/rules.md](docs/rules.md).**

### Rule Structure

```yaml
- ruleID: unique-rule-id-00000
  description: Brief description of what this rule detects
  effort: 5  # Estimated effort to fix (1-10)
  category: mandatory  # mandatory, potential, or optional
  labels:
    - konveyor.io/source=source-technology
    - konveyor.io/target=target-technology
  when:
    # Provider-specific condition (see below)
  message: |
    Detailed message explaining the issue and how to fix it.

    Before:
    ```
    old code example
    ```

    After:
    ```
    new code example
    ```
  links:
    - url: https://docs.example.com/migration
      title: Migration Documentation
```

### Provider Types and When to Use Each

#### builtin Provider (Text/Regex Matching)

**Use for:**
- File content patterns
- Comments or documentation
- CSS/HTML patterns
- Configuration files
- When you need file filtering (`filePattern`)

**Example:**
```yaml
when:
  builtin.filecontent:
    pattern: "oldFunction\\s*\\("
    filePattern: "\\.tsx?$"  # Regex pattern for .ts and .tsx files
```

**File Pattern Examples:**
```yaml
filePattern: "\\.tsx?$"           # .ts and .tsx files
filePattern: "\\.(js|ts)x?$"      # .js, .jsx, .ts, .tsx files
filePattern: "\\.(css|scss)$"     # .css and .scss files
filePattern: "\\.ya?ml$"          # .yaml and .yml files
```

#### nodejs Provider (TypeScript/JavaScript Semantic Analysis)

**Use for:**
- Function/class/variable references
- Import statements
- JSX component usage
- Semantic code analysis

**Cannot find:**
- Class methods (use builtin)
- Object properties (use builtin)
- Type annotations (use builtin)
- JSX props (use builtin)

**Example:**
```yaml
when:
  nodejs.referenced:
    pattern: "OldComponent"  # Finds actual symbol references
```

**Important:** nodejs provider does NOT support `filePattern`. It automatically searches all TypeScript/JavaScript files.

#### java Provider (Java Semantic Analysis)

**Use for:**
- Java class/method/field references
- Annotations
- Import statements
- Package declarations

**Example:**
```yaml
when:
  java.referenced:
    pattern: "org.example.OldClass"
```

#### Other Providers

- **go.referenced** - Go symbol references
- **python.referenced** - Python symbol references
- **builtin.xml** - XML element/attribute matching
- **builtin.json** - JSON key/value matching
- **builtin.hasTags** - Check for specific tags

### Combining Providers for Complete Coverage

#### Best Practice: Provider Combination Strategy

Use semantic providers (nodejs, java, go, python) for symbol references, and builtin provider for patterns they cannot detect.

##### Example: Detecting React Component Migration

```yaml
# nodejs provider - finds component imports and usage
- ruleID: old-button-component-00000
  when:
    nodejs.referenced:
      pattern: "OldButton"
  message: |
    OldButton component is deprecated.
    Replace with NewButton.

# builtin provider - finds prop usage (nodejs can't detect this)
- ruleID: old-button-variant-prop-00001
  when:
    builtin.filecontent:
      pattern: '<OldButton\s+variant="danger"'
      filePattern: "\\.tsx?$"
  message: |
    The "danger" variant has been renamed to "destructive".
```

## Adding New Language Support

For comprehensive guidance on creating providers, see **[Provider Development Guide](docs/development/provider_development.md)**.

### Quick Start: Add a Language Using Generic Provider

If your language has an LSP server, you can add support quickly:

```json
{
  "name": "rust",
  "binaryPath": "/path/to/generic-external-provider",
  "initConfig": [{
    "location": "/path/to/rust/project",
    "providerSpecificConfig": {
      "lspServerName": "generic",
      "lspServerPath": "rust-analyzer",
      "workspaceFolders": ["file:///path/to/rust/project"]
    }
  }]
}
```

### Creating a Custom Provider

For full control or specialized analysis:

1. **Implement provider interfaces** - See [Provider Development Guide](docs/development/provider_development.md#implementing-provider-interfaces)
2. **Add capabilities** - Define what your provider can analyze
3. **Write tests** - Add E2E tests in `external-providers/your-provider/e2e-tests/`
4. **Update build** - Add Makefile targets and Dockerfile
5. **Document** - Update provider documentation

## Common Issues

For detailed troubleshooting, see:
- **[Development Setup - Troubleshooting](docs/development/setup.md#troubleshooting)**
- **[Testing Guide - Debugging Failed Tests](docs/development/testing.md#debugging-failed-tests)**

### Quick Fixes

**Java Provider OOM:**
```bash
podman machine stop
podman machine set --memory 12288
podman machine start
```

**Pod/Volume Already Exists:**
```bash
podman pod rm -f analyzer
podman volume rm test-data
make run-external-providers-pod
```

**Build Failures on macOS:**
See [Development Setup - Troubleshooting](docs/development/setup.md#build-failures)

**Provider Not Starting:**
```bash
podman ps -a                    # Check status
podman logs <provider-name>     # Check logs
```

## Pull Request Process

### Before Submitting

1. **Build all components:** `make build`
2. **Run tests:** `go test ./...`
3. **Test with containers:** Follow container-based development workflow
4. **Regenerate demo output:** If you added/changed rules or provider behavior
5. **Sign commits:** Use `git commit -s` for Developer Certificate of Origin
6. **Follow commit conventions:** Use emoji prefixes for commits and PR titles

### Commit and PR Title Conventions

This project follows **conventional commits with emoji prefixes**. PR titles must use one of these prefixes:

**Required PR title prefixes:**
- ⚠️ `:warning:` - Breaking change
- ✨ `:sparkles:` - Non-breaking feature
- 🐛 `:bug:` - Bug fix
- 📖 `:book:` - Documentation
- 🌱 `:seedling:` - Infrastructure/Tests/Other
- 👻 `:ghost:` - No release note required

**Examples:**
```bash
# Commit messages (use text codes like :sparkles:)
git commit -s -m ":sparkles: Add TypeScript/React support to nodejs provider"
git commit -s -m ":bug: Fix Java provider OOM with large projects"
git commit -s -m ":book: Add comprehensive contributor guide"

# PR titles (must use text codes, not emoji characters)
:sparkles: Add TypeScript/React support to nodejs provider
:book: Add comprehensive contributor guide
:bug: Fix Java provider OOM issues with large projects
:seedling: Update CI workflow configuration
```

**Important:** PR titles must use the **text code** (`:sparkles:`) not the emoji character (✨), or the CI check will fail.

### Regenerating Demo Output

**When to regenerate:**
- Added new rules to `rule-example.yaml`
- Modified provider behavior
- Added new example files
- Changed file extension support

**How to regenerate:**

```bash
# 1. Build everything
make build
podman build -t quay.io/konveyor/analyzer-lsp:latest -f Dockerfile .

# 2. Run providers
make run-external-providers-pod

# 3. Build and run demo
make image-build
make run-demo-image

# 4. Verify output
cat demo-output.yaml

# 5. Commit changes
git add demo-output.yaml demo-dep-output.yaml
git commit -s -m "Regenerate demo output with [your changes]"
```

### PR Checklist

- [ ] Code builds successfully (`make build`)
- [ ] Tests pass (`go test ./...`)
- [ ] Added test rules for new functionality
- [ ] Regenerated `demo-output.yaml` if needed
- [ ] Updated documentation (README, this guide)
- [ ] Signed commits (`git commit -s`)
- [ ] PR description explains what/why/how

### CI Tests

Your PR will run these tests:

1. **Build Test** - Verifies all providers build
2. **Unit Tests** - Runs `go test ./...`
3. **Demo Output Test** - Compares `demo-output.yaml` and `demo-dep-output.yaml` against expected output
4. **Linting** - Checks Go code style

The "ensure violation and dependency outputs are unchanged" test will fail if you changed provider behavior or rules but didn't regenerate the demo output files.

## Getting Help

- **GitHub Issues:** [https://github.com/konveyor/analyzer-lsp/issues](https://github.com/konveyor/analyzer-lsp/issues)
- **Konveyor Slack:** [https://kubernetes.slack.com/archives/CR85S82A2](https://kubernetes.slack.com/archives/CR85S82A2)

**Documentation:**
- **Developer Guides:**
  - [Development Documentation Hub](docs/development/README.md) - Start here
  - [Architecture Overview](docs/development/architecture.md) - Codebase structure
  - [Development Setup](docs/development/setup.md) - Environment setup
  - [Testing Guide](docs/development/testing.md) - Testing instructions
  - [Provider Development](docs/development/provider_development.md) - Creating providers

- **User Documentation:**
  - [README](README.md) - Quick start guide
  - [Rules Documentation](docs/rules.md) - Detailed rule syntax
  - [Providers Documentation](docs/providers.md) - Provider configuration
  - [Labels Documentation](docs/labels.md) - Label selectors

## Code of Conduct

This project follows the [Konveyor Code of Conduct](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
