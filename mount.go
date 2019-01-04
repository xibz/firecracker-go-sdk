package firecracker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	mntNSType = "mnt"
)

type mounter struct {
	UID    int
	GID    int
	NSType string
}

func newMounter(uid, gid int, nstype string) *mounter {
	return &mounter{
		UID:    uid,
		GID:    gid,
		NSType: nstype,
	}
}

func (m *mounter) Mount(src, target string, readOnly bool) error {
	runtime.LockOSThread()
	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("Failed to create %q: %v", target, err)
	}
	defer f.Close()

	if err := syscall.Mount(src, target, "bind", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("Failed to mount %q: %v", target, err)
	}

	if err := os.Chown(target, m.UID, m.GID); err != nil {
		return fmt.Errorf("Failed to change ownership for %q: %v", target, err)
	}

	if readOnly {
		if err := syscall.Mount(
			src,
			target,
			"bind",
			syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY,
			"",
		); err != nil {
			return fmt.Errorf("Failed to mount %q: %v", target, err)
		}
	}

	return nil
}

func (m *mounter) Close() {
	runtime.UnlockOSThread()
}

const nsFormat = "/proc/%d/ns"

func (m *mounter) EnterNS(pid int) error {
	fmt.Println("ATTEMPTING TO SET NS", pid)
	nsPath := fmt.Sprintf(nsFormat, pid)
	nstypePath := filepath.Join(nsPath, m.NSType)
	fd, err := unix.Open(nstypePath, unix.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("failed to open %s namespace: %v", m.NSType, err)
	}
	defer unix.Close(fd)

	if err := unix.Setns(fd, unix.CLONE_NEWNS); err != nil {
		return fmt.Errorf("failed to setns: %v", err)
	}

	return nil
}
