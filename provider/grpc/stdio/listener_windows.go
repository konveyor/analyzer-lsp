//go:build windows

package local

import (
	"net"
	"os"
	"sync"
)

type stdioListener struct {
	conn net.Conn
	once sync.Once
}

func NewLocalListener(path string) (net.Listener, error) {
	if path != "" {
		return net.Listen("unix", path)
	} else {
		return &stdioListener{
			conn: &stdioConn{
				Reader: os.Stdin,
				Writer: os.Stdout,
			},
		}, nil
	}
}
