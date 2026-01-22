# Development Setup

This guide will help you set up your development environment for contributing to analyzer-lsp.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Initial Setup](#initial-setup)
- [Installing LSP Servers](#installing-lsp-servers)
- [Building the Project](#building-the-project)
- [IDE Configuration](#ide-configuration)
- [Debugging](#debugging)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.23+ | Build analyzer and Go-based providers |
| **Podman or Docker** | Latest | Container-based testing and external providers |
| **Make** | Any | Build automation |
| **Git** | Any | Version control |

### Optional Tools (for specific providers)

| Tool | Version | Purpose |
|------|---------|---------|
| **Java** | 17+ | Java provider development and testing |
| **Node.js** | 18+ | Node.js/TypeScript provider |
| **Python** | 3.9+ | Python provider |
| **npm** | Latest | Installing Node.js language servers |
| **pip** | Latest | Installing Python language servers |

### System Requirements

- **Disk Space:** At least 5GB for images and build artifacts
- **RAM:** 8GB minimum, 16GB recommended for running all providers
- **OS:** Linux, macOS, or Windows (with WSL2)

## Initial Setup

### 1. Clone the Repository

```bash
git clone https://github.com/konveyor/analyzer-lsp.git
cd analyzer-lsp
```

### 2. Verify Go Installation

```bash
go version
# Should show: go version go1.23.x or higher
```

If Go is not installed:

**Linux:**
```bash
wget https://go.dev/dl/go1.23.9.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.9.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

**macOS:**
```bash
brew install go
```

**Windows (WSL2):**
```bash
wget https://go.dev/dl/go1.23.9.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.9.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### 3. Verify Podman/Docker Installation

```bash
podman --version
# or
docker --version
```

If Podman is not installed:

**Linux:**
```bash
# Fedora/RHEL
sudo dnf install podman

# Ubuntu/Debian
sudo apt-get install podman
```

**macOS:**
```bash
brew install podman
podman machine init
podman machine start
```

**Windows:**
Install Docker/Podman Desktop with WSL2 backend

### 4. Verify Build Tools

```bash
make --version
```

## Installing LSP Servers

LSP servers enable language-specific analysis. Install the ones you need for development.

### Go Language Server (gopls)

Required for: Go provider

```bash
go install golang.org/x/tools/gopls@latest

# Verify installation
gopls version
```

**Configuration:** gopls is usually auto-configured. For custom settings, create `~/.config/gopls/settings.json`:

```json
{
  "gofumpt": true,
  "staticcheck": true
}
```

### Python Language Server (pylsp)

Required for: Python provider

```bash
python3 -m pip install 'python-lsp-server>=1.8.2'

# Verify installation
pylsp --version
```

**Optional plugins:**
```bash
python3 -m pip install python-lsp-server[all]
```

### TypeScript/JavaScript Language Server

Required for: Node.js/TypeScript provider

```bash
npm install -g typescript typescript-language-server

# Verify installation
typescript-language-server --version
```

### Java Language Server (JDTLS)

Required for: Java provider

**Note:** JDTLS is bundled in the Java provider container image. For local development, you can use the containerized version.

If you need JDTLS locally:

1. Download [jdtls](https://download.eclipse.org/jdtls/milestones/1.51.0/jdt-language-server-1.51.0-202510022025.tar.gz)
2. Extract to a directory (e.g., `~/jdtls`)
3. Set path in provider configuration

### YAML Tools (yq)

Required for: YAML provider

The YAML provider uses `yq` which is bundled in the container. For local testing:

```bash
# macOS
brew install yq

# Linux
sudo wget https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -O /usr/local/bin/yq
sudo chmod +x /usr/local/bin/yq
```

## Building the Project

### Quick Build (Analyzer Only)

```bash
make analyzer
```

Output: `build/konveyor-analyzer`

### Full Local Build

Build all components:

```bash
make build
```

This creates in `build/`:
- `konveyor-analyzer` - Main analyzer CLI
- `konveyor-analyzer-dep` - Dependency analyzer CLI
- `generic-external-provider` - Go/Python/Node.js provider
- `golang-dependency-provider` - Go dependency provider
- `yq-external-provider` - YAML provider
- `java-external-provider` - Java provider

### Building Container Images

Build the main analyzer image:

```bash
make image-build
```

Build all external provider images:

```bash
make build-external
```

This builds:
- `localhost/analyzer-lsp:latest`
- `localhost/java-provider:latest`
- `localhost/generic-provider:latest`
- `localhost/golang-dep-provider:latest`
- `localhost/yq-provider:latest`

### Building Individual Providers

```bash
# Generic provider (Go/Python/Node.js)
make external-generic

# Java provider
make java-external-provider

# YAML provider
make yq-external-provider

# Go dependency provider
make golang-dependency-provider
```

### Platform-Specific Builds

Cross-compile for different platforms:

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 make analyzer

# macOS ARM64
GOOS=darwin GOARCH=arm64 make analyzer

# Windows
GOOS=windows GOARCH=amd64 make analyzer
```

## IDE Configuration

### Visual Studio Code

**Workspace settings** (`.vscode/settings.json`):

```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.testFlags": ["-v"],
  "go.buildFlags": [],
  "files.watcherExclude": {
    "**/build/**": true
  }
}
```

**Debug configuration** (`.vscode/launch.json`):

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Analyzer",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/analyzer/main.go",
      "args": [
        "--rules=rule-example.yaml",
        "--provider-settings=provider_settings.json",
        "--output-file=debug-output.yaml"
      ]
    },
    {
      "name": "Debug Tests",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${workspaceFolder}/engine"
    }
  ]
}
```

### GoLand / IntelliJ IDEA

1. **Open project:** File → Open → Select `analyzer-lsp` directory

2. **Configure Go SDK:** File → Settings → Go → GOROOT

3. **Run Configuration:**
   - Run → Edit Configurations
   - Add → Go Build
   - Package path: `github.com/konveyor/analyzer-lsp/cmd/analyzer`
   - Program arguments: `--rules=rule-example.yaml --provider-settings=provider_settings.json`

### Vim/Neovim

Install gopls and configure with vim-go or coc.nvim:

```vim
" .vimrc / init.vim
let g:go_def_mode='gopls'
let g:go_info_mode='gopls'
```

## Debugging

### Debugging the Analyzer

#### Using VS Code

1. Set breakpoints in code
2. Press F5 or use Debug → Start Debugging
3. Analyzer runs with configured arguments

#### Using Delve (CLI)

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug analyzer
dlv debug ./cmd/analyzer/main.go -- \
  --rules=rule-example.yaml \
  --provider-settings=provider_settings.json \
  --output-file=debug-output.yaml

# In delve:
(dlv) break main.main
(dlv) continue
(dlv) next
(dlv) print variableName
```

#### Debugging with Logs

Set verbose logging:

```bash
go run cmd/analyzer/main.go \
  --verbose=9 \
  --rules=rule-example.yaml \
  --provider-settings=provider_settings.json
```

Log levels:
- `0-2` - Errors only
- `3-5` - Warnings and info
- `6-9` - Debug and trace

### Debugging Providers

#### Local Provider Debug

Run provider standalone:

```bash
# Generic provider (Go example)
./build/generic-external-provider \
  --port 14653 \
  --name gopls

# In another terminal, attach analyzer
go run cmd/analyzer/main.go \
  --provider-settings=local_provider_settings.json \
  --rules=rule-example.yaml
```

#### Container Provider Debug

```bash
# Start provider with logs
podman run -it --rm \
  -p 14651:14651 \
  localhost/java-provider:latest \
  --port 14651

# View logs in real-time
podman logs -f java-provider
```

### Debugging LSP Communication

Enable LSP tracing:

```bash
# Set environment variable
export LSP_LOG_PATH=/tmp/lsp.log

# Run analyzer
go run cmd/analyzer/main.go --verbose=9 ...

# View LSP traffic
tail -f /tmp/lsp.log
```

### Debugging with OpenTelemetry

Enable Jaeger tracing:

```bash
# Start Jaeger (using Docker)
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 14268:14268 \
  jaegertracing/all-in-one:latest

# Run analyzer with tracing
go run cmd/analyzer/main.go \
  --enable-jaeger \
  --jaeger-endpoint=http://localhost:14268/api/traces \
  --rules=rule-example.yaml

# View traces at http://localhost:16686
```

## Troubleshooting

### Common Issues

#### Go Module Issues

**Problem:** `cannot find module providing package`

**Solution:**
```bash
go mod tidy
go mod download
```

#### Build Failures

**Problem:** `sed: illegal option` on macOS

**Solution:** macOS uses BSD sed. Either:
```bash
# Install GNU sed
brew install gnu-sed

# Or run build commands directly
cd external-providers/generic-external-provider
go build -o ../../build/generic-external-provider main.go
```

#### Container Image Issues

**Problem:** `Error: image not found`

**Solution:**
```bash
# Rebuild images
make image-build
make build-external
```

#### Port Already in Use

**Problem:** `bind: address already in use`

**Solution:**
```bash
# Find process using port
lsof -i :14651
# or
netstat -tuln | grep 14651

# Kill process or change port in provider_settings.json
```

#### LSP Server Not Found

**Problem:** `exec: "gopls": executable file not found in $PATH`

**Solution:**
```bash
# Ensure GOBIN is in PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Reinstall gopls
go install golang.org/x/tools/gopls@latest
```

### Getting Help

If you encounter issues:

1. Check existing [GitHub Issues](https://github.com/konveyor/analyzer-lsp/issues)
2. Review [CONTRIBUTING.md](../../CONTRIBUTING.md)
3. Join Konveyor community discussions
4. Open a new issue with:
   - OS and version
   - Go version (`go version`)
   - Steps to reproduce
   - Error messages and logs

## Environment Variables

Useful environment variables for development:

```bash
# Go build settings
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0

# Go module proxy
export GOPROXY=https://proxy.golang.org,direct

# Testing
export PODMAN_USERNS=keep-id  # For rootless podman
```

## Next Steps

Once your environment is set up:

1. Run the test suite: `make test-all`
2. Try running the analyzer: `go run cmd/analyzer/main.go`
3. Read the [Testing Guide](testing.md) to understand the test infrastructure
4. Review the [Architecture](architecture.md) to understand the codebase
5. See [Provider Development](provider_development.md) to build new providers

## Development Workflow

Typical development workflow:

```bash
# 1. Create a feature branch
git checkout -b feature/my-feature

# 2. Make changes to code

# 3. Run unit tests
go test ./...

# 4. Build locally
make build

# 5. Run E2E tests
make test-all

# 6. Commit changes
git add .
git commit -m "Add my feature"

# 7. Push and create PR
git push origin feature/my-feature
```

## Performance Tips

### Faster Builds

```bash
# Use build cache
go build -o build/konveyor-analyzer ./cmd/analyzer
```

### Faster Tests

```bash
# Run tests in parallel
go test -parallel 4 ./...

# Cache test results
go test -count=1 ./...  # disable cache
go test ./...          # use cache
```

### Reduce Container Build Time

```bash
# Use layer caching
podman build --layers -t analyzer-lsp .

# Multi-stage builds are already optimized in Dockerfiles
```
