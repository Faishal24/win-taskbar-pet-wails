# Wails port — handoff notes

Port of `../win-taskbar-pet` (Electron, archived) to Wails v2 (Go + WebView2).
Status: **working and user-verified** (tray, autostart, click/poke, roam across
both monitors). Portable exe ≈12MB vs Electron's 111MB installer.

## Build & run

1. Toolchain: Go (Windows amd64), `go install github.com/wailsapp/wails/v2/cmd/wails@latest`,
   Node/npm on PATH. No MSVC, no CGO.
2. `cd frontend && npm install && cd ..`
3. `wails build` → `build/bin/win-taskbar-pet.exe` (single portable file).
4. `go test ./...` → autostart registry roundtrip.

Run from the repo root, not `frontend/` (`wails.json` lives here).

## Transparency recipe (VERIFIED)

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

Measured check: mean screenshot brightness 57.1 with the pet running vs 55.0
with it killed (transparent over desktop).

## Windows quirks discovered

- Win11 still shows a taskbar button despite `WS_EX_TOOLWINDOW`; the button is
  deleted with `ITaskbarList::DeleteTab` on every show and again every 2s
  (Explorer re-adds buttons on show cycles).
- The host window cannot be found by title reliably; look it up by our own PID
  plus class `wailsWindow`.
- With `StartHidden`, gap probes flap (630↔602) forever, so a 2x debounce never
  fires and the window never shows. The first probe report is applied
  immediately; debounce only guards later changes.
- `wails build -nsis` needs `makensis`, which is not installed on this machine
  (winget has no package; the SourceForge URL used is dead). Portable exe is
  the share artifact until an NSIS source is available.

## Protocol (frontend ↔ backend)

- `window.go.main.App.UpdateRect(x,y,w,h,selected)` @10Hz (window-client phys px)
- `window.go.main.App.Log(msg)`
- backend → frontend events: `pet-hover(bool)`, `pet-unselect`, `pet-paused(bool)`
- poke/select/hover-cursor stay local in TS (real pointer events arrive —
  overlay is a normal top-level window).

## Behavior parity with Electron build

Walk/idle/flee/hop/select/pause, rect push, 60fps cap, crisp physical-px
rendering, gap debounce (2x), 250ms moveTop insurance, fullscreen hide,
roam-all-monitors or primary-only (persisted in `%APPDATA%\win-taskbar-pet\config.json`).

Deliberately dropped: `PET_SHOT`/`PET_FREEZE` envs, in-taskbar SetParent era,
supervisor (single exe — revisit crash recovery separately), auto-hide
docking (was never live-tested; re-add after the basics work).

## Known gaps

- Auto-hide taskbar: pet disappears with the taskbar (no docking yet).
- Secondary monitor with a taskbar at a different height/position: the strip
  follows the primary taskbar's top edge.
- No crash recovery/watchdog.
- No settings window, no sprite-pack switcher, no installer.
