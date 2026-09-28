package kernel
import "fmt"
const PIPE_SIZE = 4096
type pipe_struct struct {
	buf     chan []byte // buffered channel for data transfer
	readers int        // reference count of readers (i_count in linux-0.11)
	writers int        // reference count of writers
}
func sys_pipe() (*pipe_struct, error) {
	p := &pipe_struct{
		buf:     make(chan []byte, PIPE_SIZE),
		readers: 1,
		writers: 1,
	}
	return p, nil
}
func read_pipe(p *pipe_struct) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("read_pipe: nil pipe")
	}
	data, ok := <-p.buf
	if !ok {
		return nil, fmt.Errorf("read_pipe: pipe closed")
	}
	return data, nil
}
func write_pipe(p *pipe_struct, data []byte) error {
	if p == nil {
		return fmt.Errorf("write_pipe: nil pipe")
	}
	if p.readers == 0 {
		return fmt.Errorf("write_pipe: broken pipe (SIGPIPE)")
	}
	p.buf <- data
	return nil
}
func close_pipe(p *pipe_struct, reader bool) {
	if p == nil {
		return
	}
	if reader {
		p.readers--
	} else {
		p.writers--
		if p.writers == 0 {
			close(p.buf)
		}
	}
}
