//go:build linux

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// InspectProcess fails closed when the image, owner or creation time cannot be read.
func InspectProcess(pid int) (ProcessIdentity, error) {
	if pid <= 0 {
		return ProcessIdentity{}, fmt.Errorf("invalid pid %d", pid)
	}
	proc := filepath.Join("/proc", strconv.Itoa(pid))
	info, err := os.Stat(proc)
	if err != nil {
		return ProcessIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ProcessIdentity{}, fmt.Errorf("pid %d owner unavailable", pid)
	}
	exe, err := os.Readlink(filepath.Join(proc, "exe"))
	if err != nil {
		return ProcessIdentity{}, err
	}
	data, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return ProcessIdentity{}, err
	}
	// comm (field 2) may contain spaces and parentheses. The last closing
	// parenthesis ends it; starttime is field 22, index 19 after that point.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return ProcessIdentity{}, fmt.Errorf("pid %d has invalid proc stat", pid)
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) <= 19 || fields[0] == "Z" {
		return ProcessIdentity{}, fmt.Errorf("pid %d has exited or invalid proc stat", pid)
	}
	return ProcessIdentity{PID: pid, Exe: exe, Owner: strconv.FormatUint(uint64(stat.Uid), 10), StartID: fields[19]}, nil
}
