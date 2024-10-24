package namespaces

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	libcontainer "github.com/OneThing98/containerpkg"
)

func JoinExistingNamespace(fd uintptr, ns libcontainer.Namespace) error {
	if err := unix.Setns(int(fd), 0); err != nil {
		return fmt.Errorf("failed to join existing namespace: %v", err)
	}
	fmt.Printf("Successfully joined %s namespace. \n", ns)
	return nil
}

func ContainerExec(container *libcontainer.Container) error {
	flags := unix.CLONE_NEWPID | unix.CLONE_NEWUTS | unix.CLONE_NEWNS | unix.CLONE_NEWIPC | unix.SIGCHLD

	// if container.NetNsFd > 0 {
	// 	if err := JoinExistingNamespace(container.NetNsFd, libcontainer.CLONE_NEWNET); err != nil {
	// 		return fmt.Errorf("failed to join existing namespace: %v", err)
	// 	}
	// 	flags &= ^unix.CLONE_NEWNET
	// }

	pid, _, errno := unix.RawSyscall(unix.SYS_CLONE, uintptr(flags), 0, 0)
	if errno != 0 {
		return fmt.Errorf("unix clone failed: %v", errno)
	}

	if pid == 0 {
		// this is the child process
		fmt.Println("Child process created")
		if err := unix.Sethostname([]byte(container.ID)); err != nil {
			return fmt.Errorf("failed to set hostname: %v", err)
		}

		if err := SetupRootFilesystem(container); err != nil {
			return fmt.Errorf("failed to setup rootfs: %v", err)
		}
		//setup terminal handling for the container
		master, console, err := createMasterAndConsole()
		if err != nil {
			return fmt.Errorf("failed to create console, %v", err)
		}

		//close master and std fd for the container process
		if err := closeMasterAndStd(master); err != nil {
			return fmt.Errorf("failed to close master and std: %v", err)
		}

		//open slave terminal
		slave, err := openTerminal(console, unix.O_RDWR)
		if err != nil {
			return fmt.Errorf("failed to open slave terminal: %v", err)
		}

		//duplicate slave to stdout and stderr
		if err := dupSlave(slave); err != nil {
			return fmt.Errorf("failed to duplicate slave: %v", err)
		}

		//setup /dev/console inside the container
		if err := setupConsole(container.RootFs, console); err != nil {
			return fmt.Errorf("failed to setup console: %v", err)
		}

		fmt.Printf("Attempting to exec command: %s with args: %v\n", container.Command.Args[0], container.Command.Args)
		if err := unix.Exec(container.Command.Args[0], container.Command.Args, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "failed to exec command %s: %v\n", container.Command.Args[0], err)
			os.Exit(1)
		}

		// this should never be reached
		os.Exit(1)
	} else if pid > 0 {
		// this is the parent process
		// wait for the child to complete
		var ws unix.WaitStatus
		_, err := unix.Wait4(int(pid), &ws, 0, nil)
		if err != nil {
			return fmt.Errorf("error waiting for process: %v", err)
		}

		if ws.Exited() {
			fmt.Printf("Process exited with code %d\n", ws.ExitStatus())
		} else {
			fmt.Printf("Process terminated abnormally\n")
		}
	} else {
		return fmt.Errorf("fork failed: %v", errno)
	}

	return nil
}

func createMasterAndConsole() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	console, err := PTSName(master)
	if err != nil {
		return nil, "", err
	}
	if err := UnlockPT(master); err != nil {
		return nil, "", err
	}
	return master, console, nil
}

func closeMasterAndStd(master *os.File) error {
	if err := unix.Close(int(master.Fd())); err != nil {
		return fmt.Errorf("failed to close master: %v", err)
	}
	if err := unix.Close(0); err != nil {
		return fmt.Errorf("failed to close stdin: %v", err)
	}
	if err := unix.Close(1); err != nil {
		return fmt.Errorf("failed to close stdout: %v", err)
	}
	if err := unix.Close(2); err != nil {
		return fmt.Errorf("failed to close stderr: %v", err)
	}
	return nil
}

func openTerminal(name string, flag int) (*os.File, error) {
	r, e := unix.Open(name, flag, 0)
	if e != nil {
		return nil, &os.PathError{"open", name, e}
	}
	return os.NewFile(uintptr(r), name), nil
}

func dupSlave(slave *os.File) error {
	if err := unix.Dup2(int(slave.Fd()), 1); err != nil {
		return err
	}
	if err := unix.Dup2(int(slave.Fd()), 2); err != nil {
		return err
	}
	return nil
}

func setupConsole(rootfs, console string) error {
	stat, err := os.Stat(console)
	if err != nil {
		return fmt.Errorf("stat console %s: %v", console, err)
	}
	st := stat.Sys().(*unix.Stat_t)
	dest := filepath.Join(rootfs, "dev/console")

	if err := os.Remove(dest); err != nil && os.IsNotExist(err) {
		return fmt.Errorf("remove old console: %v", err)
	}
	if err := os.Chmod(console, 0600); err != nil {
		return fmt.Errorf("chmod console: %v", err)
	}

	if err := unix.Mknod(dest, st.Mode, int(st.Rdev)); err != nil {
		return fmt.Errorf("mknod console: %v", err)
	}
	return nil
}

func SetupRootFilesystem(container *libcontainer.Container) error {
	rootfs := container.RootFs

	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		return fmt.Errorf("root filesystem does not exist: %v", rootfs)
	}

	if err := unix.Mount("", "/", "", unix.MS_PRIVATE|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make / a private mount: %v", err)
	}
	if err := unix.Mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %v", err)
	}

	putOld := filepath.Join(rootfs, ".pivot_root")
	if err := os.MkdirAll(putOld, 0700); err != nil {
		return fmt.Errorf("failed to create pivot_root directory: %v", err)
	}

	if err := unix.Chdir(rootfs); err != nil {
		return fmt.Errorf("failed to chdir to new root: %v", err)
	}

	if err := unix.PivotRoot(rootfs, putOld); err != nil {
		return fmt.Errorf("pivot_root failed: %v", err)
	}

	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root after pivot_root: %v", err)
	}

	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %v", err)
	}

	// error on dev pts no such file or directory
	if err := unix.Mount("devpts", "/dev/pts", "devpts", 0, ""); err != nil {
		return fmt.Errorf("failed to mount devpts: %v", err)
	}
	//uncomment this code and try running again

	putOld = "/.pivot_root"
	if err := unix.Unmount(putOld, unix.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount old root: %v", err)
	}

	if err := os.RemoveAll(putOld); err != nil {
		return fmt.Errorf("failed to remove old root directory: %v", err)
	}

	return nil
}
