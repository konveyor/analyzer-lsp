# Providers

The analyzer rule engine uses pluggable providers that enable source code analysis. Providers communicate with the engine over gRPC. Currently, some providers are also in-tree.

## Configuring providers

Provider configurations go in a JSON file. It's a list of JSON objects with each object being configuration for a provider.

Provider configuration fields are:

* `name`: Name of the provider.
* `binaryPath`: Path to binary used to initiate a gRPC provider.
* `address`: Remote address of an already running gRPC provider.
* `proxyConfig`: HTTP / HTTPS proxy to use. 
  * `httpproxy`: HTTP proxy string in format `<proto>://<user>@<password>:<host>:<port>`.
  * `httpsproxy`: HTTPS proxy string in format `<proto>://<user>@<password>:<host>:<port>`.
  * `noproxy`: Comma separated list of hosts excluded from the proxy.
* `initConfig`: List of init configs for the provider.
  * `location`: Path to the source code / binary of the application to analyze. Note that only `java` provider supports binary analysis.
  * `dependencyPath`: Path to look for dependencies of the app.
  * `lspServerPath`: Path to language server binary used by the provider.
  * `analysisMode`: one of full or source-only. This will tell the provider what it should analyze.
  * `providerSpecificConfig`: Reserved for additional configuration options specific to a provider.

In-tree and first-party external providers include `builtin`, `java`, `go`, `python`, `nodejs`, and `yaml` (via `yq-external-provider`). Any binary that implements the analyzer gRPC provider interface can be referenced from `binaryPath`.

If an explicit `proxyConfig` is not specified for a provider, system-wide proxy settings configured via environment variables `http_proxy`, `https_proxy` & `no_proxy` are used by default. An explicit `proxyConfig` is typically needed for providers that run externally and are not part of the same process as the rule engine. For the rule engine and the builtin providers, system-wide proxy settings are sufficient.

```Note For Java: full analysis mode will search all the dependency and source, source-only will only search the source code. for a Jar/Ear/War, this is the code that is compiled in that archive and nothing else.
```

### External LSP providers (Go, Python, Node.js)

Go, Python, and Node.js each use a **dedicated** external provider binary (`go-external-provider`, `python-external-provider`, `nodejs-external-provider` in container images). The analyzer passes `--name` to the binary from `initConfig[0].providerSpecificConfig.lspServerName` (for gopls-backed rules this is typically `"generic"`; Python uses `"pylsp"` and Node.js uses `"nodejs"`).

Example for an external **Go** provider:

```json
{
    "name": "go",
    "binaryPath": "/usr/local/bin/go-external-provider",
    "initConfig": [
        {
            "location": "/path/to/application/source/code",
            "analysisMode": "full",
            "providerSpecificConfig": {
                "lspServerName": "generic",
                "lspServerPath": "/path/to/gopls",
                "lspServerArgs": ["arg1", "arg2"],
                "dependencyProviderPath": "/path/to/golang-dependency-provider"
            }
        }
    ]
}
```

Example for an external **Python** provider:

```json
{
    "name": "python",
    "binaryPath": "/usr/local/bin/python-external-provider",
    "initConfig": [
        {
            "location": "/path/to/application/source/code",
            "analysisMode": "full",
            "providerSpecificConfig": {
                "lspServerName": "pylsp",
                "lspServerPath": "/usr/local/bin/pylsp",
                "lspServerArgs": [],
                "workspaceFolders": ["file:///path/to/application/source/code"],
                "dependencyFolders": ["path/to/venv", "path/to/__pycache__"],
                "dependencyProviderPath": ""
            }
        }
    ]
}
```

Example for an external **Node.js** provider:

```json
{
    "name": "nodejs",
    "binaryPath": "/usr/local/bin/nodejs-external-provider",
    "initConfig": [
        {
            "location": "/path/to/application/source/code",
            "analysisMode": "full",
            "providerSpecificConfig": {
                "lspServerName": "nodejs",
                "lspServerPath": "/usr/local/bin/typescript-language-server",
                "lspServerArgs": ["--stdio"],
                "workspaceFolders": ["file:///path/to/application/source/code"],
                "dependencyFolders": [],
                "dependencyProviderPath": ""
            }
        }
    ]
}
```

Common `providerSpecificConfig` fields for these providers include:

* `lspServerName`: Passed to the external provider process as `--name`; must match the language / capability namespace your rules expect.

