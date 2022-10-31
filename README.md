# Analyzer Rule Engine 

In this project, we are writing a rule engine that can use pluggable providers for rules to make a consistent way to execute rules for the konveyor hub. 

One of the primary drivers for this repository is adding providers for specific languages using the Language Server Protocol. Today these providers are in tree, but we will be moving them out in the future.

## Quick Demo

If you would like to run a quick demo we have a Dockerfile that has all the dependencies.

To run this demo build the container:

```
$ podman build -f Dockerfile -t test-analyzer-engine
```

This will build the engine, and include the current set of rules and examples in the container to be used. 

To run the rules (rule-example.yaml) against the examples:

```
$ podman run test-analyzer-engine
```

## Code Base Starting Point

 Using the LSP/Protocal from ACME https://github.com/fhs/acme-lsp and stripping out anything related to serving, proxy or anything. Just keeping the types for communication

 Using JSONRPC2 from google.org/x/tools/internal. Copied and removed anything to do with serving. 


## Code of Conduct

Refer to Konveyor's [Code of Conduct page](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md)
