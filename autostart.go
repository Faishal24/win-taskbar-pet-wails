package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// Autostart via HKCU\...\Run (per-user, no admin). The tray menu exposes a
// "Start with Windows" checkbox driving these two functions.

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	runName    = "win-taskbar-pet"
)

func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runName)
	return err == nil
}

func setAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enable {
		if err := k.DeleteValue(runName); err != nil {
			// Deleting a missing value: registry.ErrNotExist may surface as
			// a syscall errno — treat "not exist" as success either way by
			// re-checking.
			if autostartEnabled() {
				return err
			}
			return nil
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(runName, `"`+exe+`"`)
}
