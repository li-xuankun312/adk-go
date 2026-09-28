// kernel/pipe.go
//
// Go port of linux-0.11 fs/pipe.c
// Implements inter-process communication via pipes.
//
// In linux-0.11, pipes use a shared memory page with head/tail
// pointers and sleep/wake for synchronization. In Go, we use
// channels for the same semantics but with proper goroutine safety.

package kernel

import "fmt"

// PIPE_SIZE is the buffer capacity.
// Linux-0.11 uses PAGE_SIZE (4096).
const PIPE_SIZE = 4096

// pipe_struct represents a pipe between two processes.
// Replaces the m_inode-based pipe in linux-0.11.
type pipe_struct struct {
	buf     chan []byte // buffered channel for data transfer
	readers int        // reference count of readers (i_count in linux-0.11)
	writers int        // reference count of writers
}

// sys_pipe creates a pipe and returns the read and write ends.
//
// Port of linux-0.11 sys_pipe():
//
//	int sys_pipe(unsigned long * fildes)
//	{
//	    // allocate 2 file structs, 2 file descriptors, 1 pipe inode
//	    f[0]->f_mode = 1;    /* read */
//	    f[1]->f_mode = 2;    /* write */
//	    return 0;
//	}
//
// In Go, we return a pipe_struct instead of file descriptors.
func sys_pipe() (*pipe_struct, error) {
	p := &pipe_struct{
		buf:     make(chan []byte, PIPE_SIZE),
		readers: 1,
		writers: 1,
	}
	return p, nil
}

// read_pipe reads from a pipe.
//
// Port of linux-0.11 read_pipe():
//
//	int read_pipe(struct m_inode * inode, char * buf, int count)
//	{
//	    while (count>0) {
//	        while (!(size=PIPE_SIZE(*inode))) {
//	            wake_up(&inode->i_wait);
//	            if (inode->i_count != 2) return read;
//	            sleep_on(&inode->i_wait);
//	        }
//	        // copy data from pipe to buf
//	    }
//	    wake_up(&inode->i_wait);
//	    return read;
//	}
//
// In Go, we receive from the channel. If the writer has closed
// (writers == 0), we return what we have.
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

// write_pipe writes to a pipe.
//
// Port of linux-0.11 write_pipe():
//
//	int write_pipe(struct m_inode * inode, char * buf, int count)
//	{
//	    while (count>0) {
//	        while (!(size=(PAGE_SIZE-1)-PIPE_SIZE(*inode))) {
//	            wake_up(&inode->i_wait);
//	            if (inode->i_count != 2) {
//	                current->signal |= (1<<(SIGPIPE-1));
//	                return written?written:-1;
//	            }
//	            sleep_on(&inode->i_wait);
//	        }
//	        // copy data from buf to pipe
//	    }
//	    wake_up(&inode->i_wait);
//	    return written;
//	}
func write_pipe(p *pipe_struct, data []byte) error {
	if p == nil {
		return fmt.Errorf("write_pipe: nil pipe")
	}
	if p.readers == 0 {
		// No readers — SIGPIPE equivalent
		return fmt.Errorf("write_pipe: broken pipe (SIGPIPE)")
	}
	p.buf <- data
	return nil
}

// close_pipe closes one end of a pipe.
// reader=true closes the read end, reader=false closes the write end.
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
