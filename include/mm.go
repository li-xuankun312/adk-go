// include/mm.go — ported from linux-0.11/include/linux/mm.h
package include

// mm.h line 24
const PAGE_SIZE = 4096

// mm.h lines 26-28: function declarations
// get_free_page, put_page, free_page are implemented in mm/memory.go.
// We declare the interfaces here; actual functions live in the mm package.