* `lspServerPath` / `lspServerArgs`: How to start the language server.

* `workspaceFolders` / `dependencyFolders`: Workspace roots and paths to treat as dependencies (see [`provider_container_settings.json`](../provider_container_settings.json)).

* `dependencyProviderPath`: Optional path to a binary that prints dependencies as `map[uri.URI][]provider.Dep` (see `"github.com/konveyor/analyzer-lsp/provider"`). Often required for **Go**; may be empty for Python/Node when unused.

### Java provider

Here's an example config for `java` provider that is currently in-tree and does not use gRPC:

```json
{
    "name": "java",
    "binaryPath": "/path/to/language/server/binary",
    "initConfig": [
        {
            "location": "/path/to/application/source/or/binary",
            "lspServerPath": "/path/to/language/server/binary",
            "analysisMode": "full",
            "providerSpecificConfig": {
                "bundles": "/path/to/extension/bundles",
                "workspace": "/path/to/workspace",
                "depOpenSourceLabelsFile": "/usr/local/etc/maven.default.index",
                "mavenSettingsFile": "/path/to/maven/settings/file",
                "excludePackages": [
                    "package1.test",
                    "package2.test"
                ],
                "jvmMaxMem": "2048m",
            }
        }
    ]
}
```

The `location` can be a path to the application's source code or to a binary JAR, WAR, or EAR file. Optionally, coordinates to a maven artifact can be provided as input in the format `mvn://<group-id>:<artifact-id>:<version>:<classifier>@<path>`. The field `<path>` is optional, it specifies a local path where the artifact will be downloaded. If not specified, provider will use the current working directory to download it.

The `java` provider also takes following options in `providerSpecificConfig`:

* `bundles`: Path to extension bundles to enhance default Java language server's capabilities. See the [bundle](https://github.com/konveyor/java-analyzer-bundle) Konveyor uses.

* `workspace`: Path to directory where the provider generates debug information such as logs.

* `depOpenSourceLabelsFile`: Path to a text file, that contains the regex's per line to be added as open-source dependencies. The base image already contains a default file at `/usr/local/etc/maven.default.index`.

* `mavenSettingsFile`: Path to maven settings file (settings.xml) to use.

* `excludePackages`: List of dependency packages on which to add exclude label.

* `jvmMaxMem`: Max memory for JVM, value is passed as-is using `-Xmx` option. _Note that the default `-Xms` value set on JVM is `1G`, therefore, `jvmMaxMem` value less than `1G` has no effect_

### Builtin Provider

The `builtin` provider is configured by default. To override the default config, a new config can be added to provider settings file:

```json
{
    "name": "builtin",
    "initConfig": [
        {
            "location": "/home/pranav/Projects/windup-test-runner/links/apps/example-1/"
        }
    ]
}
```

The `builtin` provider takes following additional configuration options in `providerSpecificConfig`:

* `tagsFile`: Path to YAML file that contains a list of tags for the application being analyzed

* `excludedDirs`: List of directory names, paths, or patterns to exclude from analysis.

  **Path types:**
  - **Directory names** (e.g., `"node_modules"`, `"bower_components"`): Matched anywhere in the project tree
  - **Relative paths** (e.g., `"src/generated"`, `"test/fixtures"`): Treated as patterns relative to analyzed files
  - **Absolute paths** (e.g., `"/path/to/specific/dir"`): Match exact directory locations

  The following directories are excluded by default to prevent performance issues and "argument list too long" errors:
  - `node_modules` - JavaScript/TypeScript dependencies
  - `vendor` - PHP/Go dependencies
  - `.git` - Git repository data
  - `dist` - Build output
  - `build` - Build output
  - `target` - Java/Rust build output
  - `.venv`, `venv` - Python virtual environments

  **Behavior based on configuration:**

  - **Not configured** (default): The default excludes listed above are applied
  - **Empty array** (`[]`): No directories are excluded - analyzes everything including dependencies
  - **Non-empty array**: Default excludes are applied, plus any additional directories specified

  To add custom excludes in addition to the defaults:

  ```json
  {
      "name": "builtin",
      "initConfig": [
          {
              "location": "/path/to/application",
              "providerSpecificConfig": {
                  "excludedDirs": [
                      "bower_components",        // Exclude all bower_components dirs
                      "jspm_packages",            // Exclude all jspm_packages dirs
                      "generated",                // Exclude all dirs named "generated"
                      "/path/to/specific/dir"     // Exclude this specific directory only
                  ]
              }
          }
      ]
  }
  ```

  To disable all excludes and analyze everything (including dependencies):

  ```json
  {
      "name": "builtin",
      "initConfig": [
          {
              "location": "/path/to/application",
              "providerSpecificConfig": {
                  "excludedDirs": []
              }
          }
      ]
  }
  ```

  **Note:** Analyzing dependency directories like `node_modules` can significantly increase analysis time and may cause "argument list too long" errors on projects with many files.

