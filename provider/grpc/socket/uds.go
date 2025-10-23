//go:build !windows

package socket

import (
	"context"
	"fmt"
	"net"
	"os"

	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GetAddress(name string) (string, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("provider-%v-*.sock", name))
	if err != nil {
		return "", err
	}
	f.Close()
	os.Remove(f.Name())
	return f.Name(), nil

}

func GetConnectionString(address string) string {
	return fmt.Sprintf("unix://%s", address)
}

func Listen(address string) (net.Listener, error) {
	return net.Listen("unix", address)
}

func ConnectGRPC(connectionString string) (*grpc.ClientConn, error) {
	return grpc.NewClient(connectionString,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(MAX_MESSAGE_SIZE),
		),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

func ConnectRPC(ctx context.Context, address string, handler jsonrpc2.Handler) (*jsonrpc2.Connection, error) {
	dialer := jsonrpc2.NetDialer("unix", address, net.Dialer{})
	options := jsonrpc2.ConnectionOptions{}
	if handler != nil {
		options.Handler = handler
	}
	conn, err := jsonrpc2.Dial(ctx, dialer, options)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
