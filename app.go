package main

import (
	"context"
	"os"
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
	cfg      config
	lastGap  gapInfo
	hasGap   bool
	docked   bool
	mons     []monitorInfo
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
	a.followMonitor()
}

// petCenterX is the pet's screen x (window x + pushed rect x).
func (a *App) petCenterX() int32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.slotX + a.rect.x + a.rect.w/2
}

// railTop is the taskbar top edge on the monitor holding screen x.
func (a *App) railTop(x int32) int32 {
	a.mu.Lock()
	list := a.mons
	a.mu.Unlock()
	if len(list) == 0 {
		_, h := screenSize()
		return h - 60
	}
	return monitorAt(list, x).RailTop
}

// followMonitor slides the strip vertically when the pet crosses onto a
// monitor with a different taskbar height or offset. The renderer always
// draws at the canvas bottom, so moving the window is all it takes.
func (a *App) followMonitor() {
	a.mu.Lock()
	shown := a.shown
	docked := a.docked
	s := a.lastSlot
	x := a.slotX + a.rect.x + a.rect.w/2
	a.mu.Unlock()
	if !shown || s.w == 0 {
		return
	}
	want := a.railTop(x) - slotH
	if docked {
		a.mu.Lock()
		list := a.mons
		a.mu.Unlock()
		if len(list) > 0 {
			m := monitorAt(list, x)
			want = m.Y + m.H - slotH
		}
	}
	a.mu.Lock()
	cur := a.slotY
	if want == cur {
		a.mu.Unlock()
		return
	}
	a.slotY = want
	a.lastSlot.y = want
	if a.hasPend {
		a.pending.y = want
	}
	a.mu.Unlock()
	placeWindow(a.hwnd, s.x, want, s.w, s.h)
	r, _ := windowRect(a.hwnd)
	a.Log(fmt.Sprintf("follow pet-x=%d -> y=%d (rect %d,%d %dx%d)", x, want, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top))
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
	a.mu.Lock()
	a.mons = monitors()
	list := a.mons
	a.mu.Unlock()
	for _, m := range list {
		a.Log(fmt.Sprintf("monitor %d,%d %dx%d rail=%d", m.X, m.Y, m.W, m.H, m.RailTop))
	}
	go runGapProbe(a.gapCh, a.stopCh, a.Log)
	go a.gapLoop()
	go a.inputLoop()
	go a.watchLoop()
	go a.trayLoop()
	if os.Getenv("PET_FREEZE") != "" {
		go func() {
			// Debug/testing aid: freeze the pet where it spawned.
			time.Sleep(700 * time.Millisecond)
			a.mu.Lock()
			a.paused = true
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "pet-paused", true)
			a.Log("PET_FREEZE: pet paused")
		}()
	}
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
	a.mu.Unlock()
	a.ensureShown()
	runtime.EventsEmit(a.ctx, "pet-layout", s.w, s.h)
}

// ensureShown shows the overlay once and drops the stray taskbar button.
func (a *App) ensureShown() {
	a.mu.Lock()
	shown := a.shown
	a.mu.Unlock()
	if shown {
		return
	}
	runtime.WindowShow(a.ctx)
	a.mu.Lock()
	a.shown = true
	a.mu.Unlock()
	a.Log("overlay positioned and shown")
	// Win11 shows a taskbar button for us despite WS_EX_TOOLWINDOW —
	// delete it every time we (re)show.
	deleteTaskbarTab(a.hwnd)
}

