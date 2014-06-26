package network

import (
	"errors"

	"github.com/docker/libcontainer"
	libct "github.com/avagin/libct/go"
)

var (
	ErrNotValidStrategyType = errors.New("not a valid network strategy type")
)

var strategies = map[string]NetworkStrategy{
	"veth":     &Veth{},
	"loopback": &Loopback{},
	"netns":    &NetNS{},
}

// NetworkStrategy represents a specific network configuration for
// a container's networking stack
type NetworkStrategy interface {
	Create(*libct.Container, *libcontainer.Network, libcontainer.Context) error
	Initialize(*libcontainer.Network, libcontainer.Context) error
}

// GetStrategy returns the specific network strategy for the
// provided type.  If no strategy is registered for the type an
// ErrNotValidStrategyType is returned.
func GetStrategy(tpe string) (NetworkStrategy, error) {
	s, exists := strategies[tpe]
	if !exists {
		return nil, ErrNotValidStrategyType
	}
	return s, nil
}
