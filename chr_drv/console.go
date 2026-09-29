// chr_drv/console.go — ported from linux-0.11/kernel/chr_drv/console.c
// (C) 1991 Linus Torvalds
//
// Console output. In the original, this drives VGA hardware.
// In the Go port, we simulate the console by writing to a
// configurable io.Writer (defaults to os.Stdout).
package chr_drv

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// Console state
const (
	NPAR       = 16
	VIDEO_COLS = 80
	VIDEO_LINES = 25
	VIDEO_SIZE = VIDEO_COLS * VIDEO_LINES
)

var (
	conMu      sync.Mutex
	conWriter  io.Writer = os.Stdout
	// VGA-like state
	originX    int
	originY    int
	top        int
	bottom     int = VIDEO_LINES
	state      int // 0=normal, 1=ESC, 2=ESC-[, 3=ESC-[?, 4=ESC-(
	npar       int
	par        [NPAR]uint32
	ques       int
	savedX     int
	savedY     int
	attr       byte = 0x07 // default: white on black
	// Screen buffer (for completeness)
	screenBuf  [VIDEO_SIZE * 2]byte
)

// SetConsoleWriter allows redirecting console output
func SetConsoleWriter(w io.Writer) {
	conMu.Lock()
	conWriter = w
	conMu.Unlock()
}

// console.c: gotoxy — set cursor position
func gotoxy(newX, newY int) {
	if newX < 0 { newX = 0 }
	if newX >= VIDEO_COLS { newX = VIDEO_COLS - 1 }
	if newY < 0 { newY = 0 }
	if newY >= VIDEO_LINES { newY = VIDEO_LINES - 1 }
	originX = newX
	originY = newY
}

// console.c: scroll up
func scrUp() {
	// In VGA, this shifts video memory. For Go, we just reset state.
	originY = top
	originX = 0
}

// console.c: scroll down
func scrDn() {
	originY = bottom - 1
	originX = 0
}

// console.c: lf — line feed
func lf() {
	originY++
	if originY >= bottom {
		scrUp()
		originY = bottom - 1
	}
}

// console.c: cr — carriage return
func cr() { originX = 0 }

// console.c: del — delete character
func del() {
	if originX > 0 { originX-- }
}

// console.c: csi_J — erase display
func csiJ(vpar int) {
	// 0=erase from cursor to end, 1=from start to cursor, 2=whole display
	// Simplified: no-op in Go port
}

// console.c: csi_K — erase line
func csiK(vpar int) {}

// console.c: csi_m — set graphics rendition
func csiM() {
	// ANSI color/attribute codes → just track attr
	for i := 0; i <= npar; i++ {
		switch par[i] {
		case 0: attr = 0x07
		case 1: attr = 0x0F // bold
		case 7: attr = 0x70 // reverse
		}
	}
}

// console.c: set_cursor
func setCursor() {
	// In VGA: update CRTC cursor registers. No-op in Go.
}

// console.c lines 350-710: con_write — the main console output function
// Processes characters including ANSI escape sequences.
func ConWriteConsole(tty *TtyStruct) {
	conMu.Lock()
	defer conMu.Unlock()

	for !tty.WriteQ.Empty() {
		c := tty.WriteQ.Getch()
		switch state {
		case 0: // Normal state
			switch c {
			case 7: // BEL
				// beep
			case 8: // BS
				del()
			case 9: // TAB
				originX = (originX + 8) &^ 7
				if originX >= VIDEO_COLS {
					originX -= VIDEO_COLS
					lf()
				}
			case 10, 11, 12: // LF, VT, FF
				lf()
				fmt.Fprint(conWriter, "\n")
			case 13: // CR
				cr()
			case 27: // ESC
				state = 1
			case 127: // DEL
				del()
			default:
				if c >= 32 {
					fmt.Fprint(conWriter, string(rune(c)))
					originX++
					if originX >= VIDEO_COLS {
						originX = 0
						lf()
					}
				}
			}
		case 1: // After ESC
			state = 0
			switch c {
			case '[':
				state = 2
				npar = 0
				for i := range par { par[i] = 0 }
				ques = 0
			case 'E': lf(); cr()
			case 'M': // reverse line feed
				if originY > top { originY-- }
			case 'D': lf()
			case 'Z': // identify terminal
			case '7': savedX = originX; savedY = originY
			case '8': gotoxy(savedX, savedY)
			case '(': state = 4
			case ')': state = 4
			}
		case 2: // After ESC [
			if c == '?' { ques = 1; break }
			if c >= '0' && c <= '9' {
				par[npar] = par[npar]*10 + uint32(c-'0')
				break
			}
			if c == ';' {
				npar++
				if npar >= NPAR { npar = NPAR - 1 }
				break
			}
			state = 0
			switch c {
			case 'G', '`': // horizontal absolute
				if par[0] > 0 { par[0]-- }
				gotoxy(int(par[0]), originY)
			case 'A': // cursor up
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX, originY-n)
			case 'B', 'e': // cursor down
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX, originY+n)
			case 'C', 'a': // cursor right
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX+n, originY)
			case 'D': // cursor left
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX-n, originY)
			case 'H', 'f': // cursor position
				gotoxy(int(par[1])-1, int(par[0])-1)
			case 'J': csiJ(int(par[0]))
			case 'K': csiK(int(par[0]))
			case 'm': csiM()
			case 'r': // scroll region
				if par[0] != 0 { par[0]-- }
				if par[1] == 0 { par[1] = uint32(VIDEO_LINES) }
				if par[0] < par[1] && par[1] <= uint32(VIDEO_LINES) {
					top = int(par[0]); bottom = int(par[1])
				}
			case 's': savedX = originX; savedY = originY
			case 'u': gotoxy(savedX, savedY)
			}
		case 4: // After ESC ( or ESC )
			state = 0
		}
	}
}

// con_init: initialize console
func ConInit() {
	gotoxy(0, 0)
	top = 0
	bottom = VIDEO_LINES
	state = 0
	attr = 0x07
}

// Override the stub ConWrite in tty_io.go
func init() {
	// Wire console write function
	TtyTable[0].WriteFn = ConWriteConsole
}
