// +build linux

package libcontainer

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/docker/libcontainer/cgroups"
	"github.com/docker/libcontainer/configs"
	_ "github.com/docker/libcontainer/namespaces/nsenter"
)

type mockCgroupManager struct {
	pids  []int
	stats *cgroups.Stats
}

func (m *mockCgroupManager) GetPids() ([]int, error) {
	return m.pids, nil
}

func (m *mockCgroupManager) GetStats() (*cgroups.Stats, error) {
	return m.stats, nil
}

func TestGetContainerPids(t *testing.T) {
	container := &linuxContainer{
		id:            "myid",
		config:        &configs.Config{},
		cgroupManager: &mockCgroupManager{pids: []int{1, 2, 3}},
	}

	pids, err := container.Processes()
	if err != nil {
		t.Fatal(err)
	}

	for i, expected := range []int{1, 2, 3} {
		if pids[i] != expected {
			t.Fatalf("expected pid %d but received %d", expected, pids[i])
		}
	}
}

func TestGetContainerStats(t *testing.T) {
	container := &linuxContainer{
		id:     "myid",
		config: &configs.Config{},
		cgroupManager: &mockCgroupManager{
			pids: []int{1, 2, 3},
			stats: &cgroups.Stats{
				MemoryStats: cgroups.MemoryStats{
					Usage: 1024,
				},
			},
		},
		state: &configs.State{},
	}

	stats, err := container.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.CgroupStats == nil {
		t.Fatal("cgroup stats are nil")
	}
	if stats.CgroupStats.MemoryStats.Usage != 1024 {
		t.Fatalf("expected memory usage 1024 but recevied %d", stats.CgroupStats.MemoryStats.Usage)
	}
}

func TestFactoryCreateContainer(t *testing.T) {
	root, err := newTestRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// setup default container config and state for mocking
	var (
		id     = "2"
		config = &configs.Config{
			RootFs: "/",
			Env:    []string{"PATH=/bin:/usr/bin"},
			Namespaces: []configs.Namespace{
				configs.Namespace{Name: "NEWPID"},
			},
			MountConfig: &configs.MountConfig{},
		}
	)

	factory, err := New(root, []string{os.Args[0], "init", "--"})
	if err != nil {
		t.Fatal(err)
	}

	container, err := factory.Create(id, config)
	if err != nil {
		t.Fatal(err)
	}
	defer container.Destroy()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	pconfig := ProcessConfig{
		Args:  []string{"cat"},
		Stdin: r,
	}
	pid, err := container.StartProcess(&pconfig)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}

	cr, cw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	pconfig.Args = []string{"cat"}
	pconfig.Stdin = cr
	cpid, err := container.StartProcess(&pconfig)
	cr.Close()
	if err != nil {
		t.Fatal(err)
	}
	process, err := os.FindProcess(cpid)
	if err != nil {
		t.Fatal(err)
	}

	pidns, err := os.Stat(fmt.Sprintf("/proc/%d/ns/pid", pid))
	if err != nil {
		t.Fatal(err)
	}

	cpidns, err := os.Stat(fmt.Sprintf("/proc/%d/ns/pid", cpid))
	if err != nil {
		t.Fatal(err)
	}

	if os.SameFile(pidns, cpidns) {
		t.Fatal("The second process isn't in the required pid namespace");
	}

	cw.Close()

	s, err := process.Wait()
	w.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !s.Success() {
		t.Fatal(s.String())
	}

	process, err = os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}

	s, err = process.Wait()
	w.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !s.Success() {
		t.Fatal(s.String())
	}
}

// init runs the libcontainer initialization code because of the busybox style needs
// to work around the go runtime and the issues with forking
func init() {
	if len(os.Args) < 2 || os.Args[1] != "init" {
		return
	}
	runtime.LockOSThread()

	factory, err := New("", nil)
	if err != nil {
		os.Exit(1)
	}

	factory.StartInitialization(3)

	os.Exit(1)
}
