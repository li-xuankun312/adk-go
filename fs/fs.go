package fs
import (
	"sync"
)
const (
	READ  = 0
	WRITE = 1
	NAME_LEN  = 14
	ROOT_INO  = 1
	NR_OPEN   = 20
	NR_INODE  = 32
	NR_FILE   = 64
	NR_SUPER  = 8
	BLOCK_SIZE = 1024
)
func MAJOR(a int) int { return int(uint(a) >> 8) }
func MINOR(a int) int { return a & 0xff }
type buffer_head struct {
	b_data     []byte
	b_blocknr  uint64
	b_dev      uint16
	b_uptodate byte
	b_dirt     byte
	b_count    byte
	b_lock     byte
}
type d_inode struct {
	i_mode   uint16
	i_uid    uint16
	i_size   uint32
	i_time   uint32
	i_gid    byte
	i_nlinks byte
	i_zone   [9]uint16
}
type m_inode struct {
	i_mode   uint16
	i_uid    uint16
	i_size   uint32
	i_mtime  uint32
	i_gid    byte
	i_nlinks byte
	i_zone   [9]uint16
	i_atime  uint32
	i_ctime  uint32
	i_dev    uint16
	i_num    uint16
	i_count  uint16
	i_lock   byte
	i_dirt   byte
	i_pipe   byte
	i_mount  byte
	i_seek   byte
	i_update byte
	mu       sync.Mutex
}
type file struct {
	f_mode  uint16
	f_flags uint16
	f_count uint16
	f_inode *m_inode
	f_pos   int64
}
type super_block struct {
	s_ninodes        uint16
	s_nzones         uint16
	s_imap_blocks    uint16
	s_zmap_blocks    uint16
	s_firstdatazone  uint16
	s_log_zone_size  uint16
	s_max_size       uint32
	s_magic          uint16
	s_dev            uint16
	s_isup           *m_inode
	s_imount         *m_inode
	s_time           uint32
	s_lock           byte
	s_rd_only        byte
	s_dirt           byte
}
type dir_entry struct {
	inode uint16
	name  [NAME_LEN]byte
}
var (
	inode_table [NR_INODE]m_inode
	file_table  [NR_FILE]file
	super_block_table [NR_SUPER]super_block
	ROOT_DEV    int
	mu          sync.Mutex
)
