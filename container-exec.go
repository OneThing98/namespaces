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

	if container.NetNsFd > 0 {
		if err := joinExistingNamespace(container.NetNsFd, libcontainer.CLONE_NEWNET); err != nil {
			return fmt.Errorf("failed to join existing namespace: %v", err)
		}
		flags &= ^unix.CLONE_NEWNET
	}

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

		if err := unix.Exec(container.Command.Args[0], container.Command.Args, os.Environ()); err != nil {
			return fmt.Errorf("failed to exec command: %v", err)
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

	putOld = "/.pivot_root"
	if err := unix.Unmount(putOld, unix.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount old root: %v", err)
	}

	if err := os.RemoveAll(putOld); err != nil {
		return fmt.Errorf("failed to remove old root directory: %v", err)
	}

	return nil
}
