# generic-external-provider

The generic-external-provider has two main goals:

1. Create a generic provider for analyzer-lsp, leveraging the power of existing
   LSP servers to easily support new languages

2. Create a library of base components so it's easy to add new specific language
   support.

For example, if your language has an LSP server called `foo-lsp`, you can either
use the `generic` server configuration to get some functionality right out of
the box by setting an `analyzer-lsp` configuration like this:

```yaml
{
  "name": "foo",
  "binaryPath": "/path/to/generic-external-provider",
  "initConfig": [{
      "analysisMode": "full",
      "providerSpecificConfig": {
          "lspServerName": "generic",
          "lspServerPath": "/path/to/foo-lsp",
          "lspServerArgs": ["args", "go", "here"],
          "lspServerInitializationOptions": "",

          "workspaceFolders": ["file:///folder/to/analyze"],
          "dependencyFolders": [],

          "dependencyProviderPath": ""
      }
  }]
}
```

Or you can create a new service client to add more service-client-specific
capabilities.

`jsonrpc2_v2` and `event` libraries from [gopls's internal
libraries](https://github.com/golang/tools/tree/master/internal/jsonrpc2_v2)

## Adding Support for New Languages by Creating a New Service Client

Suppose the name of your language server is `foo-lsp`. The recommended pattern
of adding new service clients is:

1. Create a new folder in `server_configurations` with the name of your lsp
   server, `foo_lsp`. Copy `generic/service_client.go` into the new directory. 
   
1. Change the package name to `foo_lsp`. Change the occurrences of
   `GenericServiceClient` to `FooServiceClient`.

1. Add any variables you need to the `FooServiceClient` and
   `FooServiceClientConfig` struct.

1. Modify any parameters related to the `initialize` request. If you need
   additional `jsonrpc2_v2` handlers, say for responding to messages from the
   server in a specific way, pass those into the base service client.

1. Implement your capabilities and update the `FooServiceClientCapabilities`
   slice

1. In constants.go, add `NewFooServiceClient` to SupportedLanguages and
   `FooServiceClientCapabilities` to SupportedCapabilities

## Architecture

Each instance of `generic-external-provider` supports one type of service
client. For example, you can't have one `gep` doing things for pylsp *and*
gopls, you must spawn to separate processes.

*TODO: Talk about jsonrpc2_v2 dialers and CmdDialer*

*TODO: Talk about jsonrpc2_v2 handlers (ChainHandler) and AwaitCache*

## Rationale

### What's going on with `LSPServiceClientEvaluator`? `ServiceClientCapabilities` slices with functions?

LSPServiceClientEvaluator was an attempt to kill two birds with one stone. 

We must already send `analyzer-lsp` what capabilities we support. Additionally,
each `service_client.go` in `server_configurations` must conform to the
`Evaluate` method in `protocol.ServiceClient`. The evaluate method must process
the requests based on which capability is requested. 

However, we already know what capabilities each each server configuration has at
compile time. Thus we can define the capabilities that we send for each service
client as a slice, and create single embed-able struct,
`LSPServiceClientEvaluator`, that calls the appropriate capability function (as
long as it has a reference to the actual service client) using this same slice,
reducing code duplication. 

`LSPServiceClientEvaluator` also allows us to have generic capabilities that can
be utilized by multiple different service clients. Each service client that
embeds the `LSPServiceClientBase` struct also embeds the
`GetLSPServiceClientBase` method. Keeping in mind that receiver functions [are
just regular functions that take the receiver as the first
argument](https://go.dev/ref/spec#Method_expressions), we can create generic
capability functions that work for any service client like so:

```go
func EvaluateReferenced[T base](t T, ctx ctx, cap string, info []byte) (resp, error) {
	sc := t.GetLSPServiceClientBase()

  // do stuff with sc, the base of the service client
}
```

Take a look at `base_capabilities.go` for some more examples. 

This receiver-function-to-regular-function trick is also how we load in the
service-client-specific methods to the evaluator as well.

### Why use `jsonrpc2_v2` as opposed to the current `jsonrpc` implementation or `go.lsp.dev/jsonrpc`?

The choice to use `jsonrpc2_v2` over the existing jsonrpc implementation or
`go.lsp.dev/jsonrpc` is based on several key factors. Firstly, the current
jsonrpc implementation lacks essential features, such as the ability to respond
to server requests, which is crucial for bidirectional communication.

While `go.lsp.dev/jsonrpc` addresses this issue, it introduces a new challenge
as both the current and `go.lsp.dev` implementations are copies of the internal
gopls code, making them inherently unstable. However, the compelling rationale
for `jsonrpc2_v2` is that there are plans to transition it from an internal to a
public API, [as indicated in this GitHub
issue](https://github.com/golang/go/issues/46187). 

This future move positions `jsonrpc2_v2` as a more feature-rich and
forward-looking solution, aligning with the broader community's intentions to
establish it as a public API, enhancing its long-term stability and
functionality for our project's needs.

