# Hello World

This example demonstrates our ability to perform rules based analysis of a .NET
Core project mounted in the provider container we provide.

# Requirements

This walkthrough assumes a few things:

* This repository is cloned and go toolset is installed
* [Podman](https://podman.io/) is installed

# Procedure

## Run Provider

This provider is, by design, not included in the analyzer-lsp image. Therefore,
we must start the provider before running the analyzer.

```shell
podman run -it --rm -P -u 1000:1000 \
    -v $PWD/external-providers/dotnet-external-provider/examples/:$PWD/external-providers/dotnet-external-provider/examples/ \
    quay.io/konveyor/dotnet-external-provider:latest
```

**NOTE** Using the exact same path inside the container and host is our way of
simulating a shared filesystem between the analyzer and provider.

Verify provider is running and take note of open ports:

```shell
➜  podman ps
CONTAINER ID  IMAGE                                             COMMAND     CREATED         STATUS         PORTS                                             NAMES
c3fde1874ff9  quay.io/konveyor/dotnet-external-provider:latest              10 seconds ago  Up 10 seconds  0.0.0.0:33095->3456/tcp, 0.0.0.0:34093->8080/tcp  nostalgic_joliot
```

Port `33095` is what we'll need for the provider settings.

## Update Provider Settings

Find the [provider-settings-example.json](./provider-settings-example.json) and
update the `address` based on the previous step and `location` with the absolute
path to the `HelloWorld` project we are analyzing.

## Run the Analyzer

From the root of the analyzer-lsp project, run the analyzer:

```shell
go run cmd/analyzer/main.go \
    --provider-settings external-providers/dotnet-external-provider/examples/HelloWorld/provider-settings-example.json \
    --rules external-providers/dotnet-external-provider/examples/HelloWorld/rule-example.yaml
```

After the run is complete, check the `output.yaml`. Expect it to look something like:

```yaml
- name: konveyor-analysis
  violations:
    dotnet-lang-ref-example-001:
      description: ""
      category: potential
      incidents:
      - uri: file:///path/to/analyzer-lsp/external-providers/dotnet-external-provider/examples/HelloWorld/HelloWorld/Program.cs
        message: This method is not portable
        codeSnip: " 3      class Program\n 4      {\n 5          public void NonPortableMethod()\n 6          {\n 7              Console.WriteLine(\"Hello World!\");\n 8          }\n 9  \n10          static void Main(string[] args)\n11          {\n12              Program p = new Program();\n13              p.NonPortableMethod();\n14          }\n15      }\n16  }\n"
        lineNumber: 12
        variables:
          file: file:///path/to/analyzer-lsp/external-providers/dotnet-external-provider/examples/HelloWorld/HelloWorld/Program.cs
```

# Conclusion

At this point, we have effectively demonstrated our ability to analyze a .NET 8
project in our dotnet-external-provider container.

What's next? If you made it through this example and have experience with .NET,
we would love for you to
[contribute](https://github.com/konveyor/community/blob/main/CONTRIBUTING.md)
or [reach out](https://github.com/konveyor/community#communication) to help
make this better.
