package namespaces

import (
	container "github.com/OneThing98/containerpkg"
)

const (
	CLONE_NEWNS   = 0x00020000 // new mount namespace
	CLONE_NEWUTS  = 0x04000000 // new UTS namespace
	CLONE_NEWIPC  = 0x08000000 // new IPC namespace
	CLONE_NEWUSER = 0x10000000 // new user namespace
	CLONE_NEWPID  = 0x20000000 // new PID namespace
	CLONE_NEWNET  = 0x40000000 // new network namespace
)

var namespaceMap = map[container.Namespace]int{
	"mnt":  CLONE_NEWNS,
	"uts":  CLONE_NEWUTS,
	"ipc":  CLONE_NEWIPC,
	"user": CLONE_NEWUSER,
	"pid":  CLONE_NEWPID,
	"net":  CLONE_NEWNET,
}

// /proc file system file names corresponding to the namespaces.
var namespaceFileMap = map[container.Namespace]string{
	"mnt":  "mnt",
	"uts":  "uts",
	"ipc":  "ipc",
	"user": "user",
	"pid":  "pid",
	"net":  "net",
}
