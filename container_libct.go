// +build linux

package libcontainer

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"syscall"

	log "github.com/Sirupsen/logrus"
	libct "github.com/avagin/libct/go"
	"github.com/docker/libcontainer/configs"
	"github.com/docker/libcontainer/system"
	"github.com/docker/libcontainer/utils"
)

// this is to enforce that the libctContainer conforms to the Container interface at compile time
var _ Container = (*libctContainer)(nil)

// libctContainer represents a container that can be executed on linux based host machines
type libctContainer struct {
	id   string
	root string

	m sync.Mutex

	// initial (immutable) config for the container
	config *configs.Config

	ct *libct.Container

	session *libct.Session

	initProcess *Process
}

// ID returns the container's unique ID
func (c *libctContainer) ID() string {
	return c.id
}

// Config returns the initial configuration for the container that was used
// during initializtion of the container
func (c *libctContainer) Config() configs.Config {
	return *c.config
}

// Status returns the containers current status
func (c *libctContainer) Status() (Status, error) {
	if c.initProcess == nil {
		return Destroyed, nil
	}
	return Running, nil
}

func (c *libctContainer) State() (*State, error) {
	status, err := c.Status()
	if err != nil {
		return nil, err
	}
	if status == Destroyed {
		return nil, newGenericError(fmt.Errorf("container destroyed"), ContainerNotExists)
	}
	pid, err := c.initProcess.Pid()
	if err != nil {
		return nil, newSystemError(err)
	}
	startTime, err := system.GetProcessStartTime(pid)
	if err != nil {
		return nil, err
	}
	state := &State{
		ID:                   c.ID(),
		Config:               *c.config,
		InitProcessPid:       pid,
		InitProcessStartTime: startTime,
		NamespacePaths:       make(map[configs.NamespaceType]string),
	}
	for _, ns := range c.config.Namespaces {
		state.NamespacePaths[ns.Type] = ns.GetPath(pid)
	}
	return state, nil
}

// Destroy kills all running process inside the container and cleans up any
// state left on the filesystem
func (c *libctContainer) Destroy() error {
	c.m.Lock()
	defer c.m.Unlock()

	// FIXME c.ct.Status() != Destroyed
	c.ct.Wait()

	if err := os.RemoveAll(c.root); err == nil {
		return err
	}

	return nil
}

// Processes return the PIDs for processes running inside the container
func (c *libctContainer) Processes() ([]int, error) {
	return c.ct.Processes()
}

// Pause pauses all processes inside the container
func (c *libctContainer) Pause() error {
	panic("not implemented")
}

// Resume unpause all processes inside the container
func (c *libctContainer) Resume() error {
	panic("not implemented")
}

// getEnabledCapabilities returns the capabilities that should not be dropped by the container.
func getEnabledCapabilities(capList []string) (uint64, error) {
	var keep uint64 = 0
	for _, c := range capList {
		v, ok := capabilityList[c]
		if !ok {
			return 0, newGenericError(fmt.Errorf("unknown capability %q", c), ConfigInvalid)
		}
		keep |= uint64(v)
	}
	return keep, nil
}

func dropBoundingSet(ct *libct.ProcessDesc, capabilities []string) error {
	caps, err := getEnabledCapabilities(capabilities)
	if err != nil {
		return err
	}

	if err := ct.SetCaps(caps, libct.CAPS_BSET); err != nil {
		return newSystemError(err)
	}

	return nil
}

type libctProcessOps struct {
	p *libct.ProcessDesc
}

func (p *libctProcessOps) pid() int {
	pid, err := p.p.GetPid()
	if err != nil {
		log.Warn(err)
		pid = math.MinInt32
	}
	return pid
}

func (p *libctProcessOps) wait() (*os.ProcessState, error) {
	return p.p.Wait()
}

func (p *libctProcessOps) signal(sig os.Signal) error {
	process, err := os.FindProcess(p.pid())
	if err != nil {
		return err
	}

	return process.Signal(sig)
}

func (c *libctContainer) Start(process *Process) error {
	c.m.Lock()
	defer c.m.Unlock()

	pd, err := c.session.ProcessCreateDesc()
	if err != nil {
		return newSystemError(err)
	}

	if err := pd.SetParentDeathSignal(syscall.SIGKILL); err != nil {
		return newSystemError(err)
	}

	if err := dropBoundingSet(pd, c.config.Capabilities); err != nil {
		return err
	}

	if err := pd.SetEnv(process.Env); err != nil {
		return err
	}

	pd.Stdin = process.Stdin
	pd.Stdout = process.Stdout
	pd.Stderr = process.Stderr

	if c.initProcess != nil {
		err = c.ct.EnterExecve(pd, process.Args[0], process.Args, nil)
		if err != nil {
			return newSystemError(err)
		}

		process.ops = &libctProcessOps{p: pd}

		return nil
	}

	err = c.ct.SetNsMask(uint64(c.config.Namespaces.CloneFlags()))
	if err != nil {
		return newSystemError(err)
	}

	if err := c.setupMountNamespace(); err != nil {
		return err
	}

	if err := c.setupNetwork(); err != nil {
		return fmt.Errorf("setup networking %s", err)
	}

	if err = c.setupCgroups(); err != nil {
		return newSystemError(err)
	}

	if process.consolePath != "" {
		pd.Stdin = libct.Console
		pd.Stdout = libct.Console
		pd.Stderr = libct.Console
		err = c.ct.SetConsoleFd(libct.Console)
		if err != nil {
			return newSystemError(err)
		}
	}

	err = c.ct.SpawnExecve(pd, process.Args[0], process.Args, process.Env)
	if err != nil {
		return newSystemError(err)
	}

	process.ops = &libctProcessOps{p: pd}

	c.initProcess = process

	return nil
}

