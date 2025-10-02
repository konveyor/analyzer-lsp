//go:build !windows

package socket

import (
	"fmt"
	"net"
	"os"

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
