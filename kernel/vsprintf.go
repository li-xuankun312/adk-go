package kernel
import "fmt"
func vsprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
func sprintf(format string, args ...interface{}) string {
	return vsprintf(format, args...)
}
