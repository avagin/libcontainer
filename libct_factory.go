package libcontainer

import (
	"github.com/Sirupsen/logrus"
	libct "github.com/avagin/libct/go"
)

type libctFactory struct {
	initArgs []string
	logger   *logrus.Logger
	session  *libct.Session
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

func (f *libctFactory) Create(id string, config *Config) (Container, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	return newLibctContainer(id, config, f)
}

func (f *libctFactory) Load(path string) (Container, error) {
	panic("not implemented")
}

// StartInitialization loads a container by opening the pipe fd from the parent to read the configuration and state
// This is a low level implementation detail of the reexec and should not be consumed externally
func (f *libctFactory) StartInitialization(pipefd uintptr) (err error) {
	return nil
}
