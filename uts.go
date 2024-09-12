package namespaces

import "syscall"

func SetupUTSNamespace() *syscall.SysProcAttr {
    return &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWUTS,
    }
}
