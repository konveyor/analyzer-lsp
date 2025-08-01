//go:build !windows

package socket

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func ConnectGRPC(connectionString string) (*grpc.ClientConn, error) {
	return grpc.NewClient(connectionString,
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MAX_MESSAGE_SIZE)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))

}
