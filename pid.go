package namespaces

import "syscall"

func SetupPIDNamespace() *syscall.SysProcAttr {
    return &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWPID,
    }
}
