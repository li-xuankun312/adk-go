// include/fs.go — ported from linux-0.11/include/linux/fs.h
// Line-for-line translation of all filesystem structures and constants.
package include

// fs.h lines 11-22: device major numbers
// 0 - unused (nodev)
// 1 - /dev/mem
// 2 - /dev/fd
// 3 - /dev/hd
// 4 - /dev/ttyx
// 5 - /dev/tty
// 6 - /dev/lp
// 7 - unnamed pipes

// fs.h line 24
func IS_SEEKABLE(x int) bool { return x >= 1 && x <= 3 }

// fs.h lines 26-29
const (
	READ   = 0
	WRITE  = 1
	READA  = 2 // read-ahead — don't pause
	WRITEA = 3 // "write-ahead" — silly, but somewhat useful
)

// fs.h lines 33-34
func MAJOR(a uint32) uint32 { return a >> 8 }
func MINOR(a uint32) uint32 { return a & 0xff }

// fs.h lines 36-53
const (
	NAME_LEN       = 14
	ROOT_INO       = 1
	I_MAP_SLOTS    = 8
	Z_MAP_SLOTS    = 8
	SUPER_MAGIC    = 0x137F
	NR_OPEN        = 20
	NR_INODE       = 32
	NR_FILE        = 64
	NR_SUPER       = 8
	NR_HASH        = 307
	BLOCK_SIZE     = 1024
	BLOCK_SIZE_BITS = 10
)

// fs.h line 55-56: derived constants
// INODES_PER_BLOCK = BLOCK_SIZE / sizeof(d_inode) — computed at init
// DIR_ENTRIES_PER_BLOCK = BLOCK_SIZE / sizeof(dir_entry)

// fs.h lines 58-64: pipe macros using i_zone[0] and i_zone[1]
func PIPE_HEAD(inode *MInode) uint16 { return inode.IZone[0] }
func PIPE_TAIL(inode *MInode) uint16 { return inode.IZone[1] }
func PIPE_SIZE(inode *MInode) uint16 {
	return (inode.IZone[0] - inode.IZone[1]) & (PAGE_SIZE - 1)
}
func PIPE_EMPTY(inode *MInode) bool { return inode.IZone[0] == inode.IZone[1] }
func PIPE_FULL(inode *MInode) bool {
	return PIPE_SIZE(inode) == (PAGE_SIZE - 1)
}

// fs.h line 66: typedef char buffer_block[BLOCK_SIZE];
type BufferBlock [BLOCK_SIZE]byte

// fs.h lines 68-81: struct buffer_head
// Original:
//   struct buffer_head {
//       char * b_data;
//       unsigned long b_blocknr;
//       unsigned short b_dev;
//       unsigned char b_uptodate;
//       unsigned char b_dirt;
//       unsigned char b_count;
//       unsigned char b_lock;
//       struct task_struct * b_wait;
//       struct buffer_head * b_prev;
//       struct buffer_head * b_next;
//       struct buffer_head * b_prev_free;
//       struct buffer_head * b_next_free;
//   };
type BufferHead struct {
	BData      []byte      // char * b_data (pointer to data block, 1024 bytes)
	BBlocknr   uint32      // unsigned long b_blocknr
	BDev       uint16      // unsigned short b_dev (0 = free)
	BUptodate  uint8       // unsigned char b_uptodate
	BDirt      uint8       // unsigned char b_dirt (0-clean, 1-dirty)
	BCount     uint8       // unsigned char b_count (users using this block)
	BLock      uint8       // unsigned char b_lock (0-ok, 1-locked)
	BWait      *TaskStruct // struct task_struct * b_wait
	BPrev      *BufferHead // struct buffer_head * b_prev
	BNext      *BufferHead // struct buffer_head * b_next
	BPrevFree  *BufferHead // struct buffer_head * b_prev_free
	BNextFree  *BufferHead // struct buffer_head * b_next_free
}