func (a *App) gapLoop() {
	vx, _, vw, _ := virtualScreen()
	pw, _ := screenSize()
	a.Log(fmt.Sprintf("virtual=%d..%d primaryW=%d roamAll=%v", vx, vx+vw, pw, a.cfg.RoamAll))
	first := true
	for g := range a.gapCh {
		a.mu.Lock()
		a.lastGap, a.hasGap = g, true
		roam := a.cfg.RoamAll
		docked := a.docked
		a.mu.Unlock()
		if docked {
			// The probe still sees the (slid-away) taskbar while we are
			// docked; watchLoop owns positioning until it comes back.
			continue
		}
		slot := a.slotFor(g, vx, vw, pw, roam)
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

// slotFor maps a probe gap to our strip: full virtual width when roaming
// all monitors, primary width only otherwise. y follows the primary
// taskbar; on monitors with a different taskbar top it floats slightly.
func (a *App) slotFor(g gapInfo, vx, vw, pw int32, roam bool) slotRect {
	w := vw
	if !roam {
		w = pw
	}
	return slotRect{x: vx, y: a.railTop(a.petCenterX()) - slotH, w: w, h: slotH}
}

// reslot re-applies the current setting immediately (tray toggle path).
func (a *App) reslot() {
	a.mu.Lock()
	g, ok := a.lastGap, a.hasGap
	roam := a.cfg.RoamAll
	docked := a.docked
	a.mu.Unlock()
	if !ok {
		return
	}
	if docked {
		a.applyDockedSlot()
		return
	}
	vx, _, vw, _ := virtualScreen()
	pw, _ := screenSize()
	a.applySlot(a.slotFor(g, vx, vw, pw, roam))
	a.Log(fmt.Sprintf("reslot roamAll=%v", roam))
}

// dockedSlot is the strip parked flush against the screen bottom, used
// while an auto-hide taskbar is slid away (the pet stays visible).
func (a *App) dockedSlot() slotRect {
	vx, _, vw, _ := virtualScreen()
	pw, _ := screenSize()
	w := vw
	if !a.cfg.RoamAll {
		w = pw
	}
	_, sh := screenSize()
	y := sh
	a.mu.Lock()
	list := a.mons
	a.mu.Unlock()
	if len(list) > 0 {
		m := monitorAt(list, a.petCenterX())
		y = m.Y + m.H
	}
	return slotRect{x: vx, y: y - slotH, w: w, h: slotH}
}

// applyDockedSlot parks the window at the bottom edge. It intentionally does
// NOT touch lastSlot: that stays the real above-taskbar slot so undocking
// can climb straight back.
func (a *App) applyDockedSlot() {
	s := a.dockedSlot()
	placeWindow(a.hwnd, s.x, s.y, s.w, s.h)
	a.mu.Lock()
	a.slotX, a.slotY = s.x, s.y
	a.mu.Unlock()
	a.ensureShown()
}

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
			runtime.EventsEmit(a.ctx, "pet-hover", inside)
			a.Log(fmt.Sprintf("hover=%v cursor=%d,%d rect=%d,%d %dx%d slot=%d,%d",
				inside, cx, cy, r.x, r.y, r.w, r.h, sx, sy))
		}
		if wasDown == nil {
			b := down
			wasDown = &b
			continue
		}
		if down && !*wasDown {
			// Clicks are handled here, not in the webview: a layered window
			// spanning monitors with different DPIs does not receive mouse
			// input on the secondary monitor, so polling is the only reliable
			// path. The window stays click-through (WS_EX_TRANSPARENT), which
			// also keeps the taskbar under the strip fully usable.
			if inside {
				dir := 1
				if cx >= sx+r.x+r.w/2 {
					dir = -1
				}
				runtime.EventsEmit(a.ctx, "pet-poke", dir)
				a.Log(fmt.Sprintf("poke at %d,%d dir=%d", cx, cy, dir))
			} else if r.selected {
				const up = int32(25)
				rx, ry := sx+r.x, sy+r.y
				if cx < rx-up || cx > rx+r.w+up || cy < ry-up || cy > ry+r.h+up {
					runtime.EventsEmit(a.ctx, "pet-unselect")
				}
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
		// Auto-hide taskbar: the pet must NOT vanish with it. When the
		// taskbar slides away (its rect collapses to the screen edge), park
		// the strip flush against the bottom; when it returns, climb back up.
		docked := false
		a.mu.Lock()
		stillHidden := a.hidden
		a.mu.Unlock()
		if !stillHidden && taskbarAutoHide() {
			if r, ok := taskbarRect(); ok {
				_, sh := screenSize()
				overlap := min(r.Bottom, sh) - max(r.Top, 0)
				docked = overlap < 20
			}
		}
		a.mu.Lock()
		wasDocked := a.docked
		a.docked = docked
		stillHidden = a.hidden
		shown = a.shown
		last := a.lastSlot
		a.mu.Unlock()
		if !stillHidden {
			if docked && (!wasDocked || !shown) {
				a.applyDockedSlot()
				a.Log("taskbar hidden — docking to bottom edge")
			} else if !docked && wasDocked {
				a.applySlot(last)
				a.Log("taskbar back — climbing on top")
			}
			moveTop(a.hwnd)
		}
		tick++
		if tick%8 == 0 {
			// Monitor layout is cheap to refresh and can change any time
			// (plug/unplug, taskbar height, scale).
			list := monitors()
			a.mu.Lock()
			a.mons = list
			a.mu.Unlock()
			a.followMonitor()
		}
		if tick%8 == 0 && shown {
			// Explorer re-adds taskbar buttons on show/focus cycles.
			deleteTaskbarTab(a.hwnd)
		}
	}
}
