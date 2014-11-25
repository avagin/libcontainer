// +build !cgo !linux

package libcontainer

import (
	"fmt"
	"github.com/Sirupsen/logrus"
)

// New returns a linux based container factory based in the root directory.
func LibctNew(root string, logger *logrus.Logger) (Factory, error) {
	return nil, fmt.Errorf("Not supported")
}
