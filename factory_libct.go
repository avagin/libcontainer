// +build libct

package libcontainer

import (
	"fmt"
	"os"
	"path/filepath"

	libct "github.com/avagin/libct/go"
	"github.com/docker/libcontainer/configs"
)

type libctFactory struct {
	session *libct.Session
	root    string
}

func NewLibctFactory(root string) (Factory, error) {
	f := libctFactory{
		root: root,
	}

	if err := f.init(); err != nil {
		return nil, err
	}

	return &f, nil
}

func (f *libctFactory) init() error {
	if f.session != nil {
		return nil
	}

	s := &libct.Session{}
	err := s.OpenLocal()
	if err != nil {
		return err
	}

	f.session = s
	return nil
}

func (f *libctFactory) Create(id string, config *configs.Config) (Container, error) {
	if f.root == "" {
		return nil, newGenericError(fmt.Errorf("invalid root"), ConfigInvalid)
	}
	containerRoot := filepath.Join(f.root, id)
	if _, err := os.Stat(containerRoot); err == nil {
		return nil, newGenericError(fmt.Errorf("Container with id exists: %v", id), IdInUse)
	} else if !os.IsNotExist(err) {
		return nil, newGenericError(err, SystemError)
	}
	if err := os.MkdirAll(containerRoot, 0700); err != nil {
		return nil, newGenericError(err, SystemError)
	}

	ct, err := f.session.ContainerCreate(id)
	if err != nil {
		return nil, err
	}

	return &libctContainer{
		id:      id,
		root:    containerRoot,
		config:  config,
		ct:      ct,
		session: f.session,
	}, nil
}

func (f *libctFactory) Load(id string) (Container, error) {
	containerRoot := filepath.Join(f.root, id)
	_, err := os.Stat(containerRoot)
	if err != nil {
		return nil, err
	}
	panic("not implemented")
}

// StartInitialization loads a container by opening the pipe fd from the parent to read the configuration and state
// This is a low level implementation detail of the reexec and should not be consumed externally
func (f *libctFactory) StartInitialization(pipefd uintptr) (err error) {
	return nil
}

func (l *libctFactory) Type() string {
	return "libct"
}
