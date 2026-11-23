package audit

import (
	"bytes"
	"encoding/binary"
	"os"
	"slices"

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

// Holders lists the current user's processes, other than this one, whose
// environment sets envName. It compares only the "envName=" prefix and never
// copies the value.
func Holders(envName string) ([]Process, error) {
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil, err
	}
	prefix := []byte(envName + "=")
	var holders []Process
	for _, info := range infos {
		pid := int(info.Proc.P_pid)
		if pid == os.Getpid() {
			continue
		}
		args, env := processArgsAndEnv(pid)
		if !slices.ContainsFunc(env, func(field []byte) bool { return bytes.HasPrefix(field, prefix) }) {
			continue
		}
		holders = append(holders, Process{
			PID:  pid,
			Comm: unix.ByteSliceToString(info.Proc.P_comm[:]),
			Argv: cutArgs(RedactArgv(toStrings(args), nil)),
		})
	}
	slices.SortFunc(holders, func(left, right Process) int { return left.PID - right.PID })
	return holders, nil
}

func processArgv(pid int) []string {
	args, _ := processArgsAndEnv(pid)
	if args == nil {
		return nil
	}
	return toStrings(args)
}

// processArgsAndEnv reads kern.procargs2: int32 argc, the exec path, NUL
// padding, argc NUL-terminated arguments, then the environment strings.
// Another user's process yields nil.
func processArgsAndEnv(pid int) (args, env [][]byte) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return nil, nil
	}
	argc := int(binary.NativeEndian.Uint32(raw[:4]))
	fields := bytes.Split(raw[4:], []byte{0})[1:]
	for len(fields) > 0 && len(fields[0]) == 0 {
		fields = fields[1:]
	}
	argc = min(argc, len(fields))
	return fields[:argc], fields[argc:]
}

func toStrings(fields [][]byte) []string {
	out := make([]string, len(fields))
	for index, field := range fields {
		out[index] = string(field)
	}
	return out
}

func cutArgs(argv []string) []string {
	cut := make([]string, len(argv))
	for index, arg := range argv {
		cut[index] = cutArg(arg)
	}
	return cut
}

func cutArg(arg string) string {
	return arg[:min(len(arg), maxArgBytes)]
}
