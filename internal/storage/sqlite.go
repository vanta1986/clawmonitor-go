package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const DB_PATH = "~/.clawmonitor/monitor.db"

// Storage SQLite 存储
type Storage struct {
	db *sql.DB
}

// New 创建新的存储实例
func New() (*Storage, error) {
	dbPath := os.ExpandEnv(DB_PATH)
	
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Storage{db: db}
	if err := s.initTables(); err != nil {
		return nil, err
	}

	return s, nil
}

// Close 关闭数据库连接
func (s *Storage) Close() error {
	return s.db.Close()
}

// initTables 初始化表结构
func (s *Storage) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS system_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			cpu_percent REAL,
			cpu_per_core TEXT,
			memory_percent REAL,
			memory_total INTEGER,
			memory_used INTEGER,
			disk_usage TEXT,
			network_bytes_sent INTEGER,
			network_bytes_recv INTEGER,
			process_count INTEGER,
			uptime_seconds INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS openclaw_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			gateway_running INTEGER,
			agents_count INTEGER,
			plugins_count INTEGER,
			sessions_count INTEGER,
			models_count INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS minimax_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			total_calls INTEGER,
			input_tokens INTEGER,
			output_tokens INTEGER,
			estimated_cost REAL,
			today_calls INTEGER,
			today_input_tokens INTEGER,
			today_output_tokens INTEGER,
			by_model TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_ts ON system_metrics(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_openclaw_ts ON openclaw_metrics(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_minimax_ts ON minimax_metrics(timestamp)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// SaveSystemMetrics 保存系统资源数据
func (s *Storage) SaveSystemMetrics(data *SystemMetricsData) error {
	query := `INSERT INTO system_metrics 
		(timestamp, cpu_percent, cpu_per_core, memory_percent, memory_total, memory_used, 
		disk_usage, network_bytes_sent, network_bytes_recv, process_count, uptime_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	cpuPerCoreJSON, err := json.Marshal(data.CPUPerCore)
	if err != nil {
		return fmt.Errorf("marshal cpu per core: %w", err)
	}
	diskUsageJSON, err := json.Marshal(data.DiskUsage)
	if err != nil {
		return fmt.Errorf("marshal disk usage: %w", err)
	}

	_, err = s.db.Exec(query,
		data.Timestamp,
		data.CPUPercent,
		string(cpuPerCoreJSON),
		data.MemoryPercent,
		data.MemoryTotal,
		data.MemoryUsed,
		string(diskUsageJSON),
		data.Network.BytesSent,
		data.Network.BytesRecv,
		data.ProcessCount,
		data.UptimeSeconds,
	)

	return err
}

// SaveOpenClawMetrics 保存 OpenClaw 状态数据
func (s *Storage) SaveOpenClawMetrics(data *OpenClawMetricsData) error {
	query := `INSERT INTO openclaw_metrics 
		(timestamp, gateway_running, agents_count, plugins_count, sessions_count, models_count)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query,
		data.Timestamp,
		boolToInt(data.GatewayRunning),
		data.AgentsCount,
		data.PluginsCount,
		data.SessionsCount,
		data.ModelsCount,
	)

	return err
}

// SaveMiniMaxMetrics 保存 MiniMax 使用数据
func (s *Storage) SaveMiniMaxMetrics(data *MiniMaxMetricsData) error {
	byModelJSON, _ := json.Marshal(data.ByModel)

	query := `INSERT INTO minimax_metrics 
		(timestamp, total_calls, input_tokens, output_tokens, estimated_cost,
		today_calls, today_input_tokens, today_output_tokens, by_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query,
		data.Timestamp,
		data.TotalCalls,
		data.InputTokens,
		data.OutputTokens,
		data.EstimatedCost,
		data.TodayCalls,
		data.TodayInputTokens,
		data.TodayOutputTokens,
		string(byModelJSON),
	)

	return err
}

// GetSystemHistory 获取系统资源历史
func (s *Storage) GetSystemHistory(hours, limit int) ([]map[string]interface{}, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	query := `SELECT timestamp, cpu_percent, memory_percent, memory_used, process_count, uptime_seconds
		FROM system_metrics WHERE timestamp >= ? ORDER BY timestamp DESC LIMIT ?`

	rows, err := s.db.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var timestamp string
		var cpuPercent, memoryPercent float64
		var memoryUsed, processCount, uptimeSeconds int

		if err := rows.Scan(&timestamp, &cpuPercent, &memoryPercent, &memoryUsed, &processCount, &uptimeSeconds); err != nil {
			log.Printf("scan row error: %v", err)
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":      timestamp,
			"cpu_percent":   cpuPercent,
			"memory_percent": memoryPercent,
			"memory_used":   memoryUsed,
			"process_count": processCount,
			"uptime_seconds": uptimeSeconds,
		})
	}

	return results, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// 数据结构定义
type SystemMetricsData struct {
	Timestamp      string
	CPUPercent     float64
	CPUPerCore    []float64
	MemoryPercent float64
	MemoryTotal   uint64
	MemoryUsed    uint64
	DiskUsage     []DiskPartData
	Network       NetworkInfo
	ProcessCount  int
	UptimeSeconds int64
}

type NetworkInfo struct {
	BytesSent uint64
	BytesRecv uint64
}

type DiskPartData struct {
	Device  string
	Mount   string
	Total   uint64
	Used    uint64
	Percent float64
}

type OpenClawMetricsData struct {
	Timestamp      string
	GatewayRunning bool
	AgentsCount    int
	PluginsCount   int
	SessionsCount  int
	ModelsCount    int
}

type MiniMaxMetricsData struct {
	Timestamp        string
	TotalCalls       int64
	InputTokens      int64
	OutputTokens     int64
	EstimatedCost    float64
	TodayCalls       int
	TodayInputTokens  int64
	TodayOutputTokens int64
	ByModel          map[string]ModelStatsData
}

type ModelStatsData struct {
	Calls        int
	InputTokens  int64
	OutputTokens int64
	EstimatedCost float64
}
