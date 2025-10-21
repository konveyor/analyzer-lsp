//go:build windows

package socket

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GetAddress(name string) (string, error) {
	randInt := rand.Int()
	pipe_name := fmt.Sprintf("\\\\.\\pipe\\%s-%v", name, randInt)
	return pipe_name, nil

}

func GetConnectionString(address string) string {
	return fmt.Sprintf("passthrough:unix://%s", address)
}

func ConnectGRPC(connectionString string) (*grpc.ClientConn, error) {
	// Note that gRPC by default performs name resolution on the target passed to NewClient.
	// // To bypass name resolution and cause the target string to be passed directly to the dialer here instead, use the "passthrough" resolver by specifying it in the target string, e.g. "passthrough:target".
	return grpc.NewClient(connectionString,
		grpc.WithContextDialer(DialWindowsPipePassthrough),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MAX_MESSAGE_SIZE)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("localhost"),
	)
}

func Listen(socketName string) (net.Listener, error) {
	return winio.ListenPipe(socketName, &winio.PipeConfig{})
}

func DialWindowsPipePassthrough(ctx context.Context, connectionString string) (net.Conn, error) {
	pipeName, _ := strings.CutPrefix(connectionString, "unix://")
	pipe, err := winio.DialPipeContext(ctx, pipeName)
	if err != nil {
		return nil, err
	}
	fmt.Printf("pipe: %#v", pipe)
	return pipe, nil
}

func ConnectRPC(ctx context.Context, address string) (*jsonrpc2.Connection, error) {
	wrapper := jsonrpc2WindowsWrapper{address}
	conn, err := jsonrpc2.Dial(ctx, &wrapper, jsonrpc2.ConnectionOptions{})
	if err != nil {
		return nil, err
	}
	return conn, nil
}

type jsonrpc2WindowsWrapper struct {
	address string
}

func (j *jsonrpc2WindowsWrapper) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	conn, err := winio.DialPipeContext(ctx, j.address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
