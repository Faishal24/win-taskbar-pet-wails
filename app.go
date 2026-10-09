package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Strip the pet roams: full screen width, sitting on top of the taskbar.
// 45 physical px tall (same as the Electron build).
const slotH = int32(45)

type monkeyState struct {
	x, y, w, h int32
	selected   bool
}

type App struct {
	ctx      context.Context
	mu       sync.Mutex
	rect     monkeyState
	slotX    int32
	slotY    int32
	hwnd     uintptr
	paused   bool
	shown    bool
	hovered  bool
	hidden   bool
	stopCh   chan struct{}
	gapCh    chan gapInfo
	lastSlot slotRect
	pending  slotRect
	hasPend  bool
}

type slotRect struct{ x, y, w, h int32 }

func sameSlot(a, b slotRect) bool {
	return a.x == b.x && a.y == b.y && a.w == b.w && a.h == b.h
}

// --- bound methods (called from the frontend) ---

func (a *App) Log(msg string) {
	println("[pet]", msg)
}

func (a *App) UpdateRect(x, y, w, h int, selected bool) {
	a.mu.Lock()
	a.rect = monkeyState{int32(x), int32(y), int32(w), int32(h), selected}
	a.mu.Unlock()
}

// --- lifecycle (wired in main.go) ---

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.stopCh = make(chan struct{})
	a.gapCh = make(chan gapInfo, 4)
}

func (a *App) domready(ctx context.Context) {
	// Find our window (title must match options.Title) and apply the
	// overlay styles: no taskbar button, never takes focus, click-through
	// until hover (backend toggles transparency itself).
	for i := 0; i < 50; i++ {
		a.hwnd = ownMainWindow()
		if a.hwnd != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if a.hwnd == 0 {
		println("[pet] FATAL: overlay window not found")
		return
	}
	a.Log(fmt.Sprintf("overlay hwnd=0x%X, applying styles", a.hwnd))
	applyBaseStyles(a.hwnd)
	// The WebviewIsTransparent flag only takes effect inside
	// WindowSetBackgroundColour — without this call the controller keeps
	// its default opaque background. Alpha 0 = transparent (Windows only
	// honors 0/255). This also slaps an opaque brush on the host window,
	// so clear it right after (see clearClassBackground).
	runtime.WindowSetBackgroundColour(a.ctx, 0, 0, 0, 0)
	clearClassBackground(a.hwnd)
	go runGapProbe(a.gapCh, a.stopCh, a.Log)
	go a.gapLoop()
	go a.inputLoop()
	go a.watchLoop()
	go a.trayLoop()
}

func (a *App) shutdown(ctx context.Context) {
	select {
	case <-a.stopCh:
	default:
		close(a.stopCh)
	}
}

// --- positioning ---

func (a *App) applySlot(s slotRect) {
	placeWindow(a.hwnd, s.x, s.y, s.w, s.h)
	if r, ok := windowRect(a.hwnd); ok {
		a.Log(fmt.Sprintf("window rect now %d,%d %dx%d", r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top))
	}
	a.mu.Lock()
	a.slotX, a.slotY = s.x, s.y
	a.lastSlot = s
	shown := a.shown
	a.mu.Unlock()
	if !shown {
		runtime.WindowShow(a.ctx)
		a.mu.Lock()
		a.shown = true
		a.mu.Unlock()
		a.Log("overlay positioned and shown")
	}
	// Win11 shows a taskbar button for us despite WS_EX_TOOLWINDOW —
	// delete it every time we (re)show.
	deleteTaskbarTab(a.hwnd)
}

func (a *App) gapLoop() {
	sw, _ := screenSize()
	a.Log(fmt.Sprintf("screenW=%d", sw))
	first := true
	for g := range a.gapCh {
		slot := slotRect{x: 0, y: int32(g.Gap.Y) - slotH, w: sw, h: slotH}
		if first {
			// Position immediately on the first report — the layout may
			// flap forever (630<->602), which would otherwise deadlock
			// the initial show behind the debounce below.
			first = false
			a.Log(fmt.Sprintf("first gap side=%s taskbar=%d,%d %dx%d",
				g.Side, g.Taskbar.X, g.Taskbar.Y, g.Taskbar.W, g.Taskbar.H))
			a.applySlot(slot)
			continue
		}
		a.mu.Lock()
		last := a.lastSlot
		pend, hasPend := a.pending, a.hasPend
		a.mu.Unlock()
		if sameSlot(last, slot) {
			a.mu.Lock()
			a.hasPend = false
			a.mu.Unlock()
			continue
		}
		// Debounce: apply only after two consecutive identical reports.
		if !hasPend || !sameSlot(pend, slot) {
			a.mu.Lock()
			a.pending, a.hasPend = slot, true
			a.mu.Unlock()
			continue
		}
		a.mu.Lock()
		a.hasPend = false
		a.mu.Unlock()
		a.Log(fmt.Sprintf("gap side=%s taskbar=%d,%d %dx%d -> slot=%d,%d %dx%d",
			g.Side, g.Taskbar.X, g.Taskbar.Y, g.Taskbar.W, g.Taskbar.H,
			slot.x, slot.y, slot.w, slot.h))
		a.applySlot(slot)
	}
}

// --- input: hover detect + click-anywhere unselect ---

func (a *App) inputLoop() {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	var wasDown *bool
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
		}
		cx, cy, ok := cursorPos()
		if !ok {
			continue
		}
		down := buttonDown(vkLButton) || buttonDown(vkRButton)
		a.mu.Lock()
		r := a.rect
		sx, sy := a.slotX, a.slotY
		hovered := a.hovered
		a.mu.Unlock()

		const pad = int32(5)
		inside := cx >= sx+r.x-pad && cx <= sx+r.x+r.w+pad &&
			cy >= sy+r.y-pad && cy <= sy+r.y+r.h+pad
		if inside != hovered {
			a.mu.Lock()
			a.hovered = inside
			a.mu.Unlock()
			setTransparent(a.hwnd, !inside)
			runtime.EventsEmit(a.ctx, "pet-hover", inside)
		}
		if wasDown == nil {
			b := down
			wasDown = &b
			continue
		}
		if down && !*wasDown && r.selected {
			const up = int32(25)
			rx, ry := sx+r.x, sy+r.y
			if cx < rx-up || cx > rx+r.w+up || cy < ry-up || cy > ry+r.h+up {
				runtime.EventsEmit(a.ctx, "pet-unselect")
			}
		}
		*wasDown = down
	}
}

// --- z-order insurance + fullscreen hide ---

func (a *App) watchLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var tick uint64
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
		}
		a.mu.Lock()
		hidden := a.hidden
		shown := a.shown
		a.mu.Unlock()
		full := fullscreenActive()
		if full && !hidden {
			a.mu.Lock()
			a.hidden = true
			a.mu.Unlock()
			runtime.WindowHide(a.ctx)
			a.Log("watch: fullscreen -> hide")
		} else if !full && hidden {
			a.mu.Lock()
			a.hidden = false
			a.mu.Unlock()
			if shown {
				runtime.WindowShow(a.ctx)
				a.Log("watch: visible again -> show")
			}
		}
		a.mu.Lock()
		stillHidden := a.hidden
		a.mu.Unlock()
		if !stillHidden && shown {
			moveTop(a.hwnd)
		}
		tick++
		if tick%8 == 0 && shown {
			// Explorer re-adds taskbar buttons on show/focus cycles.
			deleteTaskbarTab(a.hwnd)
		}
	}
}
