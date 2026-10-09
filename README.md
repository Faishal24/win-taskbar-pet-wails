# win-taskbar-pet (Wails port)

Pixel monkey roaming above the Windows taskbar. Go + WebView2 build of the
[Electron prototype](../win-taskbar-pet) — same pet, ~10x smaller
(12MB portable exe vs 111MB installer).

## Run / build (Windows)

- Toolchain: Go stable + `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
  (no MSVC needed — pure Go, no CGO).
- Dev: `wails dev` (needs `go` + `npm` on PATH).
- Prod: `wails build` → `build/bin/win-taskbar-pet.exe` (single file, portable).
- Tests: `go test ./...` (autostart registry roundtrip).

## Layout

- `main.go`, `app.go`, `tray.go`, `autostart.go` — app, loops, tray, autostart.
- `win32.go` — Win32 via lazy DLLs (find/position/topmost/styles, cursor,
  buttons, fullscreen detect, taskbar-tab removal).
- `gap.go` + `gap-probe.ps1` — UIA taskbar gap probe (same protocol as Electron).
- `frontend/` — canvas renderer (classic sprite pack, walk/idle/flee/hop,
  crisp physical pixels) + `bridge.ts` (backend bindings without codegen).
- `assets/` — tray icon + app icon (generated from the sprite).
- `PORT.md` — port notes (assumptions, protocol, dropped scope).

## Behavior (verified live)

Transparent topmost overlay tracking the taskbar gap full-width; hover,
click → poke-flee + select, click-anywhere unselect, pause/resume + exit +
autostart toggle in tray; fullscreen hide; Explorer-restart-proof (top-level).
