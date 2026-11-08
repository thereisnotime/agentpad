// Package usbraw sends raw 64-byte command packets to a CH57x pad's vendor
// interface through Linux usbdevfs, without libusb. It mirrors what
// ch57x-keyboard-tool does: detach the kernel driver from the command
// interface, claim it, send a 64-byte zero packet, then each command padded to
// 64 bytes as an interrupt OUT transfer; finally release and reattach.
//
// It exists for LED experiments (agentpad led-lab); normal programming still
// goes through ch57x-keyboard-tool.
package usbraw

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// PacketSize is the vendor interface's report size.
const PacketSize = 64

// Defaults for 1189:8890 (interface 1, interrupt OUT endpoint 0x02).
const (
	DefaultInterface = 1
	DefaultEndpoint  = 0x02
)

// ioctl numbers (linux/usbdevice_fs.h), 'U' = 0x55.
const (
	iocClaimInterface   = 0x8004550f // _IOR('U', 15, unsigned int)
	iocReleaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	iocBulk             = 0xc0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer) (64-bit)
	iocIoctl            = 0xc0105512 // _IOWR('U', 18, struct usbdevfs_ioctl) (64-bit)
	iocDisconnect       = 0x5516     // _IO('U', 22)
	iocConnect          = 0x5517     // _IO('U', 23)
)

type bulkTransfer struct {
	Ep      uint32
	Len     uint32
	Timeout uint32 // ms
	_       uint32
	Data    uintptr
}

type usbIoctl struct {
	Ifno      int32
	IoctlCode int32
	Data      uintptr
}

// FindDevice returns /dev/bus/usb/BBB/DDD for the first device with the
// given vendor/product under sysRoot (normally /sys/bus/usb/devices).
func FindDevice(sysRoot string, vendor, product uint16) (string, error) {
	entries, err := os.ReadDir(sysRoot)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		dir := filepath.Join(sysRoot, e.Name())
		if hex(dir, "idVendor") != vendor || hex(dir, "idProduct") != product {
			continue
		}
		bus, err1 := dec(dir, "busnum")
		dev, err2 := dec(dir, "devnum")
		if err1 != nil || err2 != nil {
			continue
		}
		return fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, dev), nil
	}
	return "", fmt.Errorf("no USB device %04x:%04x", vendor, product)
}

func hex(dir, name string) uint16 {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 16, 16)
	if err != nil {
		return 0
	}
	return uint16(v)
}

func dec(dir, name string) (int, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

// Pad pads msg with zeros to PacketSize (error if longer).
func Pad(msg []byte) ([]byte, error) {
	if len(msg) > PacketSize {
		return nil, fmt.Errorf("message is %d bytes, max %d", len(msg), PacketSize)
	}
	out := make([]byte, PacketSize)
	copy(out, msg)
	return out, nil
}

// ParseHex parses "03 b0 18 01" or "03b01801" into bytes.
func ParseHex(s string) ([]byte, error) {
	s = strings.NewReplacer(" ", "", ":", "", ",", "", "0x", "").Replace(strings.ToLower(s))
	if len(s)%2 != 0 {
		return nil, errors.New("odd number of hex digits")
	}
	out := make([]byte, len(s)/2)
	for i := range out {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("bad hex %q", s[2*i:2*i+2])
		}
		out[i] = byte(v)
	}
	return out, nil
}

// Send writes each message (padded to 64 bytes), preceded by the 64-byte
// zero init packet, to iface/endpoint of the device node path.
func Send(path string, iface int, endpoint uint8, msgs [][]byte) (err error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%s: permission denied (install the udev rule: just udev-install)", path)
		}
		return err
	}
	defer func() { _ = f.Close() }()
	fd := f.Fd()

	// Detach the kernel driver (if any) from the command interface; ENODATA
	// means no driver was bound.
	detached := false
	if e := ioctlPtr(fd, iocIoctl, unsafe.Pointer(&usbIoctl{Ifno: int32(iface), IoctlCode: iocDisconnect})); e == nil {
		detached = true
	} else if !errors.Is(e, unix.ENODATA) {
		return fmt.Errorf("detach kernel driver: %w", e)
	}
	defer func() {
		if detached {
			_ = ioctlPtr(fd, iocIoctl, unsafe.Pointer(&usbIoctl{Ifno: int32(iface), IoctlCode: iocConnect}))
		}
	}()
	ifno := uint32(iface)
	if e := ioctlPtr(fd, iocClaimInterface, unsafe.Pointer(&ifno)); e != nil {
		return fmt.Errorf("claim interface %d: %w", iface, e)
	}
	defer func() { _ = ioctlPtr(fd, iocReleaseInterface, unsafe.Pointer(&ifno)) }()

	all := append([][]byte{make([]byte, PacketSize)}, msgs...)
	for i, m := range all {
		buf, perr := Pad(m)
		if perr != nil {
			return perr
		}
		bt := bulkTransfer{Ep: uint32(endpoint), Len: PacketSize, Timeout: 200, Data: uintptr(unsafe.Pointer(&buf[0]))}
		if e := ioctlPtr(fd, iocBulk, unsafe.Pointer(&bt)); e != nil {
			return fmt.Errorf("write packet %d: %w", i, e)
		}
	}
	return nil
}

func ioctlPtr(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, fd, req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}
