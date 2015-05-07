// +build libct

package integration

import (
	"github.com/docker/libcontainer"
	"testing"
)

func libctRun(m *testing.M) int {
	var err error

	factory, err = libcontainer.NewLibctFactory(".", false)
	if err != nil {
		panic(err)
	}

	libct = true
	defer func() {
		libct = false
	}()
	return m.Run()
}
