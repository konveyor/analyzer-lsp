To install protoc, go to https://github.com/protocolbuffers/protobuf/releases 
and download the appropriate zip file, say `protoc-24.3-linux-x86_64.zip` to
`~/Downloads/`. Then run:

```sh
cd ~/Downloads/
unzip protoc-24.3-linux-x86_64.zip -d protoc-24.3-linux-x86_64
cd protoc-24.3-linux-x86_64
sudo mv ./bin/* /usr/local/bin
sudo mv ./include/* /usr/local/include
```

To update the Provider GRPC definition, update `library.proto` and run:

```sh
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative provider/internal/grpc/library.proto
```