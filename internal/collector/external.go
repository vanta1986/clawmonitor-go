package collector

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ExternalMetrics Claude Code & Hermes 状态
type ExternalMetrics struct {
	Timestamp string           `json:"timestamp"`
	Hermes   HermesInfo       `json:"hermes"`
	Claude   ClaudeCodeInfo   `json:"claude_code"`
}

type HermesInfo struct {
	Version    string       `json:"version"`
	Python     string       `json:"python"`
	Project    string       `json:"project"`
	Model      string       `json:"model"`
	Provider   string       `json:"provider"`
	Gateway    GatewayInfo `json:"gateway"`
	Uptime     UptimeInfo   `json:"uptime"`
}

type GatewayInfo struct {
	Status string `json:"status"`
	PID    int    `json:"pid,omitempty"`
}

type UptimeInfo struct {
	PID           int    `json:"pid,omitempty"`
	CPU           float64 `json:"cpu,omitempty"`
	Memory        float64 `json:"memory,omitempty"`
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`
	UptimeStr     string  `json:"uptime_str,omitempty"`
	Error         string  `json:"uptime_error,omitempty"`
}

type ClaudeCodeInfo struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// CollectExternal 采集 Claude Code & Hermes 状态
func CollectExternal() (*ExternalMetrics, error) {
	metrics := &ExternalMetrics{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	metrics.Hermes = getHermesInfo()
	metrics.Claude = getClaudeCodeInfo()

	return metrics, nil
}

func getHermesInfo() HermesInfo {
	info := HermesInfo{}

	// 检查 hermes 命令
	result := runCommand("hermes", "--version")
	if result.Output != "" {
		info.Version = strings.TrimSpace(result.Output)
	}

	result = runCommand("hermes", "status", "--json")
	if result.Output != "" {
		var status struct {
			Version string `json:"version"`
			Python  string `json:"python"`
			Project string `json:"project"`
			Model   string `json:"model"`
			Provider string `json:"provider"`
		}
		if json.Unmarshal([]byte(result.Output), &status) == nil {
			info.Version = status.Version
			info.Python = status.Python
			info.Project = status.Project
			info.Model = status.Model
			info.Provider = status.Provider
		}
	}

	// 检查 Hermes Gateway
	info.Gateway = getHermesGateway()

	return info
}

func getHermesGateway() GatewayInfo {
	gateway := GatewayInfo{Status: "stopped"}

	// 使用 pgrep 查找 hermes gateway 进程
	result := runCommand("pgrep", "-f", "hermes gateway run")
	if result.Output == "" || result.Error != nil {
		return gateway
	}

	pidStr := strings.TrimSpace(result.Output)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return gateway
	}

	gateway.PID = pid
	gateway.Status = "running"
	gateway.PID = pid

	return gateway
}

func getClaudeCodeInfo() ClaudeCodeInfo {
	info := ClaudeCodeInfo{Status: "unknown"}

	// 检查 claude 命令
	result := runCommand("claude", "--version")
	if result.Output != "" {
		info.Version = strings.TrimSpace(result.Output)
	}

	// 检查 Claude Code 是否在运行
	result = runCommand("pgrep", "-f", "claude code")
	if result.Output != "" && result.Error == nil {
		info.Status = "running"
	} else {
		info.Status = "stopped"
	}

	return info
}

type CommandResult struct {
	Output string
	Error  error
}

func runCommand(name string, args ...string) CommandResult {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return CommandResult{Output: "", Error: err}
	}
	return CommandResult{Output: string(output), Error: nil}
}
