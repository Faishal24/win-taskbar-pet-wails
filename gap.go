package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

//go:embed gap-probe.ps1
var gapProbePS string

// Same JSON protocol as the Electron gapProbe.ts.
type gapRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type gapInfo struct {
	Taskbar gapRect `json:"taskbar"`
	Gap     gapRect `json:"gap"`
	Side    string  `json:"side"`
}

func parseGapLine(line string) (gapInfo, bool) {
	var g gapInfo
	start := strings.Index(line, "{")
	end := strings.LastIndex(line, "}")
	if start < 0 || end <= start {
		return g, false
	}
	if err := json.Unmarshal([]byte(line[start:end+1]), &g); err != nil {
		return g, false
	}
	if g.Gap.W <= 0 || g.Taskbar.W <= 0 {
		return g, false
	}
	return g, true
}

// runGapProbe spawns the long-lived PowerShell UIA probe and streams parsed
// gap reports. It stops when stopCh closes (kills the child).
func runGapProbe(out chan<- gapInfo, stopCh <-chan struct{}, onLog func(string)) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", gapProbePS)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		onLog("probe spawn failed: " + err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		onLog("probe start failed: " + err.Error())
		return
	}
	go func() {
		<-stopCh
		_ = cmd.Process.Kill()
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if g, ok := parseGapLine(scanner.Text()); ok {
			select {
			case out <- g:
			default:
				// slow consumer: drop, fresher data is coming
			}
		}
	}
	_ = cmd.Wait()
	onLog("gap probe exited")
}
