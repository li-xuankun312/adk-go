package fs
import "time"
func sync_inodes() {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_INODE; i++ {
		if inode_table[i].i_dirt != 0 {
			inode_table[i].i_dirt = 0
		}
	}
}
func bmap(inode *m_inode, block int) int {
	if inode == nil || block < 0 { return 0 }
	if block < 7 { return int(inode.i_zone[block]) }
	return 0
}
func create_block(inode *m_inode, block int) int {
	if inode == nil { return 0 }
	if block < 7 {
		if inode.i_zone[block] == 0 {
			inode.i_zone[block] = uint16(new_block(int(inode.i_dev)))
		}
		return int(inode.i_zone[block])
	}
	return 0
}
func truncate(inode *m_inode) {
	if inode == nil { return }
	for i := 0; i < 9; i++ {
		if inode.i_zone[i] != 0 {
			free_block(int(inode.i_dev), int(inode.i_zone[i]))
			inode.i_zone[i] = 0
		}
	}
	inode.i_size = 0
	inode.i_dirt = 1
	inode.i_mtime = uint32(time.Now().Unix())
}
func SyncInodes()                               { sync_inodes() }
func Bmap(inode *m_inode, block int) int         { return bmap(inode, block) }
func CreateBlock(inode *m_inode, block int) int   { return create_block(inode, block) }
func Truncate(inode *m_inode)                    { truncate(inode) }