// fs.h lines 83-91: struct d_inode (on-disk inode)
type DInode struct {
	IMode  uint16    // unsigned short i_mode
	IUid   uint16    // unsigned short i_uid
	ISize  uint32    // unsigned long i_size
	ITime  uint32    // unsigned long i_time
	IGid   uint8     // unsigned char i_gid
	INlinks uint8    // unsigned char i_nlinks
	IZone  [9]uint16 // unsigned short i_zone[9]
}

// fs.h lines 93-114: struct m_inode (in-memory inode)
type MInode struct {
	IMode   uint16    // unsigned short i_mode
	IUid    uint16    // unsigned short i_uid
	ISize   uint32    // unsigned long i_size
	IMtime  uint32    // unsigned long i_mtime
	IGid    uint8     // unsigned char i_gid
	INlinks uint8     // unsigned char i_nlinks
	IZone   [9]uint16 // unsigned short i_zone[9]
	// these are in memory also
	IWait   *TaskStruct // struct task_struct * i_wait
	IAtime  uint32      // unsigned long i_atime
	ICtime  uint32      // unsigned long i_ctime
	IDev    uint16      // unsigned short i_dev
	INum    uint16      // unsigned short i_num
	ICount  uint16      // unsigned short i_count
	ILock   uint8       // unsigned char i_lock
	IDirt   uint8       // unsigned char i_dirt
	IPipe   uint8       // unsigned char i_pipe
	IMount  uint8       // unsigned char i_mount
	ISeek   uint8       // unsigned char i_seek
	IUpdate uint8       // unsigned char i_update
}

// fs.h lines 116-122: struct file
type File struct {
	FMode  uint16  // unsigned short f_mode
	FFlags uint16  // unsigned short f_flags
	FCount uint16  // unsigned short f_count
	FInode *MInode // struct m_inode * f_inode
	FPos   int64   // off_t f_pos
}

// fs.h lines 124-144: struct super_block
type SuperBlock struct {
	SNinodes       uint16                // unsigned short s_ninodes
	SNzones        uint16                // unsigned short s_nzones
	SImapBlocks    uint16                // unsigned short s_imap_blocks
	SZmapBlocks    uint16                // unsigned short s_zmap_blocks
	SFirstdatazone uint16                // unsigned short s_firstdatazone
	SLogZoneSize   uint16                // unsigned short s_log_zone_size
	SMaxSize       uint32                // unsigned long s_max_size
	SMagic         uint16                // unsigned short s_magic
	// These are only in memory
	SImap [I_MAP_SLOTS]*BufferHead       // struct buffer_head * s_imap[8]
	SZmap [Z_MAP_SLOTS]*BufferHead       // struct buffer_head * s_zmap[8]
	SDev   uint16                        // unsigned short s_dev
	SIsup  *MInode                       // struct m_inode * s_isup
	SImount *MInode                      // struct m_inode * s_imount
	STime  uint32                        // unsigned long s_time
	SWait  *TaskStruct                   // struct task_struct * s_wait
	SLock  uint8                         // unsigned char s_lock
	SRdOnly uint8                        // unsigned char s_rd_only
	SDirt  uint8                         // unsigned char s_dirt
}

// fs.h lines 146-155: struct d_super_block (on-disk super block)
type DSuperBlock struct {
	SNinodes       uint16
	SNzones        uint16
	SImapBlocks    uint16
	SZmapBlocks    uint16
	SFirstdatazone uint16
	SLogZoneSize   uint16
	SMaxSize       uint32
	SMagic         uint16
}

// fs.h lines 157-160: struct dir_entry
type DirEntry struct {
	Inode uint16         // unsigned short inode
	Name  [NAME_LEN]byte // char name[NAME_LEN]
}

// fs.h lines 162-166: global tables (declared here, defined in fs package)
// extern struct m_inode inode_table[NR_INODE];
// extern struct file file_table[NR_FILE];
// extern struct super_block super_block[NR_SUPER];
// extern struct buffer_head * start_buffer;
// extern int nr_buffers;

// fs.h line 198: extern int ROOT_DEV;
var ROOT_DEV int
