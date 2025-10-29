//go:build windows

package phpenv

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"phpenv/internal/config"
)

const (
	hwndBroadcast   = 0xffff
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

func setPersistentEnvironment(cfg *config.Config, runtime ResolvedRuntime) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		key, _, err = registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE|registry.QUERY_VALUE)
		if err != nil {
			return err
		}
	}
	defer key.Close()

	if err := key.SetStringValue("PHP_ROOT", cfg.Root); err != nil {
		return err
	}
	if err := key.SetStringValue("CURRENT_PHP", runtime.PHPPath); err != nil {
		return err
	}

	existingPath, _, err := key.GetStringValue("Path")
	if err != nil {
		existingPath = os.Getenv("PATH")
	}

	merged := MergePathSegments(runtime.PathAdditions, existingPath)
	if err := key.SetStringValue("Path", merged); err != nil {
		return err
	}

	return broadcastEnvironmentChange()
}

func broadcastEnvironmentChange() error {
	ptr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}
	var result uintptr
	ret, _, callErr := procSendMessageTimeoutW.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(ptr)),
		uintptr(smtoAbortIfHung),
		uintptr(5000),
		uintptr(unsafe.Pointer(&result)),
	)
	if ret == 0 {
		if callErr != nil {
			return callErr
		}
		return syscall.EINVAL
	}
	return nil
}
