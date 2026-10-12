// Command kvmterm is a text virtual terminal over a DezKVM-Go USB KVM.
//
// It reads the remote machine's video, reconstructs the text terminal via
// glyph template matching, and forwards local keystrokes to the remote machine
// through the CH9329 HID chip.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"kvmterm/framegrab"
	"kvmterm/hid"
	"kvmterm/screen"
	"kvmterm/tui"
)

func main() {
	fs := flag.NewFlagSet("kvmterm", flag.ExitOnError)

	serial := fs.String("serial", "auto", "serial device (CH9329 bridge); 'auto' scans common ports")
	baud := fs.Int("baud", 115200, "serial baud rate")
	video := fs.String("video", "", "video device specifier (default is OS-specific)")
	width := fs.Int("width", 1920, "capture width")
	height := fs.Int("height", 1080, "capture height")
	interval := fs.Duration("interval", 300*time.Millisecond, "frame poll interval")
	calFile := fs.String("cal", defaultCalPath(), "calibration cache file ('' disables)")
	recal := fs.Bool("recalibrate", false, "force re-calibration")
	printOnce := fs.Bool("print", false, "non-interactive: calibrate, grab one frame, print text, exit")
	listVideo := fs.Bool("list-video", false, "list ffmpeg video devices and exit")
	fs.Parse(os.Args[1:])

	if *listVideo {
		run("ffmpeg", "-f", "avfoundation", "-list_devices", "true", "-i", "")
		return
	}

	dev := *serial
	if dev == "auto" {
		dev = hid.Detect(*baud)
		if dev == "" {
			fmt.Fprintln(os.Stderr, "no CH9329 serial device auto-detected; specify one with --serial")
			os.Exit(1)
		}
	}

	kb, err := hid.Open(dev, *baud)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open serial: %v\n", err)
		os.Exit(1)
	}
	defer kb.Close()
	fmt.Printf("connected to CH9329 on %s @ %d\n", dev, *baud)

	// Warm up the CH9329 so the first keystrokes are not dropped.
	if err := kb.Warmup(); err != nil {
		fmt.Fprintf(os.Stderr, "warmup: %v\n", err)
	}

	videoDev := *video
	if videoDev == "" {
		videoDev = framegrab.DefaultDevice()
	}
	grab := framegrab.New(*width, *height, framegrab.DefaultArgs(videoDev, *width, *height))
	fmt.Printf("video device: %q (%dx%d)\n", videoDev, *width, *height)

	var cal *screen.Calibration
	if !*recal && *calFile != "" {
		if cal, err = screen.LoadCalibration(*calFile); err != nil {
			fmt.Printf("no usable calibration (%v); calibrating...\n", err)
			cal = nil
		} else {
			fmt.Printf("loaded calibration from %s\n", *calFile)
		}
	}
	if cal == nil {
		cal, err = screen.Calibrate(kb, grab)
		if err != nil {
			fmt.Fprintf(os.Stderr, "calibration failed: %v\n", err)
			os.Exit(1)
		}
		if *calFile != "" {
			if err := cal.Save(*calFile); err != nil {
				fmt.Fprintf(os.Stderr, "save calibration: %v\n", err)
			}
		}
	}

	if *printOnce {
		img, err := grab.Grab()
		if err != nil {
			fmt.Fprintf(os.Stderr, "grab: %v\n", err)
			os.Exit(1)
		}
		t := cal.Render(img)
		for _, row := range t.Chars {
			fmt.Println(string(row))
		}
		return
	}

	if err := tui.New(kb, grab, cal, *interval).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}

func defaultCalPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "kvmterm-cal.json"
	}
	return filepath.Join(home, ".kvmterm-cal.json")
}

func run(name string, args ...string) {
	// Used only for --list-video.
	fmt.Printf("listing video devices via %s\n", name)
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}
