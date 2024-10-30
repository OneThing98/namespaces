package namespaces

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	libcontainer "github.com/OneThing98/containerpkg"
)

func JoinExistingNamespace(fd uintptr, ns libcontainer.Namespace) error {
	fmt.Printf("Attempting to join existing namespace with fd: %d\n", fd)
	if err := unix.Setns(int(fd), 0); err != nil {
		return fmt.Errorf("failed to join existing namespace: %v", err)
	}
	fmt.Printf("Successfully joined %s namespace. \n", ns)
	return nil
}

func ContainerExec(container *libcontainer.Container) error {
	flags := unix.CLONE_NEWPID | unix.CLONE_NEWUTS | unix.CLONE_NEWNS | unix.CLONE_NEWIPC | unix.SIGCHLD

	fmt.Println("Starting container exec...")
	fmt.Printf("Flags for clone: %d\n", flags)

	pid, _, errno := unix.RawSyscall(unix.SYS_CLONE, uintptr(flags), 0, 0)
	if errno != 0 {
		return fmt.Errorf("unix clone failed: %v", errno)
	}

	if pid == 0 {
		// this is the child process
		fmt.Println("Child process created")
		fmt.Printf("Container ID: %s\n", container.ID)
		if err := unix.Sethostname([]byte(container.ID)); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set hostname: %v\n", err)
			os.Exit(1)
		}

		// Move terminal handling before SetupRootFilesystem
		fmt.Println("Setting up terminal handling...")
		master, console, err := createMasterAndConsole()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create console: %v\n", err)
			os.Exit(1)
		}
		defer master.Close()

		fmt.Println("Opening slave terminal...")
		slave, err := openTerminal(console, unix.O_RDWR)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open slave terminal: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Duplicating slave to stdout and stderr...")
		if err := dupSlave(slave); err != nil {
			fmt.Fprintf(os.Stderr, "failed to duplicate slave: %v\n", err)
			os.Exit(1)
		}

		// Now set up the root filesystem
		fmt.Println("Setting up root filesystem...")
		if err := SetupRootFilesystem(container); err != nil {
			fmt.Fprintf(os.Stderr, "failed to setup rootfs: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Setting up /dev/console inside the container...")
		if err := setupConsole(container.RootFs, console); err != nil {
			fmt.Fprintf(os.Stderr, "failed to setup console: %v\n", err)
			os.Exit(1)
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
		fmt.Printf("Parent process, waiting for child pid: %d\n", pid)
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
	fmt.Println("Opening /dev/ptmx for terminal handling...")
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
	fmt.Printf("Created master and console: %s\n", console)
	return master, console, nil
}

func openTerminal(name string, flag int) (*os.File, error) {
	fmt.Printf("Opening terminal: %s\n", name)
	r, e := unix.Open(name, flag, 0)
	if e != nil {
		return nil, &os.PathError{"open", name, e}
	}
	return os.NewFile(uintptr(r), name), nil
}

func dupSlave(slave *os.File) error {
	if slave == nil {
		return fmt.Errorf("slave file descriptor is nil")
	}

	fd := slave.Fd()
	fmt.Printf("Duplicating slave to stdout (fd: %d)...\n", fd)
	if err := unix.Dup2(int(fd), 1); err != nil {
		fmt.Fprintf(os.Stderr, "failed to duplicate slave to stdout: %v (fd: %d)\n", err, fd)
		return fmt.Errorf("failed to duplicate slave to stdout: %v", err)
	}

	fmt.Printf("Duplicating slave to stderr (fd: %d)...\n", fd)
	if err := unix.Dup2(int(fd), 2); err != nil {
		fmt.Fprintf(os.Stderr, "failed to duplicate slave to stderr: %v (fd: %d)\n", err, fd)
		return fmt.Errorf("failed to duplicate slave to stderr: %v", err)
	}

	return nil
}

func setupConsole(rootfs, console string) error {
	fmt.Printf("Setting up /dev/console with console: %s in rootfs: %s\n", console, rootfs)
	stat, err := os.Stat(console)
	if err != nil {
		fmt.Printf("Stat failed for console %s: %v\n", console, err)
		return fmt.Errorf("stat console %s: %v", console, err)
	}
	st := stat.Sys().(*unix.Stat_t)
	dest := filepath.Join(rootfs, "dev/console")

	fmt.Printf("Creating /dev/console at %s\n", dest)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Failed to remove old console: %v\n", err)
		return fmt.Errorf("remove old console: %v", err)
	}
	if err := os.Chmod(console, 0600); err != nil {
		fmt.Printf("Failed to chmod console: %v\n", err)
		return fmt.Errorf("chmod console: %v", err)
	}

	if err := unix.Mknod(dest, st.Mode, int(st.Rdev)); err != nil {
		fmt.Printf("Failed to mknod console: %v\n", err)
		return fmt.Errorf("mknod console: %v", err)
	}

	fmt.Println("Successfully set up /dev/console")
	return nil
}

func SetupRootFilesystem(container *libcontainer.Container) error {
	rootfs := container.RootFs

	fmt.Printf("Setting up root filesystem: %s\n", rootfs)
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		return fmt.Errorf("root filesystem does not exist: %v", rootfs)
	}

	fmt.Println("Making / a private mount...")
	if err := unix.Mount("", "/", "", unix.MS_PRIVATE|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make / a private mount: %v", err)
	}

	fmt.Println("Bind-mounting root filesystem...")
	if err := unix.Mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %v", err)
	}

	putOld := filepath.Join(rootfs, ".pivot_root")
	if err := os.MkdirAll(putOld, 0700); err != nil {
		return fmt.Errorf("failed to create pivot_root directory: %v", err)
	}

	fmt.Println("Changing directory to new root...")
	if err := unix.Chdir(rootfs); err != nil {
		return fmt.Errorf("failed to chdir to new root: %v", err)
	}

	fmt.Println("Performing pivot_root...")
	if err := unix.PivotRoot(rootfs, putOld); err != nil {
		return fmt.Errorf("pivot_root failed: %v", err)
	}

	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root after pivot_root: %v", err)
	}

	fmt.Println("Mounting proc filesystem...")
	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %v", err)
	}

	// error on dev pts no such file or directory
	fmt.Println("Mounting devpts...")
	if err := unix.Mount("devpts", "/dev/pts", "devpts", 0, ""); err != nil {
		return fmt.Errorf("failed to mount devpts: %v", err)
	}

	putOld = "/.pivot_root"
	fmt.Println("Unmounting old root...")
	if err := unix.Unmount(putOld, unix.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount old root: %v", err)
	}

	if err := os.RemoveAll(putOld); err != nil {
		return fmt.Errorf("failed to remove old root directory: %v", err)
	}

	return nil
}
