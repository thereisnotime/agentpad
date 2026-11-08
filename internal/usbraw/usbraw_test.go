package usbraw

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestStructSizesMatchKernel(t *testing.T) {
	if s := unsafe.Sizeof(bulkTransfer{}); s != 24 {
		t.Fatalf("usbdevfs_bulktransfer is %d bytes, want 24", s)
	}
	if s := unsafe.Sizeof(usbIoctl{}); s != 16 {
		t.Fatalf("usbdevfs_ioctl is %d bytes, want 16", s)
	}
}

func TestParseHexAndPad(t *testing.T) {
	b, err := ParseHex("03 b0 18 01")
	if err != nil || len(b) != 4 || b[1] != 0xb0 || b[3] != 1 {
		t.Fatalf("ParseHex = %x, %v", b, err)
	}
	if _, err := ParseHex("0x03b"); err == nil {
		t.Fatal("odd digits accepted")
	}
	p, err := Pad(b)
	if err != nil || len(p) != PacketSize || p[2] != 0x18 || p[63] != 0 {
		t.Fatalf("Pad = %x, %v", p, err)
	}
	if _, err := Pad(make([]byte, 65)); err == nil {
		t.Fatal("oversized packet accepted")
	}
}

func TestFindDevice(t *testing.T) {
	root := t.TempDir()
	write := func(dir, name, v string) {
		_ = os.MkdirAll(filepath.Join(root, dir), 0o755)
		_ = os.WriteFile(filepath.Join(root, dir, name), []byte(v+"\n"), 0o644)
	}
	write("3-5", "idVendor", "1189")
	write("3-5", "idProduct", "8890")
	write("3-5", "busnum", "3")
	write("3-5", "devnum", "4")
	got, err := FindDevice(root, 0x1189, 0x8890)
	if err != nil || got != "/dev/bus/usb/003/004" {
		t.Fatalf("FindDevice = %q, %v", got, err)
	}
	if _, err := FindDevice(root, 0x1189, 0x8842); err == nil {
		t.Fatal("found a device that isn't there")
	}
}
