package chr_drv

import "sync"

const WAKEUP_CHARS = TTY_BUF_SIZE / 4

type SerialPort struct {
	BasePort uint16
	DLAB     byte
	DivisorL byte
	DivisorH byte
	LCR      byte
	MCR      byte
	IER      byte
	mu       sync.Mutex
}

var serialPorts [2]SerialPort

func serialInit(port *SerialPort, base uint16) {
	port.mu.Lock()
	defer port.mu.Unlock()
	port.BasePort = base
	port.DLAB = 0x80
	port.DivisorL = 0x30
	port.DivisorH = 0x00
	port.LCR = 0x03
	port.MCR = 0x0b
	port.IER = 0x0d
}

func RsInit() {
	serialInit(&serialPorts[0], 0x3f8)
	serialInit(&serialPorts[1], 0x2f8)
}

func RsWriteSerial(tty *TtyStruct) {
	tty.mu.Lock()
	defer tty.mu.Unlock()
	if !tty.WriteQ.Empty() {
		for !tty.WriteQ.Empty() {
			_ = tty.WriteQ.Getch()
		}
	}
}
