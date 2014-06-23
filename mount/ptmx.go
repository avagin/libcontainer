// +build linux

package mount

import (
	"fmt"
	"github.com/docker/libcontainer/console"
//	"os"
//	"path/filepath"
	libct "github.com/avagin/libct/go"
)

func SetupPtmx(ct *libct.Container, rootfs, consolePath, mountLabel string) error {
/*	ptmx := filepath.Join(rootfs, "dev/ptmx")
	if err := os.Remove(ptmx); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink("pts/ptmx", ptmx); err != nil {
		return fmt.Errorf("symlink dev ptmx %s", err)
	}
*/
	fmt.Println(consolePath);
	if consolePath != "" {
		if err := console.Setup(ct, rootfs, consolePath, mountLabel); err != nil {
			return err
		}
	}
	return nil
}
