// include/head.go — ported from linux-0.11/include/linux/head.h
package include

// head.h line 4-6: struct desc_struct
// Original: typedef struct desc_struct { unsigned long a,b; } desc_table[256];
// In the Go port, we don't have a GDT/IDT but we keep the structure
// for task_struct.ldt[3] compatibility.
type DescStruct struct {
	A uint32 // unsigned long a
	B uint32 // unsigned long b
}

// head.h line 8: extern unsigned long pg_dir[1024];
// In the Go port, page directory is simulated — see mm package.
// We declare it here for type compatibility.
var PgDir [1024]uint32

// head.h lines 11-18: GDT/LDT segment indices
const (
	GDT_NUL  = 0
	GDT_CODE = 1
	GDT_DATA = 2
	GDT_TMP  = 3

	LDT_NUL  = 0
	LDT_CODE = 1
	LDT_DATA = 2
)
