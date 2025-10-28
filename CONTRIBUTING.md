# Contributing to Konveyor Analyzer LSP

Welcome! This guide will help you contribute to the Konveyor Analyzer LSP project.

## Table of Contents

- [Development Environment Setup](#development-environment-setup)
- [Building the Project](#building-the-project)
- [Testing Your Changes](#testing-your-changes)
- [Container-Based Development](#container-based-development)
- [Adding New Rules](#adding-new-rules)
- [Adding New Language Support](#adding-new-language-support)
- [Common Issues and Solutions](#common-issues-and-solutions)
- [Pull Request Process](#pull-request-process)

## Development Environment Setup

### Prerequisites

- **Go 1.23+** - For building the analyzer and Go provider
- **Java 17+** - For Java provider (JDTLS)
- **Node.js 18+** - For Node.js/TypeScript provider
- **Python 3.9+** - For Python provider
- **Podman or Docker** - For container-based testing
- **Make** - For build automation

### Clone the Repository

```bash
git clone https://github.com/konveyor/analyzer-lsp.git
cd analyzer-lsp
```

### Install Dependencies

The project uses multiple language servers. Install them based on which providers you're working with:

**TypeScript/JavaScript Provider:**
```bash
npm install -g typescript typescript-language-server
```

**Python Provider:**
```bash
python3 -m pip install 'python-lsp-server>=1.8.2'
```

**Go Provider:**
```bash
go install golang.org/x/tools/gopls@latest
```

**Java Provider:**
Java provider uses Eclipse JDTLS which is bundled in the container image.

## Building the Project

### Local Build

Build the main analyzer binary:

```bash
go build -o analyzer-lsp main.go
```

### Building External Providers

External providers (Java, Go, Python, Node.js, YAML) are built using Make:

```bash
make build-external
```

This builds:
- `dotnet-provider`
- `golang-dependency-provider`
- `generic-external-provider` (handles Go, Python, Node.js)
- `java-external-provider`
- `yq-external-provider` (YAML)

**Note for macOS users:** The `make build-external` target uses GNU sed syntax. If you encounter sed errors, you may need to manually run the sed commands with macOS syntax:

```bash
sed -i '' 's/old/new/g' file
```

### Building Container Images

Build the main analyzer container:

```bash
podman build -t quay.io/konveyor/analyzer-lsp:latest -f Dockerfile .
```

Build external provider containers:

```bash
# Builds all external providers
make build-external
```

## Testing Your Changes

### Running Tests Locally

Run the Go test suite:

```bash
go test ./...
```

### Testing with Example Projects

The `examples/` directory contains test projects for each language:

- `examples/java/` - Java test projects
- `examples/golang/` - Go test projects
- `examples/nodejs/` - Node.js/TypeScript test projects
- `examples/python/` - Python test projects
- `examples/yaml/` - YAML/Kubernetes manifests

### Container-Based Development

Container-based testing is the **recommended approach** for comprehensive testing with all providers.

#### Why Use Containers?

1. **Consistent Environment** - Same environment as CI/CD
2. **All Providers Together** - Test interactions between providers
3. **Resource Isolation** - Prevents provider memory/CPU conflicts
4. **Reproducible** - Matches production deployment

#### Container Testing Workflow

This is the workflow used for regenerating `demo-output.yaml`:

```bash
# 1. Build external providers with your changes
make build-external

# 2. Build analyzer-lsp container image
podman build -t quay.io/konveyor/analyzer-lsp:latest -f Dockerfile .

# 3. Run external providers pod
make run-external-providers-pod

# 4. Build demo container image
podman build -f demo-local.Dockerfile -t localhost/testing:latest .

# 5. Run demo image to generate output
make run-demo-image
```

#### Provider Pod Architecture

The `run-external-providers-pod` target creates a pod named `analyzer` with 6 containers:

- `java-provider` - Port 14650 (Eclipse JDTLS)
- `generic-provider` - Port 14651 (Go/Python/Node.js)
- `dotnet-provider` - Port 14652 (.NET)
- `yq-provider` - Port 14653 (YAML)
- `golang-dep-provider` - Port 14654 (Go dependencies)
- `java-dep-provider` - Port 14655 (Java dependencies)

All containers share the `test-data` volume for accessing example projects.

**Note:** The `test-data` volume is populated by copying from the `examples/` directories that are built into the analyzer-lsp and provider images.

#### Resource Requirements

**Minimum Resources for All Providers:**
- **RAM**: 12GB (8GB causes Java provider OOM)
- **CPU**: 4 cores
- **Disk**: 20GB

Check your podman machine resources:

```bash
podman machine info
```

Increase memory if needed:

```bash
podman machine stop
podman machine set --memory 12288  # 12GB
podman machine start
```

#### Cleaning Up Containers

If you need to restart the provider pod:

```bash
# Clean up everything (pod, containers, and volume)
make stop-external-providers-pod

# Recreate pod
make run-external-providers-pod
```

### Running Analysis Locally

Create a provider settings file. See existing examples:
- [`provider_local_settings.json`](provider_local_settings.json) - Simple local development setup
- [`provider_container_settings.json`](provider_container_settings.json) - Container-based setup with all providers
- [`provider_pod_local_settings.json`](provider_pod_local_settings.json) - Pod-based local setup

Example provider settings file for Node.js:

```json
[
  {
    "name": "nodejs",
    "binaryPath": "./external-providers/generic-external-provider/generic-external-provider",
    "initConfig": [{
      "analysisMode": "full",
      "providerSpecificConfig": {
        "lspServerName": "nodejs",
        "lspServerPath": "typescript-language-server",
        "lspServerArgs": ["--stdio"],
        "workspaceFolders": ["file:///path/to/your/project"]
      }
    }]
  },
  {
    "name": "builtin",
    "initConfig": [
      {"location": "path/to/your/project/"}
    ]
  }
]
```

Run analysis:

```bash
./analyzer-lsp \
  --provider-settings=provider_settings.json \
  --rules=rule-example.yaml \
  --output-file=output.yaml \
  --verbose=1
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

**Example: Detecting React Component Migration**

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

### Steps to Add a New Provider

1. **Create Provider Binary** - Implement LSP wrapper for the language server
2. **Add Dockerfile** - Create container image for the provider
3. **Update Makefile** - Add build targets
4. **Add Example Project** - Create test project in `examples/`
5. **Add Test Rules** - Create rules in `rule-example.yaml`
6. **Update Demo Output** - Regenerate `demo-output.yaml` with all providers
7. **Documentation** - Update README and this guide

### Example: Adding File Extension Support

When adding support for new file extensions to an existing provider (e.g., adding `.tsx` to the Node.js provider):

**Typical files to modify:**
1. **Provider configuration** - Add file extension to provider's supported types
   - Example: `external-providers/generic-external-provider/pkg/server_config/providers.go`
2. **Test files** - Add example files using the new extension
   - Example: `examples/nodejs/NewFileType.tsx`
3. **Test rules** - Add rules that validate the new file type works
   - Add to `rule-example.yaml`
4. **Demo output** - Regenerate to include violations from new file types
   - Run container workflow to regenerate `demo-output.yaml`

**Example test rules:**
```yaml
# Test that builtin provider scans new file type
- ruleID: test-new-filetype-00000
  description: Test that new file extension is scanned
  when:
    builtin.filecontent:
      pattern: "import.*LibraryName"
      filePattern: "\\.newext$"

# Test that semantic provider finds references in new file type
- ruleID: test-new-filetype-00010
  description: Test that provider can find references in new file type
  when:
    provider.referenced:
      pattern: "LibraryName"
```

## Common Issues and Solutions

### Java Provider OOM (Exit 137)

**Problem:** Java provider exits with code 137 (killed by OOM)

**Symptoms:**
- Java violations appear in errors section instead of violations section
- Error: "connection to the language server is closed"

**Solution:** Increase podman machine memory to at least 12GB:

```bash
podman machine stop
podman machine set --memory 12288
podman machine start
```

### macOS sed Syntax Errors

**Problem:** `make build-external` fails with sed errors on macOS

**Error:**
```text
sed: 1: "external-providers/gene ...": invalid command code e
```

**Solution:** macOS sed requires different syntax. Use empty string after `-i`:

```bash
sed -i '' 's/old/new/g' file
```

### Provider Connection Refused Errors

**Problem:** Error: "dial tcp [::1]:14651: connect: connection refused"

**Cause:** Provider container hasn't started or crashed

**Solution:**
1. Check provider status: `podman ps -a`
2. Check provider logs: `podman logs java-provider`
3. Restart provider pod: `podman pod rm -f analyzer && make run-external-providers-pod`

### Volume/Pod Already Exists Errors

**Problem:**
```text
Error: volume with name test-data already exists
Error: pod analyzer already exists
```

**Solution:** Force remove and recreate:

```bash
podman pod rm -f analyzer
podman volume rm test-data
make run-external-providers-pod
```

### Missing node_modules in Container

**Problem:** TypeScript language server can't find dependencies

**Solution:** Ensure the demo Dockerfile copies package.json and runs npm install:

```dockerfile
COPY examples/nodejs/package.json examples/nodejs/
RUN cd examples/nodejs && npm install
```

### Rule Pattern Not Matching

**Problem:** Rule doesn't find expected violations

**Debugging Steps:**

1. **Verify file extension is supported:**
   - Check provider's file extensions in `providers.go`
   - For builtin provider, check `filePattern` regex

2. **Test pattern with grep:**
   ```bash
   grep -r "your-pattern" examples/nodejs/
   ```

3. **Check provider logs:**
   ```bash
   podman logs generic-provider
   ```

4. **Verify provider is running:**
   ```bash
   podman ps | grep provider
   ```

5. **Try simpler pattern first:**
   ```yaml
   # Start simple
   pattern: "React"

   # Then make more specific
   pattern: "import.*React"
   ```

## Pull Request Process

### Before Submitting

1. **Build all providers:** `make build-external`
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
make build-external
podman build -t quay.io/konveyor/analyzer-lsp:latest -f Dockerfile .

# 2. Run providers
make run-external-providers-pod

# 3. Build and run demo
podman build -f demo-local.Dockerfile -t localhost/testing:latest .
make run-demo-image

# 4. Verify output
cat demo-output.yaml

# 5. Commit changes
git add demo-output.yaml demo-dep-output.yaml
git commit -s -m "Regenerate demo output with [your changes]"
```

### PR Checklist

- [ ] Code builds successfully (`make build-external`)
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
- **Documentation:**
  - [README](README.md) - Quick start guide
  - [Rules Documentation](docs/rules.md) - Detailed rule syntax
  - [Providers Documentation](docs/providers.md) - Provider configuration
  - [Labels Documentation](docs/labels.md) - Label selectors

## Code of Conduct

This project follows the [Konveyor Code of Conduct](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
