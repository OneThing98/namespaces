package namespaces

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	libcontainer "github.com/OneThing98/containerpkg"
)

func ContainerExec(container *libcontainer.Container) error {
	// set namespace flags for process isolation (PID, UTS, IPC, etc.)
	flags := unix.CLONE_NEWPID | unix.CLONE_NEWUTS | unix.CLONE_NEWNS | unix.CLONE_NEWIPC | unix.SIGCHLD

	// fork the new process using clone
	pid, _, errno := unix.RawSyscall(unix.SYS_CLONE, uintptr(flags), 0, 0)
	if errno != 0 {
		return fmt.Errorf("unix clone failed: %v", errno)
	}

	if pid == 0 {
		// This is the child process
		fmt.Println("Child process created")
		// Change hostname as part of the UTS namespace isolation
		if err := unix.Sethostname([]byte(container.ID)); err != nil {
			return fmt.Errorf("failed to set hostname: %v", err)
		}

		// Set up root filesystem (pivot_root)
		if err := setupRootFilesystem(container); err != nil {
			return fmt.Errorf("failed to setup rootfs: %v", err)
		}

		// Execute the command in the container
		if err := unix.Exec(container.Command.Args[0], container.Command.Args, os.Environ()); err != nil {
			return fmt.Errorf("failed to exec command: %v", err)
		}

		// This should never be reached
		os.Exit(1)
	} else if pid > 0 {
		// This is the parent process
		// Wait for the child to complete
		var ws unix.WaitStatus
		_, err := unix.Wait4(int(pid), &ws, 0, nil)
		if err != nil {
			return fmt.Errorf("error waiting for process: %v", err)
		}

		// Handle the exit code
		if ws.Exited() {
			fmt.Printf("Process exited with code %d\n", ws.ExitStatus())
		} else {
			fmt.Printf("Process terminated abnormally\n")
		}
	} else {
		// In case of error during process creation
		return fmt.Errorf("fork failed: %v", errno)
	}

	return nil
}

func SetupRootFilesystem(container *libcontainer.Container) error {
	rootfs := container.RootFs

	// Ensure the new root filesystem exists
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		return fmt.Errorf("root filesystem does not exist: %v", rootfs)
	}

	// Bind mount the rootfs to itself, making it private
	if err := unix.Mount(rootfs, rootfs, "bind", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %v", err)
	}

	// Create a directory for the old root inside the new root
	putOld := filepath.Join(rootfs, ".pivot_root")
	if err := os.MkdirAll(putOld, 0700); err != nil {
		return fmt.Errorf("failed to create pivot_root directory: %v", err)
	}

	// Perform pivot_root: move the root filesystem to the new root
	if err := unix.PivotRoot(rootfs, putOld); err != nil {
		return fmt.Errorf("pivot_root failed: %v", err)
	}

	// Change the current working directory to "/"
	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root: %v", err)
	}

	// Unmount the old root (now at /.pivot_root)
	putOld = "/.pivot_root"
	if err := unix.Unmount(putOld, unix.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount old root: %v", err)
	}

	// Remove the old root directory
	if err := os.RemoveAll(putOld); err != nil {
		return fmt.Errorf("failed to remove old root directory: %v", err)
	}

	return nil
}
