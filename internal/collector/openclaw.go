package collector

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// OpenClawMetrics OpenClaw 状态数据
type OpenClawMetrics struct {
	Timestamp  string            `json:"timestamp"`
	Gateway    GatewayStatus     `json:"gateway"`
	Runtime    RuntimeInfo       `json:"runtime"`
	Memory     MemoryUsage       `json:"memory"`
	Config     ConfigInfo        `json:"config"`
	Skills     SkillsInfo        `json:"skills"`
	Extensions ExtensionsInfo     `json:"extensions"`
	Sessions   SessionsInfo      `json:"sessions"`
}

type GatewayStatus struct {
	Running       bool    `json:"running"`
	Port         int     `json:"port"`
	PID          int     `json:"pid,omitempty"`
	CPU          float64 `json:"cpu,omitempty"`
	Memory       float64 `json:"memory,omitempty"`
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`
	UptimeStr    string  `json:"uptime_str,omitempty"`
}

type RuntimeInfo struct {
	OS       string `json:"os"`
	Python   string `json:"python"`
	Node     string `json:"node"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
}

type MemoryUsage struct {
	RSSMB float64 `json:"rss_mb"`
}

type ConfigInfo struct {
	Loaded       bool     `json:"loaded"`
	Version     string   `json:"version"`
	AgentsCount int      `json:"agents_count"`
	ModelsCount int      `json:"models_count"`
	Providers   []string `json:"providers"`
	Models      []ModelInfo `json:"models"`
	Agents      []AgentInfo `json:"agents"`
}

type ModelInfo struct {
	Name          string `json:"name"`
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	Reasoning     bool   `json:"reasoning"`
	ContextWindow int    `json:"context_window"`
}

type AgentInfo struct {
	Name   string   `json:"name"`
	Model  string   `json:"model"`
}

type SkillsInfo struct {
	Count         int        `json:"count"`
	Skills        []SkillInfo `json:"skills"`
}

type SkillInfo struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
}

type ExtensionsInfo struct {
	Count     int       `json:"count"`
	Extensions []ExtensionInfo `json:"extensions"`
}

type ExtensionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SessionsInfo struct {
	Count int `json:"count"`
}

const GATEWAY_PORT = 18789
const OPENCLAW_CONFIG_PATH = "$HOME/.openclaw/openclaw.json"
const OPENCLAW_WORKSPACE = "$HOME/.openclaw/workspace"

// CollectOpenClaw 采集 OpenClaw 状态
func CollectOpenClaw() (*OpenClawMetrics, error) {
	metrics := &OpenClawMetrics{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Gateway 状态
	metrics.Gateway = getGatewayStatus()

	// 运行时信息
	metrics.Runtime = getRuntimeInfo()

	// 内存使用
	metrics.Memory = getMemoryUsage()

	// 配置信息
	metrics.Config = getConfig()

	// Skills
	metrics.Skills = getSkills()

	// Extensions
	metrics.Extensions = getExtensions()

	// Sessions
	metrics.Sessions = getSessions()

	return metrics, nil
}

func getGatewayStatus() GatewayStatus {
	status := GatewayStatus{
		Port: GATEWAY_PORT,
	}

	// 端口检测
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", GATEWAY_PORT), 2*time.Second)
	if err == nil {
		conn.Close()
		status.Running = true
	} else {
		return status
	}

	// 进程信息
	cmd := exec.Command("pgrep", "-f", "openclaw.*gateway")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		pidStr := strings.TrimSpace(string(output))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			status.PID = pid
			uptime := getProcessUptime(pid); status.UptimeSeconds = uptime.UptimeSeconds; status.UptimeStr = uptime.UptimeStr
		}
	}

	return status
}


func getRuntimeInfo() RuntimeInfo {
	info := RuntimeInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Hostname: getHostname(),
	}

	// Node version
	if nodeVersion, err := exec.Command("node", "--version").Output(); err == nil {
		info.Node = strings.TrimSpace(string(nodeVersion))
	}

	return info
}

func getHostname() string {
	hostname, _ := os.Hostname()
	return hostname
}

