package namespaces

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	defaults = unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV
)

func SetUpNewMountNameSpace(rootfs, console string, readonly bool) error {
	if err := mount("", "/", "", unix.MS_SLAVE|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("mounting / as slave %s", err)
	}
	if err := mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("mounting %s as bind %s", rootfs, err)
	}
	if readonly {
		if err := mount(rootfs, rootfs, "bind", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY|syscall.MS_REC, ""); err != nil {
			return fmt.Errorf("mounting %s as readonly %s", rootfs, err)
		}
	}
	if err := mountSystem(rootfs); err != nil {
		return fmt.Errorf("mount system %s", err)
	}
	if err := copyDevNodes(rootfs); err != nil {
		return fmt.Errorf("copy dev nodes %s", err)
	}
	ptmx := filepath.Join(rootfs, "dev/ptmx")
	if err := os.Remove(ptmx); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(filepath.Join(rootfs, "pts/ptmx"), ptmx); err != nil {
		return fmt.Errorf("symlink dev ptmx %s", err)
	}

	if err := setupDev(rootfs); err != nil {
		return err
	}

	if err := setupConsole(rootfs, console); err != nil {
		return err
	}

	if err := chdir(rootfs); err != nil {
		return fmt.Errorf("chdir into %s %s", rootfs, err)
	}

	if err := mount(rootfs, "/", "", syscall.MS_MOVE, ""); err != nil {
		return fmt.Errorf("mount move %s into / %s", rootfs, err)
	}

	if err := chroot("."); err != nil {
		return fmt.Errorf("chroot . %s", err)
	}

	if err := chdir("/"); err != nil {
		return fmt.Errorf("chdir / %s", err)
	}

	umask(0022)

	return nil

}

func copyDevNodes(rootfs string) error {
	umask(0000)

	for _, node := range []string{
		"null",
		"zero",
		"full",
		"random",
		"urandom",
		"tty",
	} {
		stat, err := os.Stat(filepath.Join("/dev", node))
		if err != nil {
			return err
		}

		var (
			dest = filepath.Join(rootfs, "dev", node)
			st   = stat.Sys().(*unix.Stat_t)
		)

		log.Printf("copy %s to %s %d\n", node, dest, st.Rdev)
		if err := mknod(dest, st.Mode, int(st.Rdev)); err != nil && !os.IsExist(err) {
			return fmt.Errorf("copy %s %s", node, err)
		}
	}
	return nil
}

func setupDev(rootfs string) error {
	for _, link := range []struct {
		from string
		to   string
	}{
		{"/proc/kcore", "/dev/core"},
		{"/proc/self/fd", "/dev/fd"},
		{"/proc/self/fd/0", "/dev/stdin"},
		{"/proc/self/fd/1", "/dev/stdout"},
		{"/proc/self/fd/2", "/dev/stderr"},
	} {
		dest := filepath.Join(rootfs, link.to)
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s %s", dest, err)
		}
		if err := os.Symlink(link.from, dest); err != nil {
			return fmt.Errorf("symlink %s %s", dest, err)
		}
	}
	return nil
}

func setupConsole(rootfs, console string) error {
	umask(000)
	stat, err := os.Stat(console)
	if err != nil {
		return fmt.Errorf("stat console %s %s", console, err)
	}
	st := stat.Sys().(*syscall.Stat_t)

	dest := filepath.Join(rootfs, "dev/console")

	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s %s", dest, err)
	}
	if err := os.Chmod(console, 0600); err != nil {
		return err
	}
	if err := os.Chown(console, 0, 0); err != nil {
		return err
	}
	if err := mknod(dest, (st.Mode&^07777)|0600, int(st.Rdev)); err != nil {
		return fmt.Errorf("mknod %s %s", dest, err)
	}
	return nil

}

func mountSystem(rootfs string) error {
	mounts := []struct {
		source string
		path   string
		device string
		flags  int
		data   string
	}{
		{source: "proc", path: filepath.Join(rootfs, "proc"), device: "proc", flags: defaults},
		{source: "sysfs", path: filepath.Join(rootfs, "sys"), device: "sysfs", flags: defaults},
		{source: "tmpfs", path: filepath.Join(rootfs, "dev"), device: "tmpfs", flags: syscall.MS_NOSUID | syscall.MS_STRICTATIME, data: "mode=755"},
		{source: "shm", path: filepath.Join(rootfs, "dev", "shm"), device: "tmpfs", flags: defaults, data: "mode=1777"},
		{source: "devpts", path: filepath.Join(rootfs, "dev", "pts"), device: "devpts", flags: syscall.MS_NOSUID | syscall.MS_NOEXEC, data: "newinstance,ptmxmode=0666,mode=620,gid=5"},
		{source: "tmpfs", path: filepath.Join(rootfs, "run"), device: "tmpfs", flags: syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_STRICTATIME, data: "mode=755"},
	}

	for _, m := range mounts {
		if err := os.MkdirAll(m.path, 0755); err != nil && !os.IsExist(err) {
			return fmt.Errorf("mkdirall %s %s", m.path, err)
		}
		if err := mount(m.source, m.path, m.device, uintptr(m.flags), m.data); err != nil {
			return fmt.Errorf("mounting %s into %s %s", m.source, m.path, err)
		}
	}
	return nil
}

func remountProc() error {
	if err := unmount("/proc", unix.MNT_DETACH); err != nil {
		return err
	}
	if err := mount("proc", "/proc", "proc", uintptr(defaults), ""); err != nil {
		return err
	}
	return nil
}

func remountSys() error {
	if err := unmount("/sys", unix.MNT_DETACH); err != nil {
		if err != unix.EINVAL {
			return err
		}
	} else {
		if err := mount("sysfs", "/sys", "sysfs", uintptr(defaults), ""); err != nil {
			return err
		}
	}
	return nil
}

// func SetUpNewMountNameSpace(rootfs string, console string, readonly bool) error {

// 	//ensures mount events do not propagate outwards to other namespaces but propagate inwards
// 	if err := unix.Mount("", "/", "", unix.MS_SLAVE|unix.MS_REC, ""); err != nil {
// 		return fmt.Errorf("mounting / as slave: %v", err)
// 	}

// 	//bind mounts the new root filesystem onto itself to prepare for the new environment

// 	if err := unix.Mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REC, ""); err != nil {
// 		return fmt.Errorf("binding mount: %s, %v", rootfs, err)
// 	}

// 	//if the read only flag is true then new root filesystem remounts as read-only
// 	if readonly {
// 		if err := unix.Mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_REC, ""); err != nil {
// 			return fmt.Errorf("remounting %s as readonly: %v", rootfs, err)
// 		}
// 	}

// 	//custom wrapper around pivotRoot system call
// 	if err := pivotRoot(rootfs, filepath.Join(rootfs, ".old_root")); err != nil {
// 		return fmt.Errorf("pivot_root %s: %v", rootfs, err)
// 	}

// 	//unmount old root

// 	if err := unix.Unmount("/.old_root", unix.MNT_DETACH); err != nil {
// 		return fmt.Errorf("unmounting old root: %v", err)
// 	}

// 	return os.Chdir("/")

// }
