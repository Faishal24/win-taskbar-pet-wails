package main

import "testing"

func TestTaskbarQueries(t *testing.T) {
	r, ok := taskbarRect()
	if !ok {
		t.Fatal("taskbarRect: no Shell_TrayWnd")
	}
	if r.Right <= r.Left || r.Bottom <= r.Top {
		t.Fatalf("taskbarRect: bad rect %+v", r)
	}
	// Read-only query; either value is valid depending on the user setting.
	t.Logf("autohide=%v taskbar=%+v", taskbarAutoHide(), r)
}

func TestDockedSlotSitsOnBottomEdge(t *testing.T) {
	a := &App{}
	a.cfg.RoamAll = true
	s := a.dockedSlot()
	_, sh := screenSize()
	if s.y+s.h != sh {
		t.Fatalf("docked slot bottom=%d want screen bottom=%d", s.y+s.h, sh)
	}
	_, _, vw, _ := virtualScreen()
	if s.w != vw {
		t.Fatalf("docked slot width=%d want virtual width=%d", s.w, vw)
	}
	a.cfg.RoamAll = false
	pw, _ := screenSize()
	if s := a.dockedSlot(); s.w != pw {
		t.Fatalf("primary-only docked width=%d want %d", s.w, pw)
	}
}