func getMemoryUsage() MemoryUsage {
	// 读取当前进程内存
	var rss int64
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		re := regexp.MustCompile(`VmRSS:\s+(\d+)\s+kB`)
		matches := re.FindStringSubmatch(string(data))
		if len(matches) >= 2 {
			if kb, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
				rss = kb / 1024 // MB
			}
		}
	}
	return MemoryUsage{RSSMB: float64(rss)}
}

func getConfig() ConfigInfo {
	config := ConfigInfo{Loaded: false}

	// 尝试读取 OpenClaw 配置
	configPath := os.ExpandEnv(OPENCLAW_CONFIG_PATH)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return config
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config
	}

	config.Loaded = true

	// 提取 agents
	if agents, ok := cfg["agents"].(map[string]interface{}); ok {
		// agents.map has keys: defaults, list, etc.
		// list is an array of actual agent definitions
		if agentList, ok := agents["list"].([]interface{}); ok {
			config.AgentsCount = len(agentList)
			for _, item := range agentList {
				if agentMap, ok := item.(map[string]interface{}); ok {
					agentInfo := AgentInfo{}
					if id, ok := agentMap["id"].(string); ok {
						agentInfo.Name = id
					}
					if model, ok := agentMap["model"].(string); ok {
						agentInfo.Model = model
					}
					config.Agents = append(config.Agents, agentInfo)
				}
			}
		}
		// Also handle defaults for any agent-level settings
		if defaults, ok := agents["defaults"].(map[string]interface{}); ok {
			// Could extract default model here if needed
			_ = defaults
		}
	}

	// 提取 models
	if models, ok := cfg["models"].(map[string]interface{}); ok {
		if providers, ok := models["providers"].(map[string]interface{}); ok {
			config.Providers = make([]string, 0, len(providers))
			for providerName, provider := range providers {
				config.Providers = append(config.Providers, providerName)
				if providerMap, ok := provider.(map[string]interface{}); ok {
					if modelList, ok := providerMap["models"].([]interface{}); ok {
						for _, m := range modelList {
							if modelMap, ok := m.(map[string]interface{}); ok {
								modelInfo := ModelInfo{
									Provider: providerName,
								}
								if id, ok := modelMap["id"].(string); ok {
									modelInfo.ID = id
									modelInfo.Name = id
								}
								if reasoning, ok := modelMap["reasoning"].(bool); ok {
									modelInfo.Reasoning = reasoning
								}
								if contextWindow, ok := modelMap["context_window"].(int); ok {
									modelInfo.ContextWindow = contextWindow
								}
								config.Models = append(config.Models, modelInfo)
							}
						}
					}
				}
			}
			config.ModelsCount = len(config.Models)
		}
	}

	return config
}

func getSkills() SkillsInfo {
	info := SkillsInfo{}

	skillDirs := []string{
		os.ExpandEnv("$HOME/.openclaw/workspace/skills"),
		os.ExpandEnv("$HOME/.npm-global/lib/node_modules/openclaw/skills"),
	}

	for _, dir := range skillDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
			desc := ""

			if data, err := os.ReadFile(skillFile); err == nil {
				// 尝试提取 description
				re := regexp.MustCompile(`(?m)^description:\s*(.+)$`)
				matches := re.FindStringSubmatch(string(data))
				if len(matches) >= 2 {
					desc = matches[1]
				}
			}

			info.Skills = append(info.Skills, SkillInfo{
				Name:        entry.Name(),
				Description: desc,
			})
		}
	}

	info.Count = len(info.Skills)
	return info
}

func getExtensions() ExtensionsInfo {
	info := ExtensionsInfo{}

	extDir := os.ExpandEnv("$HOME/.openclaw/extensions")
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return info
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info.Extensions = append(info.Extensions, ExtensionInfo{
			Name: entry.Name(),
		})
	}

	info.Count = len(info.Extensions)
	return info
}

func getSessions() SessionsInfo {
	info := SessionsInfo{}

	sessionsDir := filepath.Join(os.ExpandEnv(OPENCLAW_WORKSPACE), "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return info
	}

	info.Count = len(entries)
	return info
}
