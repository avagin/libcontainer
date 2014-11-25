// +build linux

package libcontainer

import (
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	_libct "github.com/avagin/libct/go"
	"github.com/docker/libcontainer/namespaces/types"
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

	state RunState
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

	if err:= p.SetParentDeathSignal(syscall.SIGKILL); err != nil {
		return nil, err
	}

	c := libctContainer{
		id:     id,
		config: config,
		logger: f.logger,
		ct:     ct,
		p:      p,
		state:  Destroyed,
	}

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

	c.state = Destroyed

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

	if c.state == Destroyed {
		pid, err = c.ct.SpawnExecve(c.p, process.Args[0], process.Args, process.Env, nil)
	} else {
		pid, err = c.ct.EnterExecve(c.p, process.Args[0], process.Args, process.Env, nil)
	}
	if err != nil {
		return 0, err
	}

	c.state = Running
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

	c.state = state
}

func (c *libctContainer) ID() string {
	return c.id
}

func (c *libctContainer) RunState() (RunState, error) {
	return c.state, nil
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
