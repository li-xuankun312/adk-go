package chr_drv

import (
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	NPAR       = 16
	VIDEO_COLS = 80
	VIDEO_LINES = 25
	VIDEO_SIZE = VIDEO_COLS * VIDEO_LINES
)

var (
	conMu      sync.Mutex
	conWriter  io.Writer = os.Stdout
	originX    int
	originY    int
	top        int
	bottom     int = VIDEO_LINES
	state      int
	npar       int
	par        [NPAR]uint32
	ques       int
	savedX     int
	savedY     int
	attr       byte = 0x07
	screenBuf  [VIDEO_SIZE * 2]byte
)

func SetConsoleWriter(w io.Writer) {
	conMu.Lock()
	conWriter = w
	conMu.Unlock()
}

func gotoxy(newX, newY int) {
	if newX < 0 { newX = 0 }
	if newX >= VIDEO_COLS { newX = VIDEO_COLS - 1 }
	if newY < 0 { newY = 0 }
	if newY >= VIDEO_LINES { newY = VIDEO_LINES - 1 }
	originX = newX
	originY = newY
}

func scrUp() {
	originY = top
	originX = 0
}

func scrDn() {
	originY = bottom - 1
	originX = 0
}

func lf() {
	originY++
	if originY >= bottom {
		scrUp()
		originY = bottom - 1
	}
}

func cr() { originX = 0 }

func del() {
	if originX > 0 { originX-- }
}

func csiJ(vpar int) {
}

func csiK(vpar int) {}

func csiM() {
	for i := 0; i <= npar; i++ {
		switch par[i] {
		case 0: attr = 0x07
		case 1: attr = 0x0F
		case 7: attr = 0x70
		}
	}
}

func setCursor() {
}

func ConWriteConsole(tty *TtyStruct) {
	conMu.Lock()
	defer conMu.Unlock()

	for !tty.WriteQ.Empty() {
		c := tty.WriteQ.Getch()
		switch state {
		case 0:
			switch c {
			case 7:
			case 8:
				del()
			case 9:
				originX = (originX + 8) &^ 7
				if originX >= VIDEO_COLS {
					originX -= VIDEO_COLS
					lf()
				}
			case 10, 11, 12:
				lf()
				fmt.Fprint(conWriter, "\n")
			case 13:
				cr()
			case 27:
				state = 1
			case 127:
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
		case 1:
			state = 0
			switch c {
			case '[':
				state = 2
				npar = 0
				for i := range par { par[i] = 0 }
				ques = 0
			case 'E': lf(); cr()
			case 'M':
				if originY > top { originY-- }
			case 'D': lf()
			case 'Z':
			case '7': savedX = originX; savedY = originY
			case '8': gotoxy(savedX, savedY)
			case '(': state = 4
			case ')': state = 4
			}
		case 2:
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
			case 'G', '`':
				if par[0] > 0 { par[0]-- }
				gotoxy(int(par[0]), originY)
			case 'A':
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX, originY-n)
			case 'B', 'e':
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX, originY+n)
			case 'C', 'a':
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX+n, originY)
			case 'D':
				n := int(par[0]); if n == 0 { n = 1 }
				gotoxy(originX-n, originY)
			case 'H', 'f':
				gotoxy(int(par[1])-1, int(par[0])-1)
			case 'J': csiJ(int(par[0]))
			case 'K': csiK(int(par[0]))
			case 'm': csiM()
			case 'r':
				if par[0] != 0 { par[0]-- }
				if par[1] == 0 { par[1] = uint32(VIDEO_LINES) }
				if par[0] < par[1] && par[1] <= uint32(VIDEO_LINES) {
					top = int(par[0]); bottom = int(par[1])
				}
			case 's': savedX = originX; savedY = originY
			case 'u': gotoxy(savedX, savedY)
			}
		case 4:
			state = 0
		}
	}
}

func ConInit() {
	gotoxy(0, 0)
	top = 0
	bottom = VIDEO_LINES
	state = 0
	attr = 0x07
}

func init() {
	TtyTable[0].WriteFn = ConWriteConsole
}
