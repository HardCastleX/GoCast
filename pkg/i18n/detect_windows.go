//go:build windows

package i18n

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")
)

// SystemLanguage returns the language code matching the Windows user locale,
// defaulting to English for any non-Spanish system.
func SystemLanguage() string {
	const localeNameMaxLength = 85

	buf := make([]uint16, localeNameMaxLength)
	n, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if n == 0 {
		return English
	}

	locale := strings.ToLower(syscall.UTF16ToString(buf))
	if strings.HasPrefix(locale, "es") {
		return Spanish
	}
	return English
}
