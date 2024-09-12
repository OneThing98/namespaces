package namespaces

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	container "github.com/OneThing98/containerpkg"

	"golang.org/x/sys/unix"
)

func SetupNamespaces() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
	}
}

func CreateNewNamespace(namespace container.Namespace, bindTo string) error {
	flag := namespaceMap[namespace]
	name := namespaceFileMap[namespace]
	nspath := filepath.Join("/proc/self/ns", name)

	pid, err := fork()
	if err != nil {
		return err
	}

	if pid == 0 {
		if err := unshare(flag); err != nil {
			return fmt.Errorf("unshare %s: %v", namespace, err)
		}
		if err := unix.Mount(nspath, bindTo, "none", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind mount %s: %v", nspath, err)
		}
		os.Exit(0)
	}

	_, err = unix.Wait4(pid, nil, 0, nil)
	return err
}

func getNamespaceFlags(namespaces container.Namespaces) (flag int) {
	for _, ns := range namespaces {
		flag |= namespaceMap[ns]
	}
	return
}
func joinExistingNamespace(fd uintptr, namespace container.Namespace) error {
	flag := namespaceMap[namespace]
	return setns(fd, uintptr(flag))
}
