// Command worker checks the Worker bootstrap entry point; it does not judge code.
package main

import (
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Worker 骨架入口可执行；任务协议和沙箱未接入，未启动判题服务", "mode", "bootstrap-check")
}