func (c *libctContainer) setupNetwork() error {
	for _, config := range c.config.Networks {
		strategy, err := libctGetStrategy(config.Type)
		if err != nil {
			return err
		}

		err = strategy.create(c.ct, (*configs.Network)(config))
		if err != nil {
			return err
		}
	}

	return nil
}

var libctFlagsMap map[int]int = map[int]int{
	syscall.MS_NOEXEC:      libct.CT_FS_NOEXEC,
	syscall.MS_NOSUID:      libct.CT_FS_NOSUID,
	syscall.MS_NODEV:       libct.CT_FS_NODEV,
	syscall.MS_STRICTATIME: libct.CT_FS_STRICTATIME,
	syscall.MS_RDONLY:      libct.CT_FS_RDONLY,
}

func libctFlags(flags int) (int, error) {
	var libctFlags int = 0

	for k, v := range libctFlagsMap {
		if k&flags != 0 {
			libctFlags |= v
			flags &= ^k
		}
	}

	if flags != 0 {
		return 0, newGenericError(fmt.Errorf("Unknown flags: %x", flags), ConfigInvalid)
	}

	return libctFlags, nil
}

func (c *libctContainer) libctMount(m *configs.Mount, mountLabel string) error {
	var flags int

	flags, err := libctFlags(m.Flags)
	if err != nil {
		return err
	}

	switch m.Device {
	case "bind":
		if err := c.ct.AddBindMount(m.Source, m.Destination, flags); err != nil {
			return newSystemError(err)
		}
		break
	default:
		if err := c.ct.AddMount(m.Source, m.Destination, flags, m.Device, m.Data); err != nil {
			return newSystemError(err)
		}
	}

	return nil
}

func (c *libctContainer) setupMountNamespace() error {
	rootfs, err := utils.ResolveRootfs(c.config.Rootfs)
	if err != nil {
		return err
	}

	if err := c.ct.SetRoot(rootfs); err != nil {
		return newSystemError(err)
	}

	for _, m := range append(baseMounts, c.config.Mounts...) {
		if err := c.libctMount(m, c.config.MountLabel); err != nil {
			return newSystemError(err)
		}
	}

	return nil
}

func (c *libctContainer) NotifyOOM() (<-chan struct{}, error) {
	panic("not implemented yet")
}

func (c *libctContainer) Set(config configs.Config) error {
	panic("not implemented yet")
}

// Stats returns the container's statistics for various cgroup subsystems
func (c *libctContainer) Stats() (*Stats, error) {
	return &Stats{}, nil //TODO
}

type libctCgroup interface {
	apply(c *libctContainer) error
}

type libctCgroupCpu struct {
}

func (cg *libctCgroupCpu) apply(c *libctContainer) error {
	var err error

	if c.config.Cgroups.CpuShares != 0 {
		err = c.ct.ConfigureController(libct.CTL_CPU, "cpu.shares",
			strconv.FormatInt(c.config.Cgroups.CpuShares, 10))
		if err != nil {
			return err
		}
	}

	if c.config.Cgroups.CpuPeriod != 0 {
		err = c.ct.ConfigureController(libct.CTL_CPU, "cpu.cfs_period_us",
			strconv.FormatInt(c.config.Cgroups.CpuPeriod, 10))
		if err != nil {
			return err
		}
	}

	if c.config.Cgroups.CpuQuota != 0 {
		err = c.ct.ConfigureController(libct.CTL_CPU, "cpu.cfs_quota_us",
			strconv.FormatInt(c.config.Cgroups.CpuQuota, 10))
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *libctContainer) setupCgroups() error {
	cgroups := map[int]libctCgroup{
		libct.CTL_CPU:     &libctCgroupCpu{},
		libct.CTL_MEMORY:  nil,
		libct.CTL_BLKIO:   nil,
		libct.CTL_FREEZER: nil,
		libct.CTL_DEVICES: nil,
	}
	for ct, cg := range cgroups {
		if err := c.ct.AddController(ct); err != nil {
			return newSystemError(err)
		}
		if cg != nil {
			if err := cg.apply(c); err != nil {
				return newSystemError(err)
			}
		}
	}

	return nil
}
