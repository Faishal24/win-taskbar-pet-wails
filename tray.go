package main

import (
	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// System tray: icon + Pause/Resume + Exit. Runs on systray's own thread via
// Run (blocking), so trayLoop is started as a goroutine from domready.
func (a *App) trayLoop() {
	systray.Run(
		func() {
			systray.SetIcon(trayIcon)
			systray.SetTooltip("win-taskbar-pet")
			mPause := systray.AddMenuItem("Pause", "Pause the pet")
			mAuto := systray.AddMenuItemCheckbox("Start with Windows", "Run at Windows startup", autostartEnabled())
			mRoam := systray.AddMenuItemCheckbox("Semua monitor", "Monyet jalan lintas monitor", a.cfg.RoamAll)
			systray.AddSeparator()
			mExit := systray.AddMenuItem("Exit", "Quit win-taskbar-pet")
			go func() {
				for {
					select {
					case <-mPause.ClickedCh:
						a.mu.Lock()
						a.paused = !a.paused
						paused := a.paused
						a.mu.Unlock()
						if paused {
							mPause.SetTitle("Resume")
						} else {
							mPause.SetTitle("Pause")
						}
						runtime.EventsEmit(a.ctx, "pet-paused", paused)
					case <-mExit.ClickedCh:
						systray.Quit()
						runtime.Quit(a.ctx)
						return
					case <-mRoam.ClickedCh:
						a.mu.Lock()
						a.cfg.RoamAll = !a.cfg.RoamAll
						roam := a.cfg.RoamAll
						a.mu.Unlock()
						if err := saveConfig(a.cfg); err != nil {
							println("[pet] config save:", err.Error())
						}
						if roam {
							mRoam.Check()
						} else {
							mRoam.Uncheck()
						}
						a.reslot()
					case <-mAuto.ClickedCh:
						// Toggle the registry (source of truth), then force
						// the checkbox to match it — correct whether or not
						// the control auto-toggles on click.
						if err := setAutostart(!autostartEnabled()); err != nil {
							println("[pet] autostart:", err.Error())
						}
						if autostartEnabled() {
							mAuto.Check()
						} else {
							mAuto.Uncheck()
						}
					case <-a.stopCh:
						systray.Quit()
						return
					}
				}
			}()
		},
		func() {},
	)
}
