package namespaces

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	TIOCGPTN   = 0x80045430 //used to get slave pty number associated with the master pty
	TIOCSPTLCK = 0x40045431 //used to lock or unlock the slave pty to control access to it
)

// changes root directory of the calling process to the one specified in the path
func chroot(dir string) error {
	return unix.Chroot(dir)
}

// changes working directory of the calling process
func chdir(dir string) error {
	return unix.Chdir(dir)
}

// replaces the current process with a new one
func exec(cmd string, args []string, env []string) error {
	return unix.Exec(cmd, args, env)
}

// forks a new process from the system
func fork() (int, error) {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	pid, _, err := unix.RawSyscall(unix.SYS_FORK, 0, 0, 0)
	if err != 0 {
		return -1, fmt.Errorf("syscall fork: %v", err)
	}

	return int(pid), nil
}

// creates a new process but unlike fork() does not copy the memory pages of the parent process
func vfork() (int, error) {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	pid, _, err := unix.RawSyscall(unix.SYS_VFORK, 0, 0, 0)
	if err != 0 {
		return -1, fmt.Errorf("vfork failed: %v", err)
	}
	return int(pid), nil
}

// mounts a file system
func mount(source, target, fstype string, flags uintptr, data string) error {
	return unix.Mount(source, target, fstype, flags, data)
}

// unmounts a file system
func unmount(target string, flags int) error {
	return unix.Unmount(target, flags)
}

// creates a new namespace
func unshare(flags int) error {
	_, _, errno := unix.RawSyscall(unix.SYS_UNSHARE, uintptr(flags), 0, 0)
	if errno != 0 {
		return fmt.Errorf("syscall unshare: %v", errno)
	}
	return nil
}

// joins an existing namespace
func setns(fd, nstype uintptr) error {
	_, _, errno := unix.RawSyscall(unix.SYS_SETNS, fd, nstype, 0)
	if errno != 0 {
		return fmt.Errorf("syscall setns: %v", errno)
	}
	return nil
}

// moves the root file system to the specified directory
func pivotRoot(newRoot, putold string) error {
	return unix.PivotRoot(newRoot, putold)
}

// fork is used for processes while clone is used for threads inside those processes
func clone(flags uintptr) (int, error) {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	pid, _, err := unix.RawSyscall(unix.SYS_CLONE, flags, 0, 0)
	if err != 0 {
		return -1, fmt.Errorf("clone failed: %v", err)
	}
	return int(pid), nil
}

//setting close-on-exec flag ensures that file descriptors are not inherited by a new process image when the old one is repalced by a family system call such as execve

// clears close-on-exec flag for the file descriptor
func unsetCloseOnExec(fd uintptr) error {
	if _, _, err := unix.Syscall(unix.SYS_FCNTL, fd, unix.F_SETFD, 0); err != 0 {
		return fmt.Errorf("fcntl failed: %v", err)
	}
	return nil
}

// sets the list of supplementary group ids for the calling process
func setgroups(gids []int) error {
	return unix.Setgroups(gids)
}

// sets real, effective and saved group IDs of the calling process
func setresgid(rgid, egid, sgid int) error {
	return unix.Setresgid(rgid, egid, sgid)
}

// sets real effective and saved user IDs of the calling process
func setresuid(ruid, euid, suid int) error {
	return unix.Setresuid(ruid, euid, suid)
}

// sets hostname of the calling process
func sethostname(name string) error {
	return unix.Sethostname([]byte(name))
}

// creates a new session and sets the calling process as the session leader
func setsid() (int, error) {
	return unix.Setsid()
}

// performs a device specific input output operations
func ioctl(fd uintptr, flag, data uintptr) error {
	if _, _, err := unix.Syscall(unix.SYS_IOCTL, fd, flag, data); err != 0 {
		return fmt.Errorf("ioctl failed: %v", err)
	}
	return nil
}

// opens psuedo terminal master device
func openptmx() (*os.File, error) {
	return os.OpenFile("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
}

// unlocks a pseudo-terminal master device
func unlockpt(f *os.File) error {
	var u int
	return ioctl(f.Fd(), TIOCSPTLCK, uintptr(unsafe.Pointer(&u)))
}

// returns the pathname of the slave pseudo terminal device corresponding to the master device
func ptsname(f *os.File) (string, error) {
	var n int
	if err := ioctl(f.Fd(), TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// closes a file descriptor
func closefd(fd uintptr) error {
	return unix.Close(int(fd))
}

// duplicates a file descriptor
func dup2(fd1, fd2 uintptr) error {
	return unix.Dup2(int(fd1), int(fd2))
}

// creates a file system node
func mknod(path string, mode uint32, dev int) error {
	return unix.Mknod(path, mode, dev)
}

// sets the parent process death signal for the calling process
func parentDeathSignal() error {
	if _, _, err := unix.Syscall6(unix.SYS_PRCTL, unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0, 0); err != 0 {
		return fmt.Errorf("prctl failed: %v", err)
	}
	return nil
}

// sets the controlling terminal for the calling process
func setctty() error {
	if _, _, err := unix.Syscall(unix.SYS_IOCTL, 0, uintptr(unix.TIOCSCTTY), 0); err != 0 {
		return fmt.Errorf("ioctl failed: %v", err)
	}
	return nil
}

// mkfifo creates a named pipe special file
func mkfifo(name string, mode uint32) error {
	return unix.Mkfifo(name, mode)
}

// umask sets the file mode creating mask
func umask(mask int) int {
	return unix.Umask(mask)
}
