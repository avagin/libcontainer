package integration

import (
    "io/ioutil"
    "os"
    "testing"

    "github.com/docker/libcontainer"
)

func TestCheckpoint(t *testing.T) {
    if testing.Short() {
        return
    }
    root, err := newTestRoot()
    if err != nil {
        t.Fatal(err)
    }
    defer os.RemoveAll(root)

    rootfs, err := newRootfs()
    if err != nil {
        t.Fatal(err)
    }
    defer remove(rootfs)

    config := newTemplateConfig(rootfs)

    factory, err := libcontainer.New(root, libcontainer.Cgroupfs, func(l *libcontainer.LinuxFactory) error {
        l.CriuPath = "criu"
        return nil
    })

    if err != nil {
        t.Fatal(err)
    }

    container, err := factory.Create("test", config)
    if err != nil {
        t.Fatal(err)
    }
    defer container.Destroy()

    stdinR, stdinW, err := os.Pipe()
    if err != nil {
        t.Fatal(err)
    }

    pconfig := libcontainer.Process{
        Args:  []string{"cat"},
        Env:   standardEnvironment,
        Stdin: stdinR,
    }
    err = container.Start(&pconfig)
    stdinR.Close()
    defer stdinW.Close()
    if err != nil {
        t.Fatal(err)
    }

    pid, err := pconfig.Pid()
    if err != nil {
        t.Fatal(err)
    }

    process, err := os.FindProcess(pid)
    if err != nil {
        t.Fatal(err)
    }

    imagesDir, err := ioutil.TempDir("", "criu")
    defer os.RemoveAll(imagesDir)

    checkpointOpts := &libcontainer.CriuOpts{
        ImagesDirectory: imagesDir,
    }

    if err := container.Checkpoint(checkpointOpts); err != nil {
        t.Fatal(err)
    }

    state, err := container.Status()
    if err != nil {
        t.Fatal(err)
    }

    if state != libcontainer.Checkpointed {
        t.Fatal("Unexpected state: ", state)
    }

    stdinW.Close()
    _, err = process.Wait()
    if err != nil {
        t.Fatal(err)
    }


    // reload the container
    container, err = factory.Load("test")

    stdinR, stdinW, err = os.Pipe()
    if err != nil {
        t.Fatal(err)
    }

    restoreProcessConfig := &libcontainer.Process{
        Stdin:  stdinR,
    }

    err = container.Restore(restoreProcessConfig, &libcontainer.CriuOpts{
        ImagesDirectory:         imagesDir,
    })
    stdinR.Close()
    defer stdinW.Close()

    if err != nil {
        t.Fatal(err)
    }

    state, err = container.Status()
    if err != nil {
        t.Fatal(err)
    }

    if state != libcontainer.Running {
        t.Fatal("Unexpected state: ", state)
    }

    pid, err = restoreProcessConfig.Pid()
    if err != nil {
        t.Fatal(err)
    }

    process, err = os.FindProcess(pid)
    if err != nil {
        t.Fatal(err)
    }

    stdinW.Close()
    s, err := process.Wait()
    if err != nil {
        t.Fatal(err)
    }
    if !s.Success() {
        t.Fatal(s.String())
    }
}
