package collector

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// SystemMetrics 系统资源数据
type SystemMetrics struct {
	Timestamp     string  `json:"timestamp"`
	CPU           CPUInfo `json:"cpu"`
	Memory        MemoryInfo `json:"memory"`
	Disk          []DiskPart `json:"disk"`
	Network       NetworkInfo `json:"network"`
	ProcessCount  int `json:"process_count"`
	UptimeSeconds int64 `json:"uptime_seconds"`
}

type CPUInfo struct {
	UsagePercent float64   `json:"usage_percent"`
	PerCPU      []float64 `json:"per_cpu"`
}

type MemoryInfo struct {
	Percent float64 `json:"percent"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
}

type DiskPart struct {
	Device  string `json:"device"`
	Mount   string `json:"mount"`
	FSType  string `json:"fs_type"`
	Total   uint64 `json:"total"`
	Used    uint64 `json:"used"`
	Free    uint64 `json:"free"`
	Percent float64 `json:"percent"`
}

type NetworkInfo struct {
	BytesSent uint64 `json:"bytes_sent"`
	BytesRecv uint64 `json:"bytes_recv"`
}

// CollectSystem 采集系统资源
func CollectSystem() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// CPU
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		metrics.CPU.UsagePercent = cpuPercent[0]
	}

	cpuPerCPU, err := cpu.Percent(time.Second, true)
	if err == nil {
		metrics.CPU.PerCPU = cpuPerCPU
	}

	// Memory
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.Memory = MemoryInfo{
			Percent: memInfo.UsedPercent,
			Total:   memInfo.Total,
			Used:    memInfo.Used,
			Free:    memInfo.Free,
		}
	}

	// Disk
	parts, err := disk.Partitions(false)
	if err == nil {
		for _, part := range parts {
			usage, err := disk.Usage(part.Mountpoint)
			if err == nil {
				metrics.Disk = append(metrics.Disk, DiskPart{
					Device:  part.Device,
					Mount:   part.Mountpoint,
					FSType:  part.Fstype,
					Total:   usage.Total,
					Used:    usage.Used,
					Free:    usage.Free,
					Percent: usage.UsedPercent,
				})
			}
		}
	}

	// Network
	ioCounters, err := net.IOCounters(true)
	if err == nil && len(ioCounters) > 0 {
		var totalSent, totalRecv uint64
		for _, c := range ioCounters {
			totalSent += c.BytesSent
			totalRecv += c.BytesRecv
		}
		metrics.Network = NetworkInfo{
			BytesSent: totalSent,
			BytesRecv: totalRecv,
		}
	}

	// Process count - 用 ps 获取
	if output, err := exec.Command("sh", "-c", "ps aux | wc -l").Output(); err == nil {
		if count, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil {
			metrics.ProcessCount = count - 1 // 减去标题行
		}
	}

	// Uptime
	if up, err := host.Uptime(); err == nil {
		metrics.UptimeSeconds = int64(up)
	}

	return metrics, nil
}

// FormatBytes 格式化字节数
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatUint(bytes, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 2, 64) + " " + string("KMGTPE"[exp]) + "B"
}
