// +build linux

package namespaces

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/cgroups"
	"github.com/docker/libcontainer/cgroups/fs"
	"github.com/docker/libcontainer/cgroups/systemd"
	"github.com/docker/libcontainer/network"
	"github.com/dotcloud/docker/pkg/system"
	"github.com/docker/libcontainer/utils"
	"github.com/docker/libcontainer/mount"
	libct "github.com/avagin/libct/go"
)

// Exec performes setup outside of a namespace so that a container can be
// executed.  Exec is a high level function for working with container namespaces.
func Exec(container *libcontainer.Container, term Terminal, rootfs, dataPath string, args []string, createCommand CreateCommand, startCallback func()) (int, error) {
	var (
		master  *os.File
		console string
		err     error
		fds	*[3]uintptr
	)

	// create a pipe so that we can syncronize with the namespaced process and
	// pass the veth name to the child
//	syncPipe, err := NewSyncPipe()
//	if err != nil {
//		return -1, err
//	}

	s := &libct.Session{}
	err = s.OpenLocal()
	if  err != nil {
		return -1, err
	}

	ct, err := s.ContainerCreate("docker")
	if err != nil {
		return -1, err
	}

	err = ct.SetNsMask(uint64(GetNamespaceFlags(container.Namespaces)))
	if err != nil {
		return -1, err
	}

	err = ct.SetOption(libct.LIBCT_OPT_AUTO_PROC_MOUNT)
	if err != nil {
		return -1, err
	}

	rootfs, err = utils.ResolveRootfs(rootfs)
	if err != nil {
		return -1, err
	}

	if container.Tty {
		master, console, err = system.CreateMasterAndConsole()
		if err != nil {
			return -1, err
		}
		term.SetMaster(master)
		ttyfd, err := os.OpenFile(console, os.O_RDWR, 0);
		if err != nil {
			return -1, err
		}
		term.Attach(nil)
		fds = &[3]uintptr{ttyfd.Fd(), ttyfd.Fd(), ttyfd.Fd()}

		err = ct.SetConsoleFd(ttyfd)
		if err != nil {
			return -1, err
		}
	} else {
		fds, err = term.GetFds()
	}

	defer term.Close()

	err = mount.InitializeMountNamespace(ct, rootfs, console, container);
	if err != nil {
		return -1, err
	}


//	command := createCommand(container, console, rootfs, dataPath, os.Args[0], syncPipe.child, args)
//
//	if err := term.Attach(command); err != nil {
//		return -1, err
//	}
//	defer term.Close()
//
//	if err := command.Start(); err != nil {
//		return -1, err
//	}

	if  container.Hostname != "" {
		err = ct.Uname(&container.Hostname, nil)
		if err != nil {
			return -1, err
		}
	}

	if err := InitializeNetworking(ct, container); err != nil {
		return -1, err
	}

//	syscall.RawSyscall(syscall.SYS_FCNTL, syncPipe.child.Fd(), syscall.F_SETFD, 0)
//	env := []string{
//		"console=" + console,
//		"pipe=" + fmt.Sprintf("%d", syncPipe.child.Fd()),
//		"data_path=" + dataPath,
//	}
//
//	err = ct.AddMount(os.Args[0], "./nsinit", 0)
//	if err != nil {
//		return -1, err
//	}
//
	err = ct.SpawnExecve(args[0], args, container.Env, fds)
	if err != nil {
		return -1, err
	}
//
//	started, err := system.GetProcessStartTime(command.Process.Pid)
//	if err != nil {
//		return -1, err
//	}
//	if err := WritePid(dataPath, command.Process.Pid, started); err != nil {
//		command.Process.Kill()
//		command.Wait()
//		return -1, err
//	}
//	defer DeletePid(dataPath)
//
//	// Do this before syncing with child so that no children
//	// can escape the cgroup
//	cleaner, err := SetupCgroups(container, command.Process.Pid)
//	if err != nil {
//		command.Process.Kill()
//		command.Wait()
//		return -1, err
//	}
//	if cleaner != nil {
//		defer cleaner.Cleanup()
//	}
//
//	if err := InitializeNetworking(container, command.Process.Pid, syncPipe); err != nil {
//		command.Process.Kill()
//		command.Wait()
//		return -1, err
//	}

	// Sync with child
//	syncPipe.Close()

	if startCallback != nil {
		startCallback()
	}

	ct.Wait()

//	if err := command.Wait(); err != nil {
//		if _, ok := err.(*exec.ExitError); !ok {
//			return -1, err
//		}
//	}
//	return command.ProcessState.Sys().(syscall.WaitStatus).ExitStatus(), nil
	return 0, nil
}

// DefaultCreateCommand will return an exec.Cmd with the Cloneflags set to the proper namespaces
// defined on the container's configuration and use the current binary as the init with the
// args provided
//
// console: the /dev/console to setup inside the container
// init: the progam executed inside the namespaces
// root: the path to the container json file and information
// pipe: sync pipe to syncronize the parent and child processes
// args: the arguemnts to pass to the container to run as the user's program
func DefaultCreateCommand(container *libcontainer.Container, console, rootfs, dataPath, init string, pipe *os.File, args []string) *exec.Cmd {
	// get our binary name from arg0 so we can always reexec ourself
	env := []string{
		"console=" + console,
		"pipe=3",
		"data_path=" + dataPath,
	}

	/*
	   TODO: move user and wd into env
	   if user != "" {
	       env = append(env, "user="+user)
	   }
	   if workingDir != "" {
	       env = append(env, "wd="+workingDir)
	   }
	*/

	command := exec.Command(init, append([]string{"init"}, args...)...)
	// make sure the process is executed inside the context of the rootfs
	command.Dir = rootfs
	command.Env = append(os.Environ(), env...)

	system.SetCloneFlags(command, uintptr(GetNamespaceFlags(container.Namespaces)))
	command.SysProcAttr.Pdeathsig = syscall.SIGKILL
	command.ExtraFiles = []*os.File{pipe}

	return command
}

// SetupCgroups applies the cgroup restrictions to the process running in the contaienr based
// on the container's configuration
func SetupCgroups(container *libcontainer.Container, nspid int) (cgroups.ActiveCgroup, error) {
	if container.Cgroups != nil {
		c := container.Cgroups
		if systemd.UseSystemd() {
			return systemd.Apply(c, nspid)
		}
		return fs.Apply(c, nspid)
	}
	return nil, nil
}

// InitializeNetworking creates the container's network stack outside of the namespace and moves
// interfaces into the container's net namespaces if necessary
func InitializeNetworking(ct *libct.Container, container *libcontainer.Container) error {
	context := libcontainer.Context{}
	for _, config := range container.Networks {
		strategy, err := network.GetStrategy(config.Type)
		if err != nil {
			return err
		}
		if err := strategy.Create(ct, config, context); err != nil {
			return err
		}
	}

	for _, config := range container.Routes {
		r, err := ct.AddRoute()
		if err != nil {
			return err;
		}
		if config.Destination != "" {
			r.SetDst(config.Destination)
		}
		if config.Source != "" {
			r.SetDst(config.Source)
		}
		if config.Gateway != "" || config.InterfaceName != ""{
			nh, err := r.AddNextHop()
			if err != nil {
				return err;
			}
			if config.Gateway != "" {
				nh.SetGateway(config.Gateway)
			}
			if config.InterfaceName != "" {
				nh.SetDev(config.InterfaceName)
			}
		}
	}

	return nil
}

// GetNamespaceFlags parses the container's Namespaces options to set the correct
// flags on clone, unshare, and setns
func GetNamespaceFlags(namespaces map[string]bool) (flag int) {
	for key, enabled := range namespaces {
		if enabled {
			if ns := libcontainer.GetNamespace(key); ns != nil {
				flag |= ns.Value
			}
		}
	}
	return flag
}
