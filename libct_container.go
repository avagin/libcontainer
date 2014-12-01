// +build linux,cgo

package libcontainer

import (
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libcontainer/libct"
	_libct "github.com/docker/libcontainer/libct/libct/go"
	"github.com/docker/libcontainer/mount"
	"github.com/docker/libcontainer/namespaces/types"
	"github.com/docker/libcontainer/network"
	"github.com/docker/libcontainer/security/capabilities"
)

// this is to enforce that the libctContainer conforms to the Container interface at compile time
var _ Container = (*libctContainer)(nil)

// libctContainer represents a container that can be executed on linux based host machines
type libctContainer struct {
	mux sync.Mutex

	id string

	// path to the containers state directory
	path string

	// initial (immutable) config for the container
	config *Config

	logger *logrus.Logger

	ct *_libct.Container

	p *_libct.ProcessDesc

	rstate RunState

	// containers state for the lifetime of the container
	state *State
}

// getEnabledCapabilities returns the capabilities that should not be dropped by the container.
func getEnabledCapabilities(capList []string) uint64 {
	var keep uint64
	for _, capability := range capList {
		if c := capabilities.GetCapability(capability); c != nil {
			keep |= uint64(c.Value)
		}
	}
	return keep
}

func newLibctContainer(id string, config *Config, f *libctFactory) (*libctContainer, error) {
	ct, err := f.session.ContainerCreate(id)
	if err != nil {
		return nil, err
	}

	p, err := f.session.ProcessCreateDesc()
	if err != nil {
		return nil, err
	}

	flags := types.GetNamespaceFlags(config.Namespaces)
	if err := ct.SetNsMask(uint64(flags)); err != nil {
		return nil, err
	}

	if err := libct.InitializeMountNamespace(ct, config.RootFs, "",
		(*mount.MountConfig)(config.MountConfig)); err != nil {
		return nil, err
	}

	caps := getEnabledCapabilities(config.Capabilities)
	if err := p.SetCaps(caps, _libct.CAPS_BSET); err != nil {
		return nil, err
	}

	if err := p.SetParentDeathSignal(syscall.SIGKILL); err != nil {
		return nil, err
	}

	c := libctContainer{
		id:     id,
		config: config,
		logger: f.logger,
		ct:     ct,
		p:      p,
		rstate: Destroyed,
		state:  &State{},
	}

	c.setupNetwork()

	return &c, nil
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

// Stats returns the container's statistics for various cgroup subsystems
func (c *libctContainer) Stats() (*ContainerStats, error) {
	c.logger.Printf("reading stats for container: %s\n", c.path)

	panic("not implemented")
}

// Destroy kills all running process inside the container and cleans up any
// state left on the filesystem
func (c *libctContainer) Destroy() error {
	c.mux.Lock()
	defer c.mux.Unlock()

	if err := c.ct.Kill(); err != nil {
		return err
	}

	c.logger.Printf("destroying container: %s\n", c.path)

	c.rstate = Destroyed

	return nil
}

// Processes return the PIDs for processes running inside the container
func (c *libctContainer) Processes() ([]int, error) {
	panic("not implemented")
}

// Pause pauses all processes inside the container
func (c *libctContainer) Pause() error {
	panic("not implemented")
}

// Resume unpause all processes inside the container
func (c *libctContainer) Resume() error {
	panic("not implemented")
}

func (c *libctContainer) StartProcess(process *ProcessConfig) (int, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	var (
		pid int
		err error
	)

	if c.rstate == Destroyed {
		pid, err = c.ct.SpawnExecve(c.p, process.Args[0], process.Args, process.Env, nil)
	} else {
		pid, err = c.ct.EnterExecve(c.p, process.Args[0], process.Args, process.Env, nil)
	}
	if err != nil {
		return 0, err
	}

	c.rstate = Running
	c.logger.Printf("container %s waiting on init process\n", c.path)

	go func() {
		c.ct.Wait()
		c.changeState(Destroyed)
	}()

	return pid, err
}

func (c *libctContainer) changeState(state RunState) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.rstate = state
}

func (c *libctContainer) ID() string {
	return c.id
}

func (c *libctContainer) RunState() (RunState, error) {
	return c.rstate, nil
}

func (c *libctContainer) Signal(pid, signal int) error {
	c.logger.Debugf("sending signal %d to pid %d", signal, pid)
	panic("not implemented")
}

func (c *libctContainer) WaitProcess(pid int) (int, error) {
	c.logger.Debugf("wait process %d", pid)
	panic("not implemented")
}

func (c *libctContainer) Wait() (int, error) {
	err := c.ct.Wait()
	return 0, err
}

func (c *libctContainer) setupNetwork() error {
	for _, config := range c.config.Networks {
		c.logger.Printf("container %s creating network for %s\n", c.path, config.Type)

		strategy, err := libct.GetStrategy(config.Type)
		if err != nil {
			return err
		}

		err = strategy.Create(c.ct, (*network.Network)(config), &c.state.NetworkState)
		if err != nil {
			return err
		}
	}

	return nil
}
