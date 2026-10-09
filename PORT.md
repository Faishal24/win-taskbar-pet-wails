# Wails port — session handoff

Port of `../win-taskbar-pet` (Electron) to Wails v2 (Go + WebView2).
No installs were done in this session (user instruction) — nothing here has
been compiled yet. Frontend is a faithful port; backend mirrors the proven
Electron behavior point for point.

## After the Go toolchain lands (in order)

1. Install Go stable (Windows amd64) + `go install github.com/wailsapp/wails/v2/cmd/wails@latest`.
2. `cd frontend && npm install && cd ..`
3. `go mod tidy` (fixes pinned versions if any drifted).
4. `wails doctor` — must be all green (WebView2 present: 154.x confirmed).
5. `wails dev` — monkey should roam above the taskbar; verify with the
   Electron repo's scanners (`../win-taskbar-pet/tools/shoot-taskbar.ps1`
   + `/tmp/opencode/scan.py` — note: /tmp does not survive WSL restarts).
6. Click monkey → poke+flee in console; click elsewhere while selected →
   ring clears (backend `pet-unselect`).
7. `wails build -nsis` → check setup size (<20MB expected).

## Transparency recipe (VERIFIED working 2026-10-08)

All four required together — any missing piece gives an opaque bar:

1. `windows.Options{ WebviewIsTransparent: true }`
2. `runtime.WindowSetBackgroundColour(ctx, 0, 0, 0, 0)` after startup
   (the flag alone does nothing — the controller keeps opaque white until
   this is called; verified in wails v2.16.0 source)
3. NO `BackgroundColour` option (wails paints it as an OPAQUE class brush)
   + clear the class brush anyway (`SetClassLongPtrW GCLP_HBRBACKGROUND, 0`)
4. `WS_EX_LAYERED` on the host window — DWM ignores per-pixel alpha on
   plain top-level windows (white bar without it, black bar with 1-3 only)
5. Page CSS: `html,body { background: transparent }`, canvas clears to
   transparent; all sprite pixels fully opaque (no AA fringes to key on)

- `options.App`: `StartHidden`, `DisableResize`, `HideWindowOnClose`,
  `OnDomReady`, `Bind` — if any field is missing in the installed wails
  version, drop it (Go covers the behavior manually anyway).
- `runtime.WindowShow/Hide`, `runtime.EventsEmit`, `runtime.Quit` (core v2).
- `systray.Run` (blocking — we call it in a goroutine) vs `Register`;
  `AddMenuItem().ClickedCh`, `SetTitle`, `SetIcon` ([]byte .ico on Windows).
- `x/sys/windows`: `NewLazySystemDLL`, `UTF16PtrFromString`,
  `SysProcAttr{HideWindow:true}`.
- Installer UX: stock `wails build -nsis` is a multi-page wizard — decide
  then whether one-click/portable flags exist for the share build.

## Protocol (frontend ↔ backend)

- `window.go.main.App.UpdateRect(x,y,w,h,selected)` @10Hz (window-client phys px)
- `window.go.main.App.Log(msg)`
- backend → frontend events: `pet-hover(bool)`, `pet-unselect`, `pet-paused(bool)`
- poke/select/hover-cursor stay local in TS (real pointer events arrive —
  overlay is a normal top-level window).

## Behavior parity with Electron build

Walk/idle/flee/hop/select/pause, rect push, 60fps cap, crisp physical-px
rendering, gap debounce (2x), 250ms moveTop insurance, fullscreen hide.
Deliberately dropped: `PET_SHOT`/`PET_FREEZE` envs, in-taskbar SetParent era,
supervisor (single exe — revisit crash recovery separately), auto-hide
docking (was never live-tested; re-add after the basics work).
