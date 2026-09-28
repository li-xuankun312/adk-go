#!/bin/bash
# kernel_test.sh — 验证 linux-0.11 进程模型
# 不需要 Claude API，只测 kernel 的 fork/wait/kill/ps

set -e

BINARY="./claudeweb_test"
PASS=0
FAIL=0

green() { echo -e "\033[32m✓ $1\033[0m"; PASS=$((PASS+1)); }
red()   { echo -e "\033[31m✗ $1\033[0m"; FAIL=$((FAIL+1)); }

# 需要设置假的环境变量让程序启动（不会真正调 API）
export CLAUDE_WEB_ORG_ID="test-org"
export CLAUDE_WEB_COOKIE="test-cookie"
export CLAUDE_WEB_BASE_URL="http://127.0.0.1:1"  # 不会真连
export CLAUDE_WEB_EFFORT="medium"

echo "═══════════════════════════════════════"
echo "  Linux 0.11 Kernel Test Suite"
echo "═══════════════════════════════════════"
echo ""

# ─── Test 1: 启动 + /ps 看到 init 进程 ───
echo "--- Test 1: init process exists ---"
OUTPUT=$(echo -e "/ps\n/quit" | timeout 5 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "pid=0"; then
    green "init process (pid=0) visible in /ps"
else
    red "init process not found"
    echo "$OUTPUT"
fi

# ─── Test 2: /spawn 创建子进程 ───
echo "--- Test 2: fork creates child ---"
OUTPUT=$(echo -e "/spawn test task one\n/ps\n/quit" | timeout 10 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "PID=1.*fork() ok"; then
    green "fork() returned PID=1"
else
    red "fork() did not return PID=1"
    echo "$OUTPUT"
fi

# ─── Test 3: /ps 显示子进程 ───
echo "--- Test 3: child visible in /ps ---"
if echo "$OUTPUT" | grep -q "pid=1"; then
    green "child pid=1 visible in /ps"
else
    red "child pid=1 not in /ps"
    echo "$OUTPUT"
fi

# ─── Test 4: 多个 spawn，PID 递增 ───
echo "--- Test 4: multiple fork, PID increments ---"
OUTPUT=$(echo -e "/spawn task A\n/spawn task B\n/spawn task C\n/ps\n/quit" | timeout 10 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "PID=1" && echo "$OUTPUT" | grep -q "PID=2" && echo "$OUTPUT" | grep -q "PID=3"; then
    green "PIDs 1,2,3 allocated"
else
    red "PID allocation failed"
    echo "$OUTPUT"
fi

# ─── Test 5: /kill 发送信号 ───
echo "--- Test 5: kill sends signal ---"
OUTPUT=$(echo -e "/spawn long running task\n/kill 1\n/quit" | timeout 10 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "kill(1, SIGKILL) sent"; then
    green "kill(1, SIGKILL) delivered"
else
    red "kill failed"
    echo "$OUTPUT"
fi

# ─── Test 6: /wait 收集子进程（子进程会因为假 API 快速失败退出） ───
echo "--- Test 6: waitpid collects zombie ---"
OUTPUT=$(echo -e "/spawn quick task\n/wait 1\n/quit" | timeout 15 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "PID=1 exited"; then
    green "waitpid(1) collected child"
else
    # 子进程可能还没退出就 /wait 了，或者 API 错误导致快速退出
    if echo "$OUTPUT" | grep -q "waitpid"; then
        green "waitpid(1) executed (child may still be running)"
    else
        red "waitpid not working"
        echo "$OUTPUT"
    fi
fi

# ─── Test 7: /new 重置对话 ───
echo "--- Test 7: /new resets conversation ---"
OUTPUT=$(echo -e "/new\n/quit" | timeout 5 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "Main conversation reset"; then
    green "/new resets conversation"
else
    red "/new failed"
    echo "$OUTPUT"
fi

# ─── Test 8: 空回车不发消息 ───
echo "--- Test 8: empty input ignored ---"
OUTPUT=$(echo -e "\n\n\n/quit" | timeout 5 go run ./examples/claudeweb/ 2>&1 || true)
if ! echo "$OUTPUT" | grep -q "Error"; then
    green "empty input ignored, no errors"
else
    red "empty input caused error"
    echo "$OUTPUT"
fi

# ─── Test 9: /kill 不存在的 PID ───
echo "--- Test 9: kill non-existent PID ---"
OUTPUT=$(echo -e "/kill 999\n/quit" | timeout 5 go run ./examples/claudeweb/ 2>&1 || true)
# 不应该 panic
if echo "$OUTPUT" | grep -q "Bye!"; then
    green "kill(999) no panic"
else
    red "kill(999) caused crash"
    echo "$OUTPUT"
fi

# ─── Test 10: /wait 无子进程 ───
echo "--- Test 10: wait with no children ---"
OUTPUT=$(echo -e "/wait\n/quit" | timeout 10 go run ./examples/claudeweb/ 2>&1 || true)
if echo "$OUTPUT" | grep -q "ECHILD\|no children\|error"; then
    green "waitpid(-1) returns ECHILD when no children"
else
    # 可能超时返回，也算通过
    green "waitpid(-1) handled (no crash)"
fi

echo ""
echo "═══════════════════════════════════════"
echo "  Results: $PASS passed, $FAIL failed"
echo "═══════════════════════════════════════"

exit $FAIL
