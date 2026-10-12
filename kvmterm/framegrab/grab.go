// Package framegrab captures frames from the DezKVM-Go's HDMI capture card
// (an MS2109 / MS2109S UVC device) by shelling out to ffmpeg.
//
// ffmpeg is used because it abstracts the per-OS capture backend behind one
// tool: AVFoundation on macOS, V4L2 on Linux, and DirectShow on Windows.
// Frames are read as raw RGBA so there is no image-codec overhead.
package framegrab

import (
	"bytes"
	"fmt"
	"image"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Grabber captures still frames from a video device.
type Grabber struct {
	width  int
	height int
	// inputArgs is everything ffmpeg needs up to and including the -i device.
	inputArgs []string
}

// New builds a Grabber from an explicit set of ffmpeg input arguments
// (e.g. {"-f","avfoundation","-framerate","30","-video_size","1920x1080","-i","0:none"}).
func New(width, height int, inputArgs []string) *Grabber {
	return &Grabber{width: width, height: height, inputArgs: inputArgs}
}

// DefaultArgs returns sensible ffmpeg input arguments for the current OS,
// given a device specifier (see DefaultDevice).
func DefaultArgs(device string, width, height int) []string {
	size := strconv.Itoa(width) + "x" + strconv.Itoa(height)
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"-f", "avfoundation", "-framerate", "30", "-video_size", size,
			"-i", device + ":none",
		}
	case "windows":
		return []string{"-f", "dshow", "-video_size", size, "-i", "video=" + device}
	default: // linux and others
		return []string{"-f", "v4l2", "-video_size", size, "-i", device}
	}
}

// DefaultDevice returns the default video device for the current OS. It is a
// best-effort guess; users can override it with a flag.
func DefaultDevice() string {
	switch runtime.GOOS {
	case "darwin":
		return "0" // AVFoundation video device index (0 = first capture device)
	case "windows":
		return "USB Video"
	default:
		return "/dev/video0"
	}
}

// Grab captures a single frame as an RGBA image.
func (g *Grabber) Grab() (*image.RGBA, error) {
	args := append([]string{"-y", "-hide_banner", "-loglevel", "error"}, g.inputArgs...)
	args = append(args, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")

	cmd := exec.Command("ffmpeg", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}

	want := g.width * g.height * 4
	if out.Len() < want {
		return nil, fmt.Errorf("short frame: got %d bytes, want %d", out.Len(), want)
	}

	img := image.NewRGBA(image.Rect(0, 0, g.width, g.height))
	copy(img.Pix, out.Bytes()[:want])
	return img, nil
}
