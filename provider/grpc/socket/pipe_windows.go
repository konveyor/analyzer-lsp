//go:build windows

package socket

import (
	"fmt"
	"math/rand"
	"net"

	"github.com/Microsoft/go-winio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Need to redefine here for windows versions.
const (
	MAX_MESSAGE_SIZE = 1024 * 1024 * 8
)

func GetSocketAddress(name string) (string, error) {
	randInt := rand.Int()
	return fmt.Sprintf("\\\\.\\pipe\\%s-%v", name, randInt), nil

}
func ConnectGRPC(connectionString string) (*grpc.ClientConn, error) {
	// Note that gRPC by default performs name resolution on the target passed to NewClient.
	// // To bypass name resolution and cause the target string to be passed directly to the dialer here instead, use the "passthrough" resolver by specifying it in the target string, e.g. "passthrough:target".
	return grpc.NewClient(fmt.Sprintf("passthrough:%s", connectionString),
		grpc.WithContextDialer(winio.DialPipeContext),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MAX_MESSAGE_SIZE)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func Listen(socketName string) (net.Listener, error) {
	return winio.ListenPipe(socketName, &winio.PipeConfig{})
}
