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

Currently supported providers are - `builtin`, `java` and `go`, or any provider that provides the GRPC interface.

If an explicit `proxyConfig` is not specified for a provider, system-wide proxy settings configured via environment variables `http_proxy`, `https_proxy` & `no_proxy` are used by default. An explicit `proxyConfig` is typically needed for providers that run externally and are not part of the same process as the rule engine. For the rule engine and the builtin providers, system-wide proxy settings are sufficient.

```Note For Java: full analysis mode will search all the dependency and source, source-only will only search the source code. for a Jar/Ear/War, this is the code that is compiled in that archive and nothing else.
```

#### Generic provider

Generic provider can be used to create an external provider for any language that is compliant with LSP 3.17 specifications.

Here's an example config for a external `go` provider that is initialized using the generic provider binary.

```json
{
    "name": "go",
    "binaryPath": "/path/to/generic/provider/binary",
    "initConfig": [
        {
            "location": "/path/to/application/source/code",
            "analysisMode": "full",
            "providerSpecificConfig": {
                "name": "go",
                "lspServerPath": "/path/to/language/server/binary",
                "lspArgs": ["arg1", "arg2", "arg3"],
                "dependencyProviderPath": "/path/to/dependency/provider/binary"
            }
        }
    ]
}
```

The `generic provider` takes the following options in `providerSpecificConfig`:

* `name`: Name of the provider to be displayed in the logs.

* `lspArgs`: Arguments to be passed to run the langauge server. Optional field.

* `dependencyProviderPath`: Path to a binary that prints the dependencies of the application as a `map[uri.URI][]provider.Dep{}`. The Dep struct can be imported from 
`"github.com/konveyor/analyzer-lsp/provider"`.

#### Java provider

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

#### Builtin Provider

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
