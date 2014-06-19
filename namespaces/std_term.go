package namespaces

import (
	"io"
	"os"
	"os/exec"
)

type StdTerminal struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

func (s *StdTerminal) SetMaster(*os.File) {
	// no need to set master on non tty
}

func (s *StdTerminal) Close() error {
	return nil
}

func (s *StdTerminal) Resize(h, w int) error {
	return nil
}

func (s *StdTerminal) Attach(command *exec.Cmd) error {
	inPipe, err := command.StdinPipe()
	if err != nil {
		return err
	}
	outPipe, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	errPipe, err := command.StderrPipe()
	if err != nil {
		return err
	}

	go func() {
		defer inPipe.Close()
		io.Copy(inPipe, s.stdin)
	}()

	go io.Copy(s.stdout, outPipe)
	go io.Copy(s.stderr, errPipe)

	return nil
}

func (s *StdTerminal) GetFds() (*[3]uintptr, error) {
	inr, inw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outr, outw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errr, errw, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	go func() {
		defer inw.Close()
		io.Copy(inw, s.stdin)
	}()

	go io.Copy(s.stdout, outr)
	go io.Copy(s.stderr, errr)
	return &[3]uintptr{inr.Fd(), outw.Fd(), errw.Fd()}, nil
}
