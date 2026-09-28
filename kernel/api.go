package kernel
import "fmt"
func Init() {
	sched_init()
}
func Fork(prompt string, pwd string) (int64, error) {
	pid, err := copy_process(prompt)
	if err != nil {
		return 0, err
	}
	mu.Lock()
	for i := 1; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			if pwd != "" {
				task[i].pwd = pwd
			}
			break
		}
	}
	mu.Unlock()
	return pid, nil
}
func Exit(pid int64, code int) {
	mu.Lock()
	defer mu.Unlock()
	saved := current
	for i := 1; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			current = task[i]
			do_exit(code)
			current = saved
			return
		}
	}
	current = saved
}
func Wait(pid int64) (int64, int, error) {
	return sys_waitpid(pid, 0)
}
func WaitNoHang(pid int64) (int64, int, error) {
	return sys_waitpid(pid, WNOHANG)
}
func Kill(pid int64, sig int64) error {
	ret := sys_kill(pid, sig)
	if ret != 0 {
		return fmt.Errorf("kill: error %d", ret)
	}
	return nil
}
func Ps() {
	show_stat()
}
func GetTask(pid int64) *task_struct {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			return task[i]
		}
	}
	return nil
}
func Current() *task_struct {
	mu.Lock()
	defer mu.Unlock()
	return current
}
func SetCurrent(p *task_struct) {
	mu.Lock()
	defer mu.Unlock()
	current = p
}
func GetPwd(pid int64) string {
	t := GetTask(pid)
	if t == nil {
		return ""
	}
	return t.pwd
}
func SetConvID(pid int64, convID string) {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			task[i].conv_id = convID
			return
		}
	}
}
func SetResult(pid int64, result string) {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			task[i].result = result
			return
		}
	}
}
func GetResult(pid int64) string {
	t := GetTask(pid)
	if t == nil {
		return ""
	}
	return t.result
}
func Pipe() (*pipe_struct, error) {
	return sys_pipe()
}
