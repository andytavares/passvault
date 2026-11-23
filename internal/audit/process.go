package audit

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/sys/unix"
)

const (
	maxCallers  = 10
	maxArgBytes = 120
)

// CallerChain walks from pid up through its parents, stopping at launchd
// (pid 1), after maxCallers processes, or at a process it cannot read.
func CallerChain(pid int) []Process {
	var chain []Process
	for pid > 1 && len(chain) < maxCallers {
		info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil {
			break
		}
		chain = append(chain, Process{
			PID:  pid,
			Comm: unix.ByteSliceToString(info.Proc.P_comm[:]),
			Argv: processArgv(pid),
		})
		pid = int(info.Eproc.Ppid)
	}
	return chain
}

// processArgv reads kern.procargs2: int32 argc, the exec path, NUL padding,
// then argc NUL-terminated arguments. Another user's process yields nil.
func processArgv(pid int) []string {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return nil
	}
	argc := int(binary.NativeEndian.Uint32(raw[:4]))
	fields := bytes.Split(raw[4:], []byte{0})[1:]
	for len(fields) > 0 && len(fields[0]) == 0 {
		fields = fields[1:]
	}
	return cutArgs(fields[:min(argc, len(fields))])
}

func cutArgs(fields [][]byte) []string {
	argv := make([]string, len(fields))
	for index, field := range fields {
		argv[index] = string(field[:min(len(field), maxArgBytes)])
	}
	return argv
}
