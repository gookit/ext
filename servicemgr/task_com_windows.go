//go:build windows

package servicemgr

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Task Scheduler 2.0 COM vtable indices are from the Windows SDK taskschd.h.
// Calling COM directly avoids a PowerShell/schtasks runtime dependency and
// locale-dependent output parsing.
const (
	iTaskServiceGetFolder = 7
	iTaskServiceConnect   = 10
	iTaskFolderGetTask    = 13
	iTaskFolderDeleteTask = 15
	iTaskFolderRegister   = 16
	iRegisteredState      = 9
	iRegisteredRun        = 12
	iRegisteredXML        = 20
	taskCreate            = 2
	taskUpdate            = 4
	taskValidateOnly      = 1
	taskLogonInteractive  = 3
	vtBSTR                = 8
)

// COM class/interface IDs and the OLE entry points are exposed as functions rather
// than package variables so the package keeps no mutable global state.
func clsidTaskScheduler() windows.GUID {
	return windows.GUID{Data1: 0x0F87369F, Data2: 0xA4E5, Data3: 0x4CFC, Data4: [8]byte{0xBD, 0x3E, 0x73, 0xE6, 0x15, 0x45, 0x72, 0xDD}}
}

func iidTaskService() windows.GUID {
	return windows.GUID{Data1: 0x2FABA4C7, Data2: 0x4DA9, Data3: 0x4013, Data4: [8]byte{0x96, 0x97, 0x20, 0xCC, 0x3F, 0xD4, 0x0F, 0x85}}
}

func procCoCreateInstance() *windows.LazyProc {
	return windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
}

func procSysAllocStringLen() *windows.LazyProc {
	return windows.NewLazySystemDLL("oleaut32.dll").NewProc("SysAllocStringLen")
}

func procSysFreeString() *windows.LazyProc {
	return windows.NewLazySystemDLL("oleaut32.dll").NewProc("SysFreeString")
}

func procSysStringLen() *windows.LazyProc {
	return windows.NewLazySystemDLL("oleaut32.dll").NewProc("SysStringLen")
}

type comObject struct{ vtable *[32]uintptr }

type comVariant struct {
	Type uint16
	_    [6]byte
	Data uintptr
	_    [8]byte
}

func comMethod(obj *comObject, index uintptr, args ...uintptr) error {
	if obj == nil {
		return errors.New("nil Task Scheduler COM object")
	}
	method := obj.vtable[index]
	argv := append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)
	hr, _, _ := syscall.SyscallN(method, argv...)
	if int32(hr) < 0 {
		return fmt.Errorf("COM method %d HRESULT 0x%08x: %w", index, uint32(hr), syscall.Errno(uint32(hr)))
	}
	return nil
}

func (obj *comObject) release() {
	if obj != nil {
		// IUnknown::Release (vtable slot 2); its result is only a reference count.
		_ = comMethod(obj, 2)
	}
}

func bstr(s string) (uintptr, error) {
	utf, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	ptr, _, _ := procSysAllocStringLen().Call(uintptr(unsafe.Pointer(&utf[0])), uintptr(len(utf)-1))
	if ptr == 0 {
		return 0, errors.New("SysAllocStringLen failed")
	}
	return ptr, nil
}

func freeBSTR(ptr uintptr) {
	if ptr != 0 {
		procSysFreeString().Call(ptr)
	}
}

func bstrValue(ptr uintptr) (string, error) {
	if ptr == 0 {
		return "", nil
	}
	n, _, _ := procSysStringLen().Call(ptr)
	if n > 1<<20 {
		return "", fmt.Errorf("Task Scheduler XML BSTR is too large")
	}
	buf := make([]uint16, int(n))
	if len(buf) == 0 {
		return "", nil
	}
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), ptr, (*byte)(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), nil); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