## Migrating from `generic-external-provider`

The former **single** multi-language binary and image **`generic-external-provider`** has been **removed**. Go, Python, and Node.js now each have a **dedicated** binary and container image. This is a **breaking change** for anyone who referenced the old binary path or image name.

**If you must stay on the old layout temporarily**, pin an older analyzer-lsp release and image tag that still ships `quay.io/konveyor/generic-external-provider` (and a matching analyzer version), then plan a one-time config update.

### Registry images (Konveyor / Quay)

Published from [`.github/workflows/image-build.yaml`](https://github.com/konveyor/analyzer-lsp/blob/main/.github/workflows/image-build.yaml) to `quay.io/konveyor/` (tags such as `latest` or release branch names):

| Language | Image | Notes |
|----------|-------|--------|
| Go (gopls) | `quay.io/konveyor/go-external-provider:<tag>` | Built with `GOLANG_DEP_IMAGE=quay.io/konveyor/golang-dependency-provider:<tag>` |
| Python (pylsp) | `quay.io/konveyor/python-external-provider:<tag>` | |
| Node.js | `quay.io/konveyor/nodejs-external-provider:<tag>` | |
| (removed) | ~~`quay.io/konveyor/generic-external-provider`~~ | Replaced by the three images above |

Other related images (unchanged pattern): `quay.io/konveyor/analyzer-lsp`, `quay.io/konveyor/golang-dependency-provider`, `quay.io/konveyor/yq-external-provider`, `quay.io/konveyor/java-external-provider`.

### Container binary paths (default layout)

Inside the language provider images, the gRPC entrypoints are installed as:

| Provider | `binaryPath` inside image |
|----------|---------------------------|
| Go | `/usr/local/bin/go-external-provider` |
| Python | `/usr/local/bin/python-external-provider` |
| Node.js | `/usr/local/bin/nodejs-external-provider` |

Go analysis also expects **`dependencyProviderPath`** pointing at **`/usr/local/bin/golang-dependency-provider`** (from the `golang-dependency-provider` image or an equivalent build). See [`provider_container_settings.json`](../provider_container_settings.json) for a full working example.

### Config mapping (before → after)

| Before (one image, three processes) | After (three images or three binaries) |
|-----------------------------------|----------------------------------------|
| One `binaryPath` to `generic-external-provider` with `lspServerName` `generic` / `pylsp` / `nodejs` | **Separate** provider entries: each `binaryPath` points at `go-external-provider`, `python-external-provider`, or `nodejs-external-provider` |
| Same `lspServerName` values in `providerSpecificConfig` | **Keep** `lspServerName` as `generic` (gopls), `pylsp`, and `nodejs` respectively so existing **rules** (`golang.*`, `python.*`, `nodejs.*`) keep matching |

You need **three** provider blocks (or three sidecars) instead of reusing one binary with different `--name` flags.

### Draft release notes (changelog bullets)

Use or adapt the following in release announcements:

- **Breaking:** Removed `generic-external-provider`. Go, Python, and Node.js analysis now use `go-external-provider`, `python-external-provider`, and `nodejs-external-provider` binaries and `quay.io/konveyor/{go,python,nodejs}-external-provider` images.
- **Migration:** Replace each `binaryPath` that pointed at `generic-external-provider` with the per-language binary; deploy the matching new image per language. `lspServerName` values for rules remain `generic` / `pylsp` / `nodejs`.
- **YAML:** Unchanged—use `yq-external-provider` only (no YAML LSP path on the old generic binary).
- **Downstream:** Operator, Kantra, Hub/KAI, and custom integrators must update default images and extension references; see [downstream tracking](enhancements/generic-provider-to-specific-providers-downstream-tracking.md) for coordination.
