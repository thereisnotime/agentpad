package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/thereisnotime/agentpad/internal/usbraw"
)

// LED lab: experimental raw LED commands for 1189:8890 boards, to discover
// effects or colour control beyond the documented `led <mode>` command.
//
//	agentpad led-lab list               show phases and steps
//	agentpad led-lab run <phase> [-secs N] [-from N]
//	agentpad led-lab step <phase> <n>   run one step and leave it on
//	agentpad led-raw <hex> [<hex>...]   send raw packets (each padded to 64 bytes)

type labStep struct {
	label string
	msgs  [][]byte
}

// wrap8890 frames LED register writes the way the 8890 LED command does:
// 03 a1 01, the writes, 03 aa a1.
func wrap8890(body ...[]byte) [][]byte {
	out := [][]byte{{0x03, 0xa1, 0x01}}
	out = append(out, body...)
	return append(out, []byte{0x03, 0xaa, 0xa1})
}

func ledMode(mode byte, extra ...byte) []byte {
	return append([]byte{0x03, 0xb0, 0x18, mode}, extra...)
}

func labPhases() map[string][]labStep {
	p := map[string][]labStep{}
	for m := 0; m <= 15; m++ {
		p["modes"] = append(p["modes"], labStep{fmt.Sprintf("mode %d", m), wrap8890(ledMode(byte(m)))})
	}
	colours := []struct {
		name    string
		r, g, b byte
	}{{"red", 0xff, 0, 0}, {"green", 0, 0xff, 0}, {"blue", 0, 0, 0xff}, {"white", 0xff, 0xff, 0xff}}
	for _, mode := range []byte{1, 2, 3} {
		for _, c := range colours {
			p["rgb"] = append(p["rgb"], labStep{fmt.Sprintf("mode %d + RGB %s (bytes 4-6)", mode, c.name), wrap8890(ledMode(mode, c.r, c.g, c.b))})
		}
	}
	for _, mode := range []byte{1, 2, 3} {
		for idx := byte(1); idx <= 7; idx++ {
			p["index"] = append(p["index"], labStep{fmt.Sprintf("mode %d + colour index %d (byte 4)", mode, idx), wrap8890(ledMode(mode, idx))})
		}
	}
	for _, mode := range []byte{1, 2} {
		for _, b := range []byte{0x10, 0x40, 0x80, 0xff} {
			p["byte5"] = append(p["byte5"], labStep{fmt.Sprintf("mode %d + byte 5 = %#02x", mode, b), wrap8890(ledMode(mode, 0, b))})
		}
	}
	return p
}

var labOrder = []string{"modes", "rgb", "index", "byte5"}

func runLedLab(args []string) int {
	if len(args) == 0 || args[0] == "list" {
		phases := labPhases()
		for _, name := range labOrder {
			fmt.Printf("%s:\n", name)
			for i, s := range phases[name] {
				fmt.Printf("  %2d  %s\n", i, s.label)
			}
		}
		return 0
	}
	dev, err := usbraw.FindDevice("/sys/bus/usb/devices", 0x1189, 0x8890)
	if err != nil {
		fmt.Fprintln(os.Stderr, "led-lab:", err)
		return 1
	}
	send := func(msgs [][]byte) error {
		return usbraw.Send(dev, usbraw.DefaultInterface, usbraw.DefaultEndpoint, msgs)
	}
	switch args[0] {
	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		secs := fs.Float64("secs", 3, "seconds per step")
		from := fs.Int("from", 0, "first step")
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: agentpad led-lab run <phase> [-secs N] [-from N]")
			return 2
		}
		steps, ok := labPhases()[args[1]]
		if !ok {
			fmt.Fprintln(os.Stderr, "unknown phase", args[1])
			return 2
		}
		if err := fs.Parse(args[2:]); err != nil {
			return 2
		}
		for i := *from; i < len(steps); i++ {
			fmt.Printf("[%s %2d] %s\n", args[1], i, steps[i].label)
			if err := send(steps[i].msgs); err != nil {
				fmt.Fprintln(os.Stderr, "  error:", err)
				return 1
			}
			time.Sleep(time.Duration(*secs * float64(time.Second)))
		}
		fmt.Println("[done] LEDs off (mode 0)")
		_ = send(wrap8890(ledMode(0)))
		return 0
	case "step":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: agentpad led-lab step <phase> <n>")
			return 2
		}
		steps := labPhases()[args[1]]
		var n int
		if _, err := fmt.Sscan(args[2], &n); err != nil || n < 0 || n >= len(steps) {
			fmt.Fprintln(os.Stderr, "bad step")
			return 2
		}
		fmt.Printf("[%s %d] %s\n", args[1], n, steps[n].label)
		if err := send(steps[n].msgs); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "usage: agentpad led-lab [list | run <phase> | step <phase> <n>]")
	return 2
}

func runLedRaw(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentpad led-raw <hex> [<hex>...]   e.g. led-raw 03a101 03b01801 03aaa1")
		return 2
	}
	var msgs [][]byte
	for _, a := range args {
		b, err := usbraw.ParseHex(a)
		if err != nil {
			fmt.Fprintln(os.Stderr, "led-raw:", err)
			return 2
		}
		msgs = append(msgs, b)
	}
	dev, err := usbraw.FindDevice("/sys/bus/usb/devices", 0x1189, 0x8890)
	if err == nil {
		err = usbraw.Send(dev, usbraw.DefaultInterface, usbraw.DefaultEndpoint, msgs)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "led-raw:", err)
		return 1
	}
	return 0
}
