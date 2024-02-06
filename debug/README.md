Debugging within the container
==
Given the containerized nature of the analyzer, and its dependency on external components, many times it will be convenient to debug its code
running inside a container. This can be achieved by executing the main command with the [Go Delve debugger](https://github.com/go-delve/delve).

`debug.Dockerfile` makes it possible to do this, and also to connect from an external IDE, like GoLand or VSCode.

### Debugging from Goland
You can follow [these instructions](https://blog.jetbrains.com/go/2020/05/06/debugging-a-go-application-inside-a-docker-container/) to debug the analyzer from the GoLand IDE.

### Debugging from VSCode
Follow [this blog post](https://dev.to/bruc3mackenzi3/debugging-go-inside-docker-using-vscode-4f67) to debug from VSCode. The Dockerfile creation steps can be skipped.