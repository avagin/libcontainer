package integration

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/configs"
)

func TestExecPS(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	config := newTemplateConfig(rootfs)
	buffers, exitCode, err := runContainer(config, "", "ps")
	if err != nil {
		t.Fatal(err)
	}

	if exitCode != 0 {
		t.Fatalf("exit code not 0. code %d stderr %q", exitCode, buffers.Stderr)
	}

	lines := strings.Split(buffers.Stdout.String(), "\n")
	if len(lines) < 2 {
		t.Fatalf("more than one process running for output %q", buffers.Stdout.String())
	}
	expected := `1 root     ps`
	actual := strings.Trim(lines[1], "\n ")
	if actual != expected {
		t.Fatalf("expected output %q but received %q", expected, actual)
	}
}

func TestIPCPrivate(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	l, err := os.Readlink("/proc/1/ns/ipc")
	if err != nil {
		t.Fatal(err)
	}

	config := newTemplateConfig(rootfs)
	buffers, exitCode, err := runContainer(config, "", "readlink", "/proc/self/ns/ipc")
	if err != nil {
		t.Fatal(err)
	}

	if exitCode != 0 {
		t.Fatalf("exit code not 0. code %d stderr %q", exitCode, buffers.Stderr)
	}

	if actual := strings.Trim(buffers.Stdout.String(), "\n"); actual == l {
		t.Fatalf("ipc link should be private to the conatiner but equals host %q %q", actual, l)
	}
}

func TestIPCHost(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	l, err := os.Readlink("/proc/1/ns/ipc")
	if err != nil {
		t.Fatal(err)
	}

	config := newTemplateConfig(rootfs)
	i := getNamespaceIndex(config, "NEWIPC")
	config.Namespaces = append(config.Namespaces[:i], config.Namespaces[i+1:]...)
	buffers, exitCode, err := runContainer(config, "", "readlink", "/proc/self/ns/ipc")
	if err != nil {
		t.Fatal(err)
	}

	if exitCode != 0 {
		t.Fatalf("exit code not 0. code %d stderr %q", exitCode, buffers.Stderr)
	}

	if actual := strings.Trim(buffers.Stdout.String(), "\n"); actual != l {
		t.Fatalf("ipc link not equal to host link %q %q", actual, l)
	}
}

func TestIPCJoinPath(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	l, err := os.Readlink("/proc/1/ns/ipc")
	if err != nil {
		t.Fatal(err)
	}

	config := newTemplateConfig(rootfs)
	i := getNamespaceIndex(config, "NEWIPC")
	config.Namespaces[i].Path = "/proc/1/ns/ipc"

	buffers, exitCode, err := runContainer(config, "", "readlink", "/proc/self/ns/ipc")
	if err != nil {
		t.Fatal(err)
	}

	if exitCode != 0 {
		t.Fatalf("exit code not 0. code %d stderr %q", exitCode, buffers.Stderr)
	}

	if actual := strings.Trim(buffers.Stdout.String(), "\n"); actual != l {
		t.Fatalf("ipc link not equal to host link %q %q", actual, l)
	}
}

func TestIPCBadPath(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	config := newTemplateConfig(rootfs)
	i := getNamespaceIndex(config, "NEWIPC")
	config.Namespaces[i].Path = "/proc/1/ns/ipcc"

	_, _, err = runContainer(config, "", "true")
	if err == nil {
		t.Fatal("container succeded with bad ipc path")
	}
}

func TestRlimit(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	config := newTemplateConfig(rootfs)
	out, _, err := runContainer(config, "", "/bin/sh", "-c", "ulimit -n")
	if err != nil {
		t.Fatal(err)
	}
	if limit := strings.TrimSpace(out.Stdout.String()); limit != "1024" {
		t.Fatalf("expected rlimit to be 1024, got %s", limit)
	}
}

func getNamespaceIndex(config *configs.Config, name string) int {
	for i, v := range config.Namespaces {
		if v.Name == name {
			return i
		}
	}
	return -1
}

func newTestRoot() (string, error) {
	dir, err := ioutil.TempDir("", "libcontainer")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func TestEnter(t *testing.T) {
	root, err := newTestRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	config := newTemplateConfig(rootfs)

	factory, err := libcontainer.New(root, []string{os.Args[0], "init", "--"})
	if err != nil {
		t.Fatal(err)
	}

	container, err := factory.Create("test", config)
	if err != nil {
		t.Fatal(err)
	}
	defer container.Destroy()

	// Execute a first process in the container
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	pconfig := libcontainer.ProcessConfig{
		Args:  []string{"sh", "-c", "cat && readlink /proc/self/ns/pid"},
		Stdin: stdinR,
		Stdout: stdoutW,
	}
	pid, err := container.StartProcess(&pconfig)
	stdinR.Close()
	stdoutW.Close()
	defer stdinW.Close()
	defer stdoutR.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Execute a process in the container
	stdinR2, stdinW2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR2, stdoutW2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	pconfig.Args = []string{"sh", "-c", "cat && readlink /proc/self/ns/pid"}
	pconfig.Stdin = stdinR2
	pconfig.Stdout = stdoutW2

	pid2, err := container.StartProcess(&pconfig)
	stdinR2.Close()
	stdoutW2.Close()
	defer stdinW2.Close()
	defer stdoutR2.Close()
	if err != nil {
		t.Fatal(err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}

	process2, err := os.FindProcess(pid2)
	if err != nil {
		t.Fatal(err)
	}

	// Check that both processes live in the same pidns
	var buf = make([]byte, 64)

	stdinW2.Close()
	n, err := stdoutR2.Read(buf)
	pidns2 := string(buf[0:n])
	if err != nil {
		t.Fatal(err)
	}

	stdinW.Close()
	n, err = stdoutR.Read(buf)
	pidns := string(buf[0:n])
	if err != nil {
		t.Fatal(err)
	}

	if pidns != pidns2 {
		t.Fatal("The second process isn't in the required pid namespace %s %s", pidns, pidns2)
	}

	s, err := process.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if !s.Success() {
		t.Fatal(s.String())
	}

	s, err = process2.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if !s.Success() {
		t.Fatal(s.String())
	}
}
