//go:build windows

package daemon

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// InspectProcess fails closed when the image, owner or creation time cannot be
// read. PIDAlive alone cannot distinguish another process after PID reuse.
func InspectProcess(pid int) (ProcessIdentity, error) {
	if pid <= 0 {
		return ProcessIdentity{}, fmt.Errorf("invalid pid %d", pid)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ProcessIdentity{}, err
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return ProcessIdentity{}, err
	}
	if code != stillActive {
		return ProcessIdentity{}, fmt.Errorf("pid %d has exited", pid)
	}
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(h, 0, &image[0], &size); err != nil {
		return ProcessIdentity{}, err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return ProcessIdentity{}, err
	}
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return ProcessIdentity{}, err
	}
	defer token.Close()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return ProcessIdentity{}, err
	}
	return ProcessIdentity{
		PID: pid, Exe: windows.UTF16ToString(image[:size]),
		Owner:   tokenUser.User.Sid.String(),
		StartID: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime),
	}, nil
}
