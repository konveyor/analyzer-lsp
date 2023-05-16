To update the Provider GRPC definition, update `library.proto` and run:

```
$ protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative provider/internal/grpc/library.proto
```