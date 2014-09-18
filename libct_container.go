// +build linux

package libcontainer

import (
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"

	libct "github.com/avagin/libct/go"
	"github.com/docker/libcontainer/cgroups"
	"github.com/docker/libcontainer/cgroups/fs"
	"github.com/docker/libcontainer/cgroups/systemd"
	"github.com/docker/libcontainer/devices"
	"github.com/docker/libcontainer/label"
	"github.com/docker/libcontainer/mount"
	"github.com/docker/libcontainer/network"
)

// this is to enforce that the libctContainer conforms to the Container interface at compile time
var _ Container = (*libctContainer)(nil)

// libctContainer represents a container that can be executed on linux based host machines
type libctContainer struct {
	mux sync.Mutex

	// path to the containers state directory
	path string

	// initial (immutable) config for the container
	config *Config

	// containers state for the lifetime of the container
	state *State

	// a map of commands in the order which they were created
	processes map[int]*Process

	// active cgroup to cleanup
	activeCgroup cgroups.ActiveCgroup

	logger *log.Logger

	container *libct.Container
}

func newLibctContainer(config *Config, state *State, logger *log.Logger, ct *libct.Container) *libctContainer {
	return &libctContainer{
		config:    config,
		state:     state,
		logger:    logger,
		processes: make(map[int]*Process),
		container: ct,
	}
}

// Path returns the path to the container's directory containing the state
func (c *libctContainer) Path() string {
	return c.path
}

// Config returns the initial configuration for the container that was used
// during initializtion of the container
func (c *libctContainer) Config() *Config {
	return c.config
}

// Status returns the containers current status
func (c *libctContainer) Status() Status {
	return c.state.Status
}

// Stats returns the container's statistics for various cgroup subsystems
func (c *libctContainer) Stats() (*ContainerStats, error) {
	var (
		err            error
		containerStats = &ContainerStats{}
	)

	c.logger.Printf("reading stats for container: %s\n", c.path)

	if containerStats.CgroupStats, err = fs.GetStats(c.config.Cgroups); err != nil {
		return containerStats, err
	}

	if containerStats.NetworkStats, err = network.GetStats(&c.state.NetworkState); err != nil {
		return containerStats, err
	}

	return containerStats, nil
}

// Start runs a new process in the container
func (c *libctContainer) Start(process *Process) (pid int, exitChan chan int, err error) {
	c.logger.Printf("starting new process in container: %s\n", c.path)

	panic("not implemented")
}

// Destroy kills all running process inside the container and cleans up any
// state left on the filesystem
func (c *libctContainer) Destroy() error {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.logger.Printf("destroying container: %s\n", c.path)

	c.state.Status = Destroyed

	return c.activeCgroup.Cleanup()
}

// Processes return the PIDs for processes running inside the container
func (c *libctContainer) Processes() ([]int, error) {
	panic("not implemented")
}

// Pause pauses all processes inside the container
func (c *libctContainer) Pause() error {
	c.mux.Lock()
	defer c.mux.Unlock()

	if err := c.changeStatus(Pausing); err != nil {
		return err
	}

	if err := c.toggleCgroupFreezer(cgroups.Frozen); err != nil {
		return err
	}

	if err := c.changeStatus(Paused); err != nil {
		return err
	}

	return nil
}

// Resume unpause all processes inside the container
func (c *libctContainer) Resume() error {
	c.mux.Lock()
	defer c.mux.Unlock()

	if err := c.changeStatus(Resuming); err != nil {
		return err
	}

	if err := c.toggleCgroupFreezer(cgroups.Thawed); err != nil {
		return err
	}

	if err := c.changeStatus(Running); err != nil {
		return err
	}

	return nil
}

// changeStatus changes the container's current status to s
// if the state change is not allowed a StateError is returned
//
// This method depends on the caller to hold any locks related to the
// container's state
func (c *libctContainer) changeStatus(s Status) error {

	c.logger.Printf("container %s changing status from %s to %s\n", c.path, c.state.Status, s)

	c.state.Status = s

	return nil
}

func (c *libctContainer) toggleCgroupFreezer(state cgroups.FreezerState) (err error) {
	if systemd.UseSystemd() {
		c.logger.Printf("container %s modifying freezer state to %s with systemd\n", c.path, state)

		err = systemd.Freeze(c.config.Cgroups, state)
	} else {
		c.logger.Printf("container %s modifying freezer state to %s with fs\n", c.path, state)

		err = fs.Freeze(c.config.Cgroups, state)
	}

	return err
}

func setupBindmounts(ct *libct.Container, rootfs string, mountConfig *MountConfig) error {
	bindMounts := mountConfig.Mounts

	for _, m := range bindMounts.OfType("bind") {
		var flags int

		if !m.Writable {
			flags |= libct.CT_FS_RDONLY
		}

		if m.Private {
			flags |= libct.CT_FS_PRIVATE
		}

		if err := ct.AddBindMount(m.Source, m.Destination, flags); err != nil {
			return err
		}
	}
	return nil
}

// Create the device nodes in the container.
func createDeviceNodes(ct *libct.Container, rootfs string, nodesToCreate []*devices.Device) error {

	for _, node := range nodesToCreate {
		if err := createDeviceNode(ct, rootfs, node); err != nil {
			return err
		}
	}
	return nil
}

// Creates the device node in the rootfs of the container.
func createDeviceNode(ct *libct.Container, rootfs string, node *devices.Device) error {
	fileMode := node.FileMode
	switch node.Type {
	case 'c':
		fileMode |= syscall.S_IFCHR
	case 'b':
		fileMode |= syscall.S_IFBLK
	default:
		return fmt.Errorf("%c is not a valid device type for device %s", node.Type, node.Path)
	}

	if err := ct.AddDeviceNode(node.Path, int(fileMode), int(node.MajorNumber), int(node.MinorNumber)); err != nil {
		return fmt.Errorf("mknod %s %s", node.Path, err)
	}
	return nil
}

type mount_entry struct {
	source string
	path   string
	device string
	flags  int
	data   string
}

// mountSystem sets up linux specific system mounts like sys, proc, shm, and devpts
// inside the mount namespace
func mountSystem(ct *libct.Container, rootfs string, mountConfig *MountConfig) error {
	for _, m := range newSystemMounts(mountConfig.MountLabel, mountConfig.Mounts) {
		if err := ct.AddMount(m.source, m.path, m.flags, m.device, m.data); err != nil {
			return fmt.Errorf("mounting %s into %s %s", m.source, m.path, err)
		}
	}
	return nil
}

// default mount point flags
const defaultMountFlags = libct.CT_FS_NOEXEC | libct.CT_FS_NOSUID | libct.CT_FS_NODEV

// TODO: this is crappy right now and should be cleaned up with a better way of handling system and
// standard bind mounts allowing them to be more dynamic
func newSystemMounts(mountLabel string, mounts mount.Mounts) []mount_entry {
	systemMounts := []mount_entry{
		{source: "proc", path: "/proc", device: "proc", flags: defaultMountFlags},
		{source: "sysfs", path: "/sys", device: "sysfs", flags: defaultMountFlags},
		{source: "tmpfs", path: "/dev", device: "tmpfs", flags: libct.CT_FS_NOSUID | libct.CT_FS_STRICTATIME, data: label.FormatMountLabel("mode=755", mountLabel)},
		{source: "tmpfs", path: "/sys/fs/cgroup", device: "tmpfs", flags: defaultMountFlags},
		{source: "shm", path: "/dev/shm", device: "tmpfs", flags: defaultMountFlags, data: label.FormatMountLabel("mode=1777,size=65536k", mountLabel)},
		{source: "devpts", path: "/dev/pts", device: "devpts", flags: libct.CT_FS_NOSUID | libct.CT_FS_NOEXEC, data: label.FormatMountLabel("newinstance,ptmxmode=0666,mode=620,gid=5", mountLabel)},
	}

	return systemMounts
}

func SetupPtmx(ct *libct.Container, rootfs, consolePath string) error {
	if consolePath == "" {
		return nil
	}

	dest := "/dev/console"

	if err := ct.AddBindMount(consolePath, dest, 0); err != nil {
		return fmt.Errorf("bind %s to %s %s", consolePath, dest, err)
	}
	return nil
}

// InitializeMountNamespace setups up the devices, mount points, and filesystems for use inside a
// new mount namepsace
func (c *libctContainer) initializeMountNamespace(console string) error {
	var (
		err error
	)

	rootfs := c.config.Rootfs
	ct := c.container

	if err := mountSystem(c.container, c.config.Rootfs, c.config.MountConfig); err != nil {
		return fmt.Errorf("mount system %s", err)
	}

	if err = c.container.SetRoot(c.config.Rootfs); err != nil {
		return fmt.Errorf("bind mounts %s", err)
	}

	if err := setupBindmounts(c.container, c.config.Rootfs, c.config.MountConfig); err != nil {
		return err
	}
	if err := createDeviceNodes(c.container, c.config.Rootfs, c.config.MountConfig.DeviceNodes); err != nil {
		return fmt.Errorf("create device nodes %s", err)
	}

	if err := SetupPtmx(ct, rootfs, console); err != nil {
		return err
	}
	return nil
}

func (c *libctContainer) startInitProcess(process *Process) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	process.exitChan = make(chan int, 1)

	// because this is our init process we can alwasy set it to 1
	c.processes[1] = process

	c.logger.Printf("container %s starting init process\n", c.path)

	err := c.container.SetNsMask(uint64(getNamespaceFlags(c.config.Namespaces)))
	if err != nil {
		return err
	}

	if err := c.initializeMountNamespace(process.ConsolePath); err != nil {
		return err
	}

	var fds *[3]uintptr
	if process.ConsolePath != "" {
		ttyfd, err := os.OpenFile(process.ConsolePath, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		fds = &[3]uintptr{ttyfd.Fd(), ttyfd.Fd(), ttyfd.Fd()}

		err = c.container.SetConsoleFd(ttyfd)
		if err != nil {
			return err
		}
	} // FIXME proxy pipes

	pid, err := c.container.SpawnExecve(process.Args[0], process.Args, process.Env, fds)
	if err != nil {
		return err
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	process.cmd.Process = p

	process.pipe.CloseChild()

	startTime, err := process.startTime()
	if err != nil {
		c.container.Kill()

		return err
	}

	// update state
	c.state.InitPid = process.pid()
	c.state.InitStartTime = startTime

	c.logger.Printf("container %s init process started at %s with pid %d\n", c.path, c.state.InitStartTime, c.state.InitPid)

	if err := c.changeStatus(Running); err != nil {
		process.kill()

		return err
	}

	c.logger.Printf("container %s waiting on init process\n", c.path)

	// finally the users' process should be running inside the container and we did not encounter
	// any errors during the init of the namespace.  we can now wait on the process and return
	go func() {
		process.wait()
		c.container.Wait()
	}()

	return nil
}
