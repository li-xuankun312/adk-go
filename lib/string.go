package lib
func Strlen(s string) int { return len(s) }
func Strcmp(a, b string) int {
	if a < b { return -1 }
	if a > b { return 1 }
	return 0
}
func Strcpy(dst []byte, src string) {
	copy(dst, []byte(src))
}
func Strncpy(dst []byte, src string, n int) {
	if n > len(src) { n = len(src) }
	copy(dst[:n], []byte(src[:n]))
}
func Memset(buf []byte, val byte, n int) {
	for i := 0; i < n && i < len(buf); i++ { buf[i] = val }
}
func Memcpy(dst, src []byte, n int) {
	copy(dst[:n], src[:n])
}
func Memcmp(a, b []byte, n int) int {
	for i := 0; i < n; i++ {
		if a[i] < b[i] { return -1 }
		if a[i] > b[i] { return 1 }
	}
	return 0
}
