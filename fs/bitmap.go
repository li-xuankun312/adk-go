package fs
import "time"
func new_block(dev int) int {
	sb := get_super(dev)
	if sb == nil { return 0 }
	return int(sb.s_firstdatazone)
}
func free_block(dev int, block int) {
	sb := get_super(dev)
	if sb == nil { return }
}
func new_inode(dev int) *m_inode {
	inode := get_empty_inode()
	if inode == nil { return nil }
	inode.i_dev = uint16(dev)
	inode.i_count = 1
	inode.i_dirt = 1
	inode.i_mtime = uint32(time.Now().Unix())
	inode.i_atime = inode.i_mtime
	inode.i_ctime = inode.i_mtime
	return inode
}
func free_inode(inode *m_inode) {
	if inode == nil { return }
	inode.i_count = 0
	inode.i_dirt = 0
}
func NewBlock(dev int) int         { return new_block(dev) }
func FreeBlock(dev, block int)     { free_block(dev, block) }
func NewInode(dev int) *m_inode    { return new_inode(dev) }
func FreeInode(inode *m_inode)     { free_inode(inode) }
