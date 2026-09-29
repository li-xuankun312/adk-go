// chr_drv/serial.go — ported from linux-0.11/kernel/chr_drv/serial.c
// (C) 1991 Linus Torvalds
//
// Serial port driver. In the original, this drives 8250/16450 UART hardware.
// In the Go port, serial I/O is stubbed since there's no physical UART.
package chr_drv

// serial.c: rs_init — initialize serial ports
func RsInit() {
	// In the original: set up COM1 (0x3F8) and COM2 (0x2F8) interrupts
	// In Go: no-op — serial functionality is unused
}

// serial.c: rs_write — write to serial port
// This is referenced as the write function for tty_table[1] and tty_table[2]
// (already defined as RsWrite in tty_io.go)

// serial.c: rs1_interrupt, rs2_interrupt — interrupt handlers
// No-op in Go port since there are no real UARTs
