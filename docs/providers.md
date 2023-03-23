# Providers

The analyzer rule engine uses pluggable providers that enable source code analysis. While the rule engine is responsible for parsing rules, invoking providers and generating output when rules match, the actual job of analyzing the source code is done by the providers. Currently, all the providers are in-tree. In future, we want to have external providers that communicate with the engine over gRPC.

## Configuring providers

Currently supported providers are - `builtin`, `java` and `golang`.

The config file for providers contains an array of JSON objects with each object being a configuration for a provider.

Provider configuration fields are:

* `name`: Name of the provider.
* `location`: Path to the source code of tha application to analyze.
* `dependencyPath`: Path to look for dependencies of the application.
* `binaryLocation`: Path to language server binary used by the provider. Note that a language server may or may not be used by a provider. For instance, the `builtin` provider does not use a language server.
* `providerSpecificConfig`: Reserved for additional configuration options specific to a provider.

Here's an example config for Go provider in `provider_settings.json`:

```json
[
    {
        "name": "go",
        "location": "examples/golang",
        "binaryLocation": "/usr/bin/gopls"
    },
]
```

### Provider Specific Configs

Some providers take additional config options specified via `providerSpecificConfig` field.

#### Java

The `java` provider takes following additional configuration options:

* `bundles`: Path to extension bundles to enhance default Java language server's capabilities. See the [bundle](https://github.com/konveyor/java-analyzer-bundle) Konveyor uses.
* `workspace`: Path to directory where the provider generates debug information such as logs.

#### Builtin

The `builtin` provider takes following additional configuration options:

* `tagsFile`: Path to YAML file that contains a list of tags for the application being analyzed

