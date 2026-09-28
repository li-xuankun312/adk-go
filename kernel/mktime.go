package kernel
import "time"
func kernel_mktime(year, mon, day, hour, min, sec int) int64 {
	t := time.Date(year, time.Month(mon), day, hour, min, sec, 0, time.UTC)
	return t.Unix()
}
func MkTime(year, mon, day, hour, min, sec int) int64 {
	return kernel_mktime(year, mon, day, hour, min, sec)
}
