package chr_drv
import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
)
const (
	TTY_BUF_SIZE = 1024
	MAX_TTYS     = 8
)
type tty_queue struct {
	data [TTY_BUF_SIZE]byte
	head int
	tail int
	mu   sync.Mutex
}
type tty_struct struct {
	pgrp       int
	stopped    bool
	read_q     tty_queue
	write_q    tty_queue
	secondary  tty_queue
	reader     io.Reader
	writer     io.Writer
	mu         sync.Mutex
}
var tty_table [MAX_TTYS]tty_struct
func tty_init() {
	tty_table[0].reader = os.Stdin
	tty_table[0].writer = os.Stdout
}
func tty_read(channel int, buf []byte) (int, error) {
	if channel < 0 || channel >= MAX_TTYS { return 0, fmt.Errorf("bad tty %d", channel) }
	tty := &tty_table[channel]
	tty.mu.Lock()
	defer tty.mu.Unlock()
	if tty.reader == nil { return 0, fmt.Errorf("tty %d not initialized", channel) }
	r := bufio.NewReader(tty.reader)
	return r.Read(buf)
}
func tty_write(channel int, buf []byte) (int, error) {
	if channel < 0 || channel >= MAX_TTYS { return 0, fmt.Errorf("bad tty %d", channel) }
	tty := &tty_table[channel]
	tty.mu.Lock()
	defer tty.mu.Unlock()
	if tty.writer == nil { return 0, fmt.Errorf("tty %d not initialized", channel) }
	return tty.writer.Write(buf)
}
func con_write(buf []byte) (int, error) {
	return tty_write(0, buf)
}
func Init()                                      { tty_init() }
func TtyRead(ch int, buf []byte) (int, error)    { return tty_read(ch, buf) }
func TtyWrite(ch int, buf []byte) (int, error)   { return tty_write(ch, buf) }
func ConWrite(buf []byte) (int, error)           { return con_write(buf) }
