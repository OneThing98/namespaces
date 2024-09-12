package namespaces

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	container "github.com/OneThing98/containerpkg"
)

func addEnvIfNotSet(container *container.Container, key, value string) {
	jv := fmt.Sprintf("%s=%s", key, value)
	if len(container.Command.Env) == 0 {
		container.Command.Env = []string{jv}
		return
	}

	for _, v := range container.Command.Env {
		parts := strings.Split(v, "=")
		if parts[0] == key {
			return
		}
	}

	container.Command.Env = append(container.Command.Env, jv)
}

func writeError(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	os.Exit(1)
}

func setupEnvironment(container *container.Container) {
	addEnvIfNotSet(container, "container", "docker")
	addEnvIfNotSet(container, "TERM", "xterm")
	addEnvIfNotSet(container, "USER", "root")
	addEnvIfNotSet(container, "LOGNAME", "root")
}

func setupUser(container *container.Container) error {
	if err := setgroups(nil); err != nil {
		return err
	}
	if err := setresgid(0, 0, 0); err != nil {
		return err
	}
	if err := setresuid(0, 0, 0); err != nil {
		return err
	}
	return nil
}

func getNsFds(container *container.Container) ([]uintptr, error) {
	var (
		namespaces = []string{}
		fds        = []uintptr{}
	)

	for _, ns := range container.Namespaces {
		namespaces = append(namespaces, namespaceFileMap[ns])
	}

	for _, ns := range namespaces {
		fd, err := getNsFd(container.NSPid, ns)
		if err != nil {
			for _, fd = range fds {
				syscall.Close(int(fd))
			}
			return nil, err
		}
		fds = append(fds, fd)
	}
	return fds, nil
}

func getNsFd(pid int, ns string) (uintptr, error) {
	nspath := filepath.Join("/proc", strconv.Itoa(pid), "ns", ns)
	f, err := os.OpenFile(nspath, os.O_RDONLY, 0666)
	if err != nil {
		return 0, err
	}
	return f.Fd(), nil
}

func getMasterAndConsole(container *container.Container) (string, *os.File, error) {
	master, err := openptmx()
	if err != nil {
		return "", nil, err
	}

	console, err := ptsname(master)
	if err != nil {
		master.Close()
		return "", nil, err
	}
	return console, master, nil
}
