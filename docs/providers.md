# Providers

The analyzer rule engine uses pluggable providers that enable source code analysis. Providers communicate with the engine over gRPC. Currently, some providers are also in-tree.

## Configuring providers

Provider configurations go in a JSON file. It's a list of JSON objects with each object being configuration for a provider.

Provider configuration fields are:

* `name`: Name of the provider.
* `binaryPath`: Path to binary used to initiate a gRPC provider.
* `address`: Remote address of an already running gRPC provider.
* `initConfig`: List of init configs for the provider.
  * `location`: Path to the source code / binary of the application to analyze. Note that only `java` provider supports binary analysis.
  * `dependencyPath`: Path to look for dependencies of the app.
  * `lspServerPath`: Path to language server binary used by the provider.
  * `analysisMode`: one of full or source-only. This will tell the provider what it should analyze.
  * `providerSpecificConfig`: Reserved for additional configuration options specific to a provider.

Currently supported providers are - `builtin`, `java` and `go`, or any provider that provides the GRPC interface.

```Note For Java: full analysis mode will search all the dependency and source, source-only will only search the source code. for a Jar/Ear/War, this is the code that is compiled in that archive and nothing else.
```

#### Go provider

Here's an example config for an external `go` provider that is initialized using a binary and works on gRPC:

```json
{
    "name": "go",
    "binaryPath": "/path/to/go/grpc/provider/binary",
    "initConfig": [
        {
            "location": "/path/to/application/source/code",
            "lspServerPath": "/path/to/language/server/binary",
            "analysisMode": "full",
        }
    ]
}
```

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
            }
        }
    ]
}
```

The `location` can be a path to the application's source code or to a binary JAR, WAR, or EAR file.

The `java` provider also takes following options in `providerSpecificConfig`:

* `bundles`: Path to extension bundles to enhance default Java language server's capabilities. See the [bundle](https://github.com/konveyor/java-analyzer-bundle) Konveyor uses.

* `workspace`: Path to directory where the provider generates debug information such as logs.

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