func withTaskFolder(fn func(*comObject) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initErr := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED)
	// A host may already have initialized this thread as STA; COM is usable in
	// that case, but this call did not add a reference to uninitialize.
	if initErr != nil && !errors.Is(initErr, syscall.Errno(1)) && !errors.Is(initErr, syscall.Errno(0x80010106)) {
		return fmt.Errorf("initialize Task Scheduler COM: %w", initErr)
	}
	if initErr == nil || errors.Is(initErr, syscall.Errno(1)) {
		defer windows.CoUninitialize()
	}
	var service *comObject
	clsid, iid := clsidTaskScheduler(), iidTaskService()
	hr, _, _ := procCoCreateInstance().Call(uintptr(unsafe.Pointer(&clsid)), 0,
		windows.CLSCTX_INPROC_SERVER, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&service)))
	if int32(hr) < 0 {
		return fmt.Errorf("create Task Scheduler COM service: %w", syscall.Errno(uint32(hr)))
	}
	defer service.release()
	var empty comVariant
	if err := comMethod(service, iTaskServiceConnect, uintptr(unsafe.Pointer(&empty)), uintptr(unsafe.Pointer(&empty)),
		uintptr(unsafe.Pointer(&empty)), uintptr(unsafe.Pointer(&empty))); err != nil {
		return fmt.Errorf("connect Task Scheduler: %w", err)
	}
	root, err := bstr(`\`)
	if err != nil {
		return err
	}
	defer freeBSTR(root)
	var folder *comObject
	if err := comMethod(service, iTaskServiceGetFolder, root, uintptr(unsafe.Pointer(&folder))); err != nil {
		return fmt.Errorf("get Task Scheduler root: %w", err)
	}
	defer folder.release()
	return fn(folder)
}

// isTaskNotFound matches the HRESULTs GetTask returns for a missing task.
func isTaskNotFound(err error) bool {
	return errors.Is(err, syscall.Errno(0x80070002)) || errors.Is(err, syscall.Errno(0x80070003))
}

type schedulerTask struct {
	XML   string
	State int32
}

func readScheduledTask(name string) (schedulerTask, bool, error) {
	var out schedulerTask
	var found bool
	err := withTaskFolder(func(folder *comObject) error {
		nameB, err := bstr(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nameB)
		var task *comObject
		if err := comMethod(folder, iTaskFolderGetTask, nameB, uintptr(unsafe.Pointer(&task))); err != nil {
			if isTaskNotFound(err) {
				return nil
			}
			return fmt.Errorf("get task %q: %w", name, err)
		}
		defer task.release()
		found = true
		var xmlPtr uintptr
		if err := comMethod(task, iRegisteredXML, uintptr(unsafe.Pointer(&xmlPtr))); err != nil {
			return err
		}
		out.XML, err = bstrValue(xmlPtr)
		freeBSTR(xmlPtr)
		if err != nil {
			return err
		}
		return comMethod(task, iRegisteredState, uintptr(unsafe.Pointer(&out.State)))
	})
	return out, found, err
}

func registerScheduledTask(name, xmlText, owner string, update bool) error {
	flags := uintptr(taskCreate)
	if update {
		flags = taskUpdate
	}
	return callRegisterTask(name, xmlText, owner, flags)
}

func validateScheduledTaskXML(name, xmlText, owner string) error {
	return callRegisterTask(name, xmlText, owner, taskValidateOnly)
}

func callRegisterTask(name, xmlText, owner string, flags uintptr) error {
	return withTaskFolder(func(folder *comObject) error {
		nameB, err := bstr(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nameB)
		xmlB, err := bstr(xmlText)
		if err != nil {
			return err
		}
		defer freeBSTR(xmlB)
		ownerB, err := bstr(owner)
		if err != nil {
			return err
		}
		defer freeBSTR(ownerB)
		ownerVariant := comVariant{Type: vtBSTR, Data: ownerB}
		var empty comVariant
		var task *comObject
		if err := comMethod(folder, iTaskFolderRegister, nameB, xmlB, flags,
			uintptr(unsafe.Pointer(&ownerVariant)), uintptr(unsafe.Pointer(&empty)), taskLogonInteractive,
			uintptr(unsafe.Pointer(&empty)), uintptr(unsafe.Pointer(&task))); err != nil {
			return fmt.Errorf("register task %q: %w", name, err)
		}
		task.release()
		return nil
	})
}

func runScheduledTask(name string) error {
	return withTaskFolder(func(folder *comObject) error {
		return withRegisteredTask(folder, name, func(task *comObject) error {
			var empty comVariant
			var running *comObject
			if err := comMethod(task, iRegisteredRun, uintptr(unsafe.Pointer(&empty)), uintptr(unsafe.Pointer(&running))); err != nil {
				return err
			}
			running.release()
			return nil
		})
	})
}

func withRegisteredTask(folder *comObject, name string, fn func(*comObject) error) error {
	nameB, err := bstr(name)
	if err != nil {
		return err
	}
	defer freeBSTR(nameB)
	var task *comObject
	if err := comMethod(folder, iTaskFolderGetTask, nameB, uintptr(unsafe.Pointer(&task))); err != nil {
		return err
	}
	defer task.release()
	return fn(task)
}

func deleteScheduledTask(name string) error {
	return withTaskFolder(func(folder *comObject) error {
		nameB, err := bstr(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nameB)
		return comMethod(folder, iTaskFolderDeleteTask, nameB, 0)
	})
}
