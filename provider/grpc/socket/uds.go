//go:build !windows

package socket

import (
	"fmt"
	"net"
	"os"
)

func GetSocketAddress(name string) (string, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("provider-%v-*.sock", name))
	if err != nil {
		return "", err
	}
	f.Close()
	os.Remove(f.Name())
	return f.Name(), nil

}

func Listen(socket string) (net.Listener, error) {
	return net.Listen("unix", fmt.Sprintf("unix://%s", socket))

}
