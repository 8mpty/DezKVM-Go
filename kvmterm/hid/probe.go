package hid

import (
	"bytes"
	"path/filepath"
	"runtime"
	"time"
)

// Probe opens device, sends a soft-reset, and reports whether the CH9329
// acknowledges it (reply header 57 AB 00 8F). It is used to identify which
// serial port the KVM is attached to.
//
// On a fresh connection the chip may emit a stale keyboard report before the
// reset ack, so we drain and accumulate replies until the ack (or a deadline).
func Probe(device string, baud int) (bool, error) {
	k, err := Open(device, baud)
	if err != nil {
		return false, err
	}
	defer k.Close()

	if err := k.Reset(); err != nil {
		return false, err
	}
	_ = k.port.SetReadTimeout(300 * time.Millisecond)

	var acc []byte
	buf := make([]byte, 64)
	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, _ := k.port.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			if bytes.Contains(acc, []byte{0x57, 0xAB, 0x00, 0x8F}) {
				return true, nil
			}
		}
	}
	return false, nil
}

// candidateDevices returns a list of plausible serial devices for the OS.
func candidateDevices() []string {
	switch runtime.GOOS {
	case "darwin":
		m, _ := filepath.Glob("/dev/cu.usbserial-*")
		return m
	case "windows":
		// COM port enumeration needs the registry; require an explicit flag.
		return nil
	default:
		var out []string
		for _, pat := range []string{"/dev/ttyUSB*", "/dev/ttyACM*", "/dev/serial/by-id/*"} {
			if m, _ := filepath.Glob(pat); len(m) > 0 {
				out = append(out, m...)
			}
		}
		return out
	}
}

// Detect finds the first candidate device that responds to the CH9329 probe.
// It returns "" if none was found.
func Detect(baud int) string {
	for _, d := range candidateDevices() {
		if ok, _ := Probe(d, baud); ok {
			return d
		}
	}
	return ""
}
