package collector

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
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
	Version          string            `json:"version"`
	Python           string            `json:"python"`
	Project          string            `json:"project"`
	Model            string            `json:"model"`
	Provider         string            `json:"provider"`
	GatewayStatus    string            `json:"gateway_status"`
	GatewayPID       int               `json:"gateway_pid,omitempty"`
	Gateway          GatewayInfo       `json:"gateway"`
	Uptime           UptimeInfo        `json:"uptime"`
	Skills           []HermesSkill     `json:"skills"`
	Messaging        map[string]string `json:"messaging"`
}

type HermesSkill struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Source   string `json:"source"`
	Trust    string `json:"trust"`
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

	// Uptime for Hermes gateway process
	if gw.PID > 0 {
		info.Uptime = getProcessUptime(gw.PID)
	}

	// Skills
	info.Skills = getHermesSkills()

	// Messaging platforms
	info.Messaging = getHermesMessaging(result.Output)

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

func getProcessUptime(pid int) UptimeInfo {
	info := UptimeInfo{PID: pid}

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		info.Error = err.Error()
		return info
	}

	// Parse starttime (field 20, index 19)
	rightParen := strings.LastIndex(string(data), ")")
	if rightParen == -1 {
		info.Error = "cannot find process name"
		return info
	}
	afterParen := strings.TrimLeft(string(data)[rightParen+2:], " ")
	fields := strings.Fields(afterParen)
	if len(fields) < 20 {
		info.Error = "invalid stat format"
		return info
	}

	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		info.Error = err.Error()
		return info
	}

	// Read btime
	btimeData, err := os.ReadFile("/proc/stat")
	if err != nil {
		info.Error = err.Error()
		return info
	}
	re := regexp.MustCompile(`btime\s+(\d+)`)
	matches := re.FindStringSubmatch(string(btimeData))
	if len(matches) < 2 {
		info.Error = "btime not found"
		return info
	}
	btime, _ := strconv.ParseInt(matches[1], 10, 64)

	clkTck := float64(getClkTck())
	uptimeSecs := float64(btime) + float64(startTicks)/clkTck - float64(time.Now().Unix())
	if uptimeSecs < 0 {
		uptimeSecs = float64(startTicks) / clkTck
	}

	info.UptimeSeconds = int64(uptimeSecs)
	info.UptimeStr = formatUptime(info.UptimeSeconds)
	return info
}

func getClkTck() int64 {
	return 100
}

func formatUptime(seconds int64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%d天 %d小时", days, hours)
	} else if hours > 0 {
		return fmt.Sprintf("%d小时 %d分钟", hours, mins)
	}
	return fmt.Sprintf("%d分钟", mins)
}

func getHermesSkills() []HermesSkill {
	var skills []HermesSkill
	result := runCommand("hermes", "skills", "list")
	if result.Error != nil || result.Output == "" {
		return skills
	}

	lines := strings.Split(result.Output, "\n")
	for _, line := range lines {
		// Skip header and separator lines
		// Header uses ┃ (0x2503), data uses │ (0x2502), separator uses ━ (0x2500)
		if strings.HasPrefix(line, "┃") || strings.HasPrefix(line, "┏") ||
		   strings.HasPrefix(line, "┡") ||
		   strings.Contains(line, "━━━") ||
		   strings.TrimSpace(line) == "" {
			continue
		}
		// Skip lines that don't start with │ (data lines start with │)
		if !strings.HasPrefix(line, "│") {
			continue
		}
		// Parse: │ name │ category │ source │ trust │
		parts := strings.Split(line, "│")
		if len(parts) >= 5 {
			name := strings.TrimSpace(parts[1])
			category := strings.TrimSpace(parts[2])
			source := strings.TrimSpace(parts[3])
			trust := strings.TrimSpace(parts[4])
			if name != "" && name != "Name" {
				skills = append(skills, HermesSkill{
					Name:     name,
					Category: category,
					Source:   source,
					Trust:    trust,
				})
			}
		}
	}
	return skills
}

func getHermesMessaging(statusOutput string) map[string]string {
	messaging := make(map[string]string)
	lines := strings.Split(statusOutput, "\n")
	inMessagingSection := false
	for _, line := range lines {
		if strings.Contains(line, "Messaging Platforms") {
			inMessagingSection = true
			continue
		}
		if inMessagingSection {
			// End of section
			if strings.HasPrefix(line, "◆") || strings.HasPrefix(line, "─") {
				break
			}
			// Parse: "  Telegram      ✓ configured"
			if strings.Contains(line, "✓") || strings.Contains(line, "✗") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					platform := parts[0]
					status := parts[2]
					messaging[platform] = status
				}
			}
		}
	}
	return messaging
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
