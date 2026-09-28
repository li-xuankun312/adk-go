package chr_drv
import (
	"fmt"
	"io"
	"net"
)
type serial_struct struct {
	port int
	conn net.Conn
}
var serial_table [2]serial_struct
func rs_init() {}
func rs_write(channel int, buf []byte) (int, error) {
	if channel < 0 || channel >= 2 { return 0, fmt.Errorf("bad serial %d", channel) }
	s := &serial_table[channel]
	if s.conn == nil { return 0, fmt.Errorf("serial %d not connected", channel) }
	return s.conn.Write(buf)
}
func rs_read(channel int, buf []byte) (int, error) {
	if channel < 0 || channel >= 2 { return 0, fmt.Errorf("bad serial %d", channel) }
	s := &serial_table[channel]
	if s.conn == nil { return 0, io.EOF }
	return s.conn.Read(buf)
}
func RsInit()                                     { rs_init() }
func RsWrite(ch int, buf []byte) (int, error)     { return rs_write(ch, buf) }
func RsRead(ch int, buf []byte) (int, error)      { return rs_read(ch, buf) }
