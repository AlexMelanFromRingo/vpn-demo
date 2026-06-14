//go:build linux
// +build linux

package tun

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	ifReqSize = unix.IFNAMSIZ + 64
)

type ifReq struct {
	Name  [unix.IFNAMSIZ]byte
	Flags uint16
	pad   [ifReqSize - unix.IFNAMSIZ - 2]byte
}

// createTunDevice creates a TUN device using /dev/net/tun
func createTunDevice(name string) (*os.File, string, error) {
	fd, err := unix.Open("/dev/net/tun", os.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open /dev/net/tun: %w", err)
	}

	var req ifReq
	req.Flags = unix.IFF_TUN | unix.IFF_NO_PI
	copy(req.Name[:], name)

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TUNSETIFF),
		uintptr(unsafe.Pointer(&req)),
	)
	if errno != 0 {
		unix.Close(fd)
		return nil, "", fmt.Errorf("ioctl TUNSETIFF failed: %v", errno)
	}

	// Get actual name
	actualName := string(req.Name[:])
	for i, c := range req.Name {
		if c == 0 {
			actualName = string(req.Name[:i])
			break
		}
	}

	return os.NewFile(uintptr(fd), "/dev/net/tun"), actualName, nil
}

// createDevice creates a TUN device for Linux
func createDevice(cfg Config) (device, string, error) {
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "vpn%d"
	}

	file, name, err := createTunDevice(deviceName)
	if err != nil {
		return nil, "", err
	}

	return &tunDevice{file: file}, name, nil
}
