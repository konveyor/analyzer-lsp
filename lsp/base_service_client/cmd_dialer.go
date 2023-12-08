package base

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Dialers in jsonrpc2_v2 return a ReadWriteCloser that sends and receives
// information from the server. CmdDialer functions as a both a ReadWriteCloser
// to the spawned process and as a Dialer that returns itself.
//
// NOTE: Dial should only be called once. This is because closing CmdDialer also
// kills the underlying process
type CmdDialer struct {
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser

	err error
}

// Create a new CmdDialer
func NewCmdDialer(ctx context.Context, name string, arg ...string) (*CmdDialer, error) {
	cmdDialer := CmdDialer{}

	Cmd := exec.CommandContext(ctx, name, arg...)

	Stdin, err := Cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	Stdout, err := Cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		err := Cmd.Start()
		// fmt.Printf("pid: %d\n", cmd.Process.Pid)

		if err != nil {
			cmdDialer.err = fmt.Errorf("cmd failed: %w", err)
			return
		}
	}()

	cmdDialer.Cmd = Cmd
	cmdDialer.Stdin = Stdin
	cmdDialer.Stdout = Stdout

	return &cmdDialer, nil
}

func (rwc *CmdDialer) Read(p []byte) (int, error) {
	if rwc.err != nil {
		return 0, fmt.Errorf("cannot read: %w", rwc.err)
	}
	return rwc.Stdout.Read(p)
}

func (rwc *CmdDialer) Write(p []byte) (int, error) {
	if rwc.err != nil {
		return 0, fmt.Errorf("cannot write: %w", rwc.err)
	}
	return rwc.Stdin.Write(p)
}

func (rwc *CmdDialer) Close() error {
	err := rwc.Cmd.Process.Kill()
	if err != nil {
		return err
	}

	return rwc.Cmd.Wait()
}

// CmdDialer.Dial returns itself as a CmdDialer is a ReadWriteCloser.
func (rwc *CmdDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	// TODO(jsussman): Check if already closed
	if rwc.err != nil {
		return rwc, fmt.Errorf("cannot close: %w", rwc.err)
	}
	return rwc, nil
}
