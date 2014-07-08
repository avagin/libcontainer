package network

import (
	"fmt"
	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/utils"
	libct "github.com/avagin/libct/go"
)

// Veth is a network strategy that uses a bridge and creates
// a veth pair, one that stays outside on the host and the other
// is placed inside the container's namespace
type Veth struct {
}

const defaultDevice = "eth0"

func (v *Veth) Create(ct *libct.Container, n *libcontainer.Network, context libcontainer.Context) error {
	var (
		bridge string
		prefix string
		exists bool
	)
	if bridge, exists = n.Context["bridge"]; !exists {
		return fmt.Errorf("bridge does not exist in network context")
	}
	if prefix, exists = n.Context["prefix"]; !exists {
		return fmt.Errorf("veth prefix does not exist in network context")
	}
	name1, err := utils.GenerateRandomName(prefix, 4)
	if err != nil {
		return err
	}
	context["veth-host"] = name1
	context["veth-child"] = defaultDevice

	dev, err := ct.AddNetVeth(name1, defaultDevice)
	if err != nil {
		return err
	}

	if err := dev.SetMtu(n.Mtu); err != nil {
		return err;
	}

	if err := dev.SetMtu(n.Mtu); err != nil {
		return err;
	}

	if err := dev.AddIpAddr(n.Address); err != nil {
		return err;
	}

	host_dev, err := dev.GetPeer()
	if err != nil {
		return err
	}

	if err := host_dev.SetMaster(bridge); err != nil {
		return err;
	}

	if n.Gateway != "" {
		r, err := ct.AddRoute()
		if err != nil {
			return err
		}
		r.SetDst("default")
		r.SetDev(defaultDevice)
		nh, err := r.AddNextHop()
		if err != nil {
			return err
		}
		nh.SetGateway(n.Gateway)
	}

	return nil
}

func (v *Veth) Initialize(config *libcontainer.Network, context libcontainer.Context) error {
	return nil
}
