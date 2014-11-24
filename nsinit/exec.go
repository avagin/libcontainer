package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/codegangsta/cli"
	"github.com/docker/docker/pkg/term"
	"github.com/docker/libcontainer"
	consolepkg "github.com/docker/libcontainer/console"
	"github.com/docker/libcontainer/namespaces"
)

var execCommand = cli.Command{
	Name:   "exec",
	Usage:  "execute a new command inside a container",
	Action: execAction,
	Flags: []cli.Flag{
		cli.BoolFlag{Name: "list", Usage: "list all registered exec functions"},
		cli.StringFlag{Name: "func", Value: "exec", Usage: "function name to exec inside a container"},
	},
}

func execAction(context *cli.Context) {
	if context.Bool("list") {
		w := tabwriter.NewWriter(os.Stdout, 10, 1, 3, ' ', 0)
		fmt.Fprint(w, "NAME\tUSAGE\n")

		for k, f := range argvs {
			fmt.Fprintf(w, "%s\t%s\n", k, f.Usage)
		}

		w.Flush()

		return
	}

	var exitCode int

	process := &libcontainer.ProcessConfig{
		Args:   context.Args(),
		Env:    context.StringSlice("env"),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	config, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	factory, err := libcontainer.New(context.GlobalString("root"), logger)
	if err != nil {
		log.Fatal(err)
	}

	container, err := factory.Create("test", config) //FIXME
	if err != nil {
		log.Fatal(err)
	}

	_, err = container.StartProcess(process)
	container.Wait()

	if err != nil {
		log.Fatalf("failed to exec: %s", err)
	}

	os.Exit(exitCode)
}

// the process for execing a new process inside an existing container is that we have to exec ourself
// with the nsenter argument so that the C code can setns an the namespaces that we require.  Then that
// code path will drop us into the path that we can do the final setup of the namespace and exec the users
// application.
func startInExistingContainer(config *libcontainer.Config, state *libcontainer.State, action string, context *cli.Context) (int, error) {
	var (
		master  *os.File
		console string
		err     error

		sigc = make(chan os.Signal, 10)

		stdin  = os.Stdin
		stdout = os.Stdout
		stderr = os.Stderr
	)
	signal.Notify(sigc)

	if config.Tty {
		stdin = nil
		stdout = nil
		stderr = nil

		master, console, err = consolepkg.CreateMasterAndConsole()
		if err != nil {
			return -1, err
		}

		go io.Copy(master, os.Stdin)
		go io.Copy(os.Stdout, master)

		state, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return -1, err
		}

		defer term.RestoreTerminal(os.Stdin.Fd(), state)
	}

	startCallback := func(cmd *exec.Cmd) {
		go func() {
			resizeTty(master)

			for sig := range sigc {
				switch sig {
				case syscall.SIGWINCH:
					resizeTty(master)
				default:
					cmd.Process.Signal(sig)
				}
			}
		}()
	}

	return namespaces.ExecIn(config, state, context.Args(), os.Args[0], action, stdin, stdout, stderr, console, startCallback)
}

func resizeTty(master *os.File) {
	if master == nil {
		return
	}

	ws, err := term.GetWinsize(os.Stdin.Fd())
	if err != nil {
		return
	}

	if err := term.SetWinsize(master.Fd(), ws); err != nil {
		return
	}
}
