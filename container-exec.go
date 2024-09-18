package namespaces

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/OneThing98/capabilities"
	container "github.com/OneThing98/containerpkg"
	"github.com/OneThing98/utils"

	"golang.org/x/sys/unix"
)

// "github.com/OneThing98/Ghost/pkg/capabilities"
// "github.com/OneThing98/Ghost/pkg/container"
// "github.com/OneThing98/Ghost/pkg/utils"

var (
	ErrExistingNetworkNamespace = errors.New("specified both CLONE_NEWNET and an existing network namespace")
)

// spawns new namespaces and runs the specified containerized process.
func ContainerExec(container *container.Container) (pid int, err error) {
	if container.NetNsFd > 0 && container.Namespaces.Contains("CLONE_NEWNET") {
		return -1, ErrExistingNetworkNamespace
	}

	rootfs, err := resolveRootfs(container)
	if err != nil {
		return -1, err
	}

	master, console, err := createMasterAndConsole()
	if err != nil {
		return -1, err
	}

	logger, err := os.OpenFile("/root/logs", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		return -1, err
	}
	log.SetOutput(logger)

	flag := getNamespaceFlags(container.Namespaces) | unix.CLONE_VFORK | int(unix.SIGCHLD)

	if pid, err = clone(uintptr(flag)); err != nil {
		return -1, fmt.Errorf("Error cloning process: %v", err)
	}

	if pid == 0 {
		if err := closeMasterAndStd(master); err != nil {
			writeError("close master and std: %v", err)
		}
		slave, err := openTerminal(console, unix.O_RDWR)
		if err != nil {
			writeError("open terminal :%v", err)
		}
		if err := dupSlave(slave); err != nil {
			writeError("dup2 slave: %v", err)
		}
		if container.NetNsFd > 0 {
			if err := joinExistingNamespace(container.NetNsFd, "CLONE_NEWNET"); err != nil {
				writeError("Join existing net namespace: %v", err)
			}
		}
		if _, err := setsid(); err != nil {
			writeError("setsid: %v", err)
		}

		if err := setctty(); err != nil {
			writeError("setctty: %v", err)
		}

		if err := parentDeathSignal(); err != nil {
			writeError("parent death signal: %v", err)
		}

		if err := SetUpNewMountNameSpace(rootfs, console, container.ReadonlyFs); err != nil {
			writeError("setup mount namespace %s", err)
		}

		//do not include chroot part yet

		if err := sethostname(container.ID); err != nil {
			writeError("sethostname: %v", err)
		}

		if err := capabilities.DropCapabilities(container); err != nil {
			writeError("drop capabilities: %v", err)
		}

		if err := setupUser(container); err != nil {
			writeError("setup user: %v", err)
		}

		if container.WorkingDir != "" {
			if err := chdir(container.WorkingDir); err != nil {
				writeError("chdir to %s: %v", container.WorkingDir, err)
			}
		}

		if err := exec(container.Command.Args[0], container.Command.Args, container.Command.Env); err != nil {
			writeError("exec: %v", err)
		}
		panic("unreachable")

	}

	//handle master slave pty communication for the container
	go func() {
		if _, err := io.Copy(os.Stdout, master); err != nil {
			log.Println(err)
		}
	}()

	go func() {
		if _, err := io.Copy(master, os.Stdin); err != nil {
			log.Println(err)
		}
	}()
	return pid, nil
}

// spawns a new command inside an existing container's namespace
func ContainerExecIn(container *container.Container, cmd *container.Command) (int, error) {
	if container.NSPid <= 0 {
		return -1, errors.New("invalid container PID")
	}

	//get namespace fds
	fds, err := getNsFds(container)
	if err != nil {
		return -1, err
	}

	//add network namespace fd(if applicable)
	if container.NetNsFd > 0 {
		fds = append(fds, container.NetNsFd)
	}

	pid, err := fork()
	if err != nil {
		for _, fd := range fds {
			unix.Close(int(fd))
		}
		return -1, err
	}

	//in the child process
	if pid == 0 {
		for _, fd := range fds {
			if fd > 0 {
				if err := joinExistingNamespace(fd, ""); err != nil {
					writeError("join existing namespace for fd %d: %v", fd, err)
				}
			}
			unix.Close(int(fd))
		}

		//handle remounting proc and sys
		if container.Namespaces.Contains("CLONE_NEWNS") &&
			container.Namespaces.Contains("CLONE_NEWPID") {
			child, err := fork()
			if err != nil {
				writeError("fork child: %v", err)
			}
			//in the grandchild process
			if child == 0 {
				if err := unshare(unix.CLONE_NEWNS); err != nil {
					writeError("unshare newns: %v", err)
				}
				if err := remountProc(); err != nil {
					writeError("remount proc: %v", err)
				}
				if err := remountSys(); err != nil {
					writeError("remount sys: %v", err)
				}

				if err := capabilities.DropCapabilities(container); err != nil {
					writeError("drop caps: %v", err)
				}
				if err := exec(cmd.Args[0], cmd.Args, cmd.Env); err != nil {
					writeError("exec: %v", err)
				}
				panic("unreachable")
			}

			exit, err := utils.WaitOnPid(child)
			if err != nil {
				writeError("wait on child: %v", err)
			}
			os.Exit(exit)
		}
		if err := exec(cmd.Args[0], cmd.Args, cmd.Env); err != nil {
			writeError("exec: %v", err)
		}
		panic("unrecheable")
	}
	return pid, err
}

func ResolveRootfs(container *container.Container) (string, error) {
	rootfs, err := filepath.Abs(container.RootFs)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(rootfs)
}

func CreateMasterAndConsole() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}

	console, err := ptsname(master)
	if err != nil {
		return nil, "", err
	}

	if err := unlockpt(master); err != nil {
		return nil, "", err
	}

	return master, console, nil
}

func CloseMasterAndStd(master *os.File) error {
	closefd(master.Fd())
	closefd(0)
	closefd(1)
	closefd(2)
	return nil
}

func OpenTerminal(name string, flag int) (*os.File, error) {
	r, e := unix.Open(name, flag, 0)

	if e != nil {
		return nil, &os.PathError{"open", name, e}
	}

	return os.NewFile(uintptr(r), name), nil
}

func DupSlave(slave *os.File) error {
	//It means that stdout (fd 1) and stderr (fd 2) are now pointing to the same file (the slave PTY) as slave.Fd().
	if slave.Fd() != 0 {
		return fmt.Errorf("slave fd not 0:%d", slave.Fd())
	}
	if err := dup2(slave.Fd(), 1); err != nil {
		return err
	}
	if err := dup2(slave.Fd(), 2); err != nil {
		return nil
	}
	return nil
}
