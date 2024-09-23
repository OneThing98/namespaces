package namespaces

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	libcontainer "github.com/OneThing98/containerpkg"
)

func ContainerExec(container *libcontainer.Container) error {
	// set namespace flags for process isolation (PID, UTS, IPC, etc.)
	flags := unix.CLONE_NEWPID | unix.CLONE_NEWUTS | unix.CLONE_NEWNS | unix.CLONE_NEWIPC | unix.SIGCHLD

	// crreate the new process using clone
	pid, _, errno := unix.RawSyscall(unix.SYS_CLONE, uintptr(flags), 0, 0)
	if errno != 0 {
		return fmt.Errorf("unix clone failed: %v", errno)
	}

	if pid == 0 {
		// this is the child process
		if err := unix.Exec(container.Command.Args[0], container.Command.Args, os.Environ()); err != nil {
			return fmt.Errorf("failed to exec command: %v", err)
		}

		// This should never be reached
		os.Exit(1)
	} else if pid > 0 {
		// this is the parent process
		// wait for the child to complete
		var ws unix.WaitStatus
		_, err := unix.Wait4(int(pid), &ws, 0, nil)
		if err != nil {
			return fmt.Errorf("error waiting for process: %v", err)
		}

		// handle the exit code
		if ws.Exited() {
			fmt.Printf("Process exited with code %d\n", ws.ExitStatus())
		} else {
			fmt.Printf("Process terminated abnormally\n")
		}
	} else {
		// in case of error during process creation
		return fmt.Errorf("fork failed: %v", errno)
	}

	return nil
}
