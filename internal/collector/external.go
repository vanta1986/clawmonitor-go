package collector

import (
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
	Version        string      `json:"version"`
	Python         string      `json:"python"`
	Project        string      `json:"project"`
	Model          string      `json:"model"`
	Provider       string      `json:"provider"`
	GatewayStatus  string      `json:"gateway_status"`
	GatewayPID     int         `json:"gateway_pid,omitempty"`
	Gateway        GatewayInfo `json:"gateway"`
	Uptime         UptimeInfo  `json:"uptime"`
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

	// Version
	result := runCommand("hermes", "--version")
	if result.Output != "" {
		lines := strings.Split(result.Output, "\n")
		info.Version = strings.TrimSpace(lines[0])
	}

	// Parse plain text status output
	result = runCommand("hermes", "status")
	if result.Output != "" {
		for _, line := range strings.Split(result.Output, "\n") {
			if strings.HasPrefix(line, "  Project:") {
				info.Project = strings.TrimSpace(strings.TrimPrefix(line, "  Project:"))
			} else if strings.HasPrefix(line, "  Python:") {
				info.Python = strings.TrimSpace(strings.TrimPrefix(line, "  Python:"))
			} else if strings.HasPrefix(line, "  Model:") {
				info.Model = strings.TrimSpace(strings.TrimPrefix(line, "  Model:"))
			} else if strings.HasPrefix(line, "  Provider:") {
				info.Provider = strings.TrimSpace(strings.TrimPrefix(line, "  Provider:"))
			}
		}
	}

	// Gateway
	gw := getHermesGateway()
	info.Gateway = gw
	info.GatewayStatus = gw.Status
	info.GatewayPID = gw.PID

	return info
}

func getHermesGateway() GatewayInfo {
	gateway := GatewayInfo{Status: "stopped"}

	// Check via systemctl
	result := runCommand("systemctl", "--user", "is-active", "hermes-gateway")
	if strings.TrimSpace(result.Output) == "active" {
		gateway.Status = "running"
	}

	// Get PID from systemctl
	result = runCommand("systemctl", "--user", "show", "hermes-gateway", "--property=MainPID", "--value")
	if result.Error == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(result.Output)); err == nil && pid > 0 {
			gateway.PID = pid
		}
	}

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
