package fs
func get_super(dev int) *super_block {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_SUPER; i++ {
		if super_block_table[i].s_dev == uint16(dev) {
			return &super_block_table[i]
		}
	}
	return nil
}
func put_super(dev int) {
	sb := get_super(dev)
	if sb == nil { return }
	sb.s_dev = 0
}
func read_super(dev int) *super_block {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_SUPER; i++ {
		if super_block_table[i].s_dev == 0 {
			sb := &super_block_table[i]
			sb.s_dev = uint16(dev)
			sb.s_magic = 0x137F
			return sb
		}
	}
	return nil
}
func mount_root() {
	sb := read_super(ROOT_DEV)
	if sb == nil { return }
	sb.s_isup = get_empty_inode()
	sb.s_imount = sb.s_isup
}
func GetSuper(dev int) *super_block { return get_super(dev) }
func MountRoot()                    { mount_root() }
