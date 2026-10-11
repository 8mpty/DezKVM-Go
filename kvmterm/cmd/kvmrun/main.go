// Command kvmrun is a headless helper for kvmterm: it types a command into the
// remote terminal and prints the reconstructed screen. Useful for scripting
// and troubleshooting without the interactive TUI.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvmterm/framegrab"
	"kvmterm/hid"
	"kvmterm/screen"
)

func main() {
	serial := flag.String("serial", "auto", "serial device (auto scans)")
	baud := flag.Int("baud", 115200, "baud rate")
	video := flag.String("video", "", "video device")
	width := flag.Int("width", 1920, "capture width")
	height := flag.Int("height", 1080, "capture height")
	calFile := flag.String("cal", defaultCalPath(), "calibration file")
	wait := flag.Duration("wait", 1800*time.Millisecond, "delay after typing before grabbing")
	noClear := flag.Bool("no-clear", false, "don't send Ctrl+L before the command")
	sendKey := flag.String("key", "", "send a named key and exit: ctrl-c, ctrl-alt-del")
	flag.Parse()

	if *sendKey == "" && flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: kvmrun [flags] \"command to run\"")
		os.Exit(2)
	}
	cmd := ""
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
	}

	dev := *serial
	if dev == "auto" {
		dev = hid.Detect(*baud)
		if dev == "" {
			fmt.Fprintln(os.Stderr, "no CH9329 device detected (is the browser disconnected?)")
			os.Exit(1)
		}
	}
	kb, err := hid.Open(dev, *baud)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open serial: %v\n", err)
		os.Exit(1)
	}
	defer kb.Close()
	// Warm up the CH9329 so the first keystrokes are not dropped.
	if err := kb.Warmup(); err != nil {
		fmt.Fprintf(os.Stderr, "warmup: %v\n", err)
	}

	if *sendKey != "" {
		var err error
		switch *sendKey {
		case "ctrl-c":
			err = kb.CtrlC()
		case "ctrl-alt-del":
			err = kb.Combo(hid.ModLCtrl|hid.ModLAlt, hid.KeyDelete, 100*time.Millisecond)
		case "sysrq-reboot":
			err = kb.SysRQReboot()
		default:
			fmt.Fprintf(os.Stderr, "unknown key %q\n", *sendKey)
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", *sendKey, err)
		}
		fmt.Println("sent", *sendKey)
		return
	}

	cal, err := screen.LoadCalibration(*calFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load calibration: %v\n", err)
		os.Exit(1)
	}

	videoDev := *video
	if videoDev == "" {
		videoDev = framegrab.DefaultDevice()
	}
	grab := framegrab.New(*width, *height, framegrab.DefaultArgs(videoDev, *width, *height))

	if !*noClear {
		if err := kb.CtrlL(); err != nil {
			fmt.Fprintf(os.Stderr, "ctrl+l: %v\n", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := kb.Command(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "type command: %v\n", err)
		os.Exit(1)
	}
	time.Sleep(*wait)

	img, err := grab.Grab()
	if err != nil {
		fmt.Fprintf(os.Stderr, "grab: %v\n", err)
		os.Exit(1)
	}
	t := cal.Render(img)
	printText(t)
}

func printText(t *screen.Text) {
	// find content region
	first, last := -1, -1
	for r := 0; r < t.Rows; r++ {
		if !blank(t.Chars[r]) {
			if first < 0 {
				first = r
			}
			last = r
		}
	}
	if first < 0 {
		return
	}
	for r := first; r <= last; r++ {
		fmt.Println(strings.TrimRight(string(t.Chars[r]), " "))
	}
}

func blank(row []byte) bool {
	for _, c := range row {
		if c != ' ' && c != 0 {
			return false
		}
	}
	return true
}

func defaultCalPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "kvmterm-cal.json"
	}
	return filepath.Join(home, ".kvmterm-cal.json")
}
