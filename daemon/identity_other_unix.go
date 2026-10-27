//go:build unix && !linux

package daemon

import "errors"

func InspectProcess(int) (ProcessIdentity, error) {
	return ProcessIdentity{}, errors.New("managed process identity is supported on Windows and Linux")
}
