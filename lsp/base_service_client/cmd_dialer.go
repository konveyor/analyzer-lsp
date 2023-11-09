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
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// Create a new CmdDialer
func NewCmdDialer(ctx context.Context, name string, arg ...string) (*CmdDialer, error) {
	cmd := exec.CommandContext(ctx, name, arg...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		err := cmd.Start()
		// fmt.Printf("pid: %d\n", cmd.Process.Pid)

		if err != nil {
			fmt.Printf("cmd failed: %v", err)
			return
		}
	}()

	return &CmdDialer{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (rwc *CmdDialer) Read(p []byte) (int, error) {
	return rwc.stdout.Read(p)
}

func (rwc *CmdDialer) Write(p []byte) (int, error) {
	return rwc.stdin.Write(p)
}

func (rwc *CmdDialer) Close() error {
	err := rwc.cmd.Process.Kill()
	if err != nil {
		return err
	}

	return rwc.cmd.Wait()
}

// CmdDialer.Dial returns itself as a CmdDialer is a ReadWriteCloser.
func (rwc *CmdDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	// TODO(jsussman): Check if already closed
	return rwc, nil
}
