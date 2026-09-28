package fs
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)
func namei(pathname string) (*m_inode, error) {
	pathname = filepath.Clean(pathname)
	info, err := os.Stat(pathname)
	if err != nil { return nil, err }
	inode := get_empty_inode()
	if inode == nil { return nil, fmt.Errorf("namei: no free inodes") }
	inode.i_size = uint32(info.Size())
	inode.i_mode = uint16(info.Mode())
	inode.i_mtime = uint32(info.ModTime().Unix())
	inode.i_count = 1
	return inode, nil
}
func open_namei(pathname string, flag int, mode int) (*m_inode, error) {
	if flag&syscall.O_CREAT != 0 {
		f, err := os.OpenFile(pathname, flag, os.FileMode(mode))
		if err != nil { return nil, err }
		f.Close()
	}
	return namei(pathname)
}
func get_empty_inode() *m_inode {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_INODE; i++ {
		if inode_table[i].i_count == 0 {
			inode_table[i] = m_inode{}
			return &inode_table[i]
		}
	}
	return nil
}
func iput(inode *m_inode) {
	if inode == nil { return }
	inode.mu.Lock()
	defer inode.mu.Unlock()
	if inode.i_count > 0 { inode.i_count-- }
}
func iget(dev int, nr int) *m_inode {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_INODE; i++ {
		if inode_table[i].i_dev == uint16(dev) && inode_table[i].i_num == uint16(nr) {
			inode_table[i].i_count++
			return &inode_table[i]
		}
	}
	return nil
}
func dir_namei(pathname string) (string, string) {
	return filepath.Dir(pathname), filepath.Base(pathname)
}
func find_entry(dirpath string, name string) (*dir_entry, error) {
	entries, err := os.ReadDir(dirpath)
	if err != nil { return nil, err }
	for _, e := range entries {
		if e.Name() == name {
			de := &dir_entry{}
			copy(de.name[:], []byte(e.Name()))
			de.inode = 1
			return de, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", name)
}
func Namei(pathname string) (*m_inode, error) { return namei(pathname) }
func OpenNamei(pathname string, flag int, mode int) (*m_inode, error) { return open_namei(pathname, flag, mode) }
func Iput(inode *m_inode)                    { iput(inode) }
func _ () { _ = strings.Contains }
