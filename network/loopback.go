package network

import (
	"fmt"
	"github.com/docker/libcontainer"
	libct "github.com/avagin/libct/go"
)

// Loopback is a network strategy that provides a basic loopback device
type Loopback struct {
}

func (l *Loopback) Create(*libct.Container, *libcontainer.Network, libcontainer.Context) error {
	return nil
}

func (l *Loopback) Initialize(config *libcontainer.Network, context libcontainer.Context) error {
	if err := SetMtu("lo", config.Mtu); err != nil {
		return fmt.Errorf("set lo mtu to %d %s", config.Mtu, err)
	}
	if err := InterfaceUp("lo"); err != nil {
		return fmt.Errorf("lo up %s", err)
	}
	return nil
}
