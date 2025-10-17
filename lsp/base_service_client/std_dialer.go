package base

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Dialers in jsonrpc2_v2 return a ReadWriteCloser that sends and receives
// information from the server. CmdDialer functions as a both a ReadWriteCloser
// to the spawned process and as a Dialer that returns itself.
//
// NOTE: Dial should only be called once. This is because closing CmdDialer also
// kills the underlying process
type StdDialer struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser

	err error
}

// Create a new CmdDialer
func NewStdDialer(ctx context.Context, name string, stdin io.WriteCloser, stdout io.ReadCloser) (*StdDialer, error) {
	stdDialer := StdDialer{}

	stdDialer.Stdin = stdin
	stdDialer.Stdout = stdout

	return &stdDialer, nil
}

func (rwc *StdDialer) Read(p []byte) (int, error) {
	if rwc.err != nil {
		return 0, fmt.Errorf("cannot read: %w", rwc.err)
	}
	return rwc.Stdout.Read(p)
}

func (rwc *StdDialer) Write(p []byte) (int, error) {
	if rwc.err != nil {
		return 0, fmt.Errorf("cannot write: %w", rwc.err)
	}
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

// CmdDialer.Dial returns itself as a CmdDialer is a ReadWriteCloser.
func (rwc *StdDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	// TODO(jsussman): Check if already closed
	if rwc.err != nil {
		return rwc, fmt.Errorf("cannot close: %w", rwc.err)
	}
	return rwc, nil
}
