package base

import (
	"context"
	"errors"
	"io"

	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
)

// Dialers in jsonrpc2_v2 return a ReadWriteCloser that sends and receives
// information from the server. StdDialer functions as a proxy for the input and output
// from the given reader and writer.
type StdDialer struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
}

// Create a new CmdDialer
func NewStdDialer(stdin io.WriteCloser, stdout io.ReadCloser) jsonrpc2.Dialer {
	stdDialer := StdDialer{}

	stdDialer.Stdin = stdin
	stdDialer.Stdout = stdout

	return &stdDialer
}

func (rwc *StdDialer) Read(p []byte) (int, error) {
	return rwc.Stdout.Read(p)
}

func (rwc *StdDialer) Write(p []byte) (int, error) {
	return rwc.Stdin.Write(p)
}

func (rwc *StdDialer) Close() error {
	inErr := rwc.Stdin.Close()
	outErr := rwc.Stdout.Close()
	if inErr != nil || outErr != nil {
		return errors.Join(inErr, outErr)
	}
	return nil
}

// StdDialer.Dial returns itself as a ReadWriteCloser.
func (rwc *StdDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	return rwc, nil
}
