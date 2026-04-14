package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"clawmonitor/internal/cache"
	"clawmonitor/internal/collector"
	"clawmonitor/internal/storage"
)

// Server API 服务
type Server struct {
	port    int
	cache   *cache.MemoryCache
	storage *storage.Storage
}

// NewServer 创建 API 服务器
func NewServer(port int) *Server {
	s := &Server{
		port:  port,
		cache: cache.New(time.Minute * 5),
	}

	// 尝试初始化存储
	if db, err := storage.New(); err == nil {
		s.storage = db
	}

	return s
}

// Run 启动服务器
func (s *Server) Run() error {
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/system", s.handleSystem)
	mux.HandleFunc("/api/openclaw", s.handleOpenClaw)
	mux.HandleFunc("/api/minimax", s.handleMiniMax)
	mux.HandleFunc("/api/external", s.handleExternal)
	mux.HandleFunc("/api/history", s.handleHistory)

	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	fmt.Printf("ClawMonitor API Server starting on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"name":    "ClawMonitor",
		"version": "v2.5",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// 尝试从缓存获取
	if data, ok := s.cache.Get("status"); ok {
		w.Header().Set("X-Cache", "hit")
		json.NewEncoder(w).Encode(data)
		return
	}

	w.Header().Set("X-Cache", "miss")

	// 采集数据
	system, err := collector.CollectSystem()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	openclaw, err := collector.CollectOpenClaw()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	data := map[string]interface{}{
		"system":   system,
		"openclaw": openclaw,
	}

	// 存入缓存
	s.cache.Set("status", data)

	// 保存到数据库
	if s.storage != nil {
		s.saveSystemMetrics(system)
		s.saveOpenClawMetrics(openclaw)
	}

	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	if data, ok := s.cache.Get("system"); ok {
		json.NewEncoder(w).Encode(data)
		return
	}

	system, err := collector.CollectSystem()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.cache.Set("system", system)
	json.NewEncoder(w).Encode(system)
}

func (s *Server) handleOpenClaw(w http.ResponseWriter, r *http.Request) {
	if data, ok := s.cache.Get("openclaw"); ok {
		json.NewEncoder(w).Encode(data)
		return
	}

	openclaw, err := collector.CollectOpenClaw()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.cache.Set("openclaw", openclaw)
	json.NewEncoder(w).Encode(openclaw)
}

func (s *Server) handleMiniMax(w http.ResponseWriter, r *http.Request) {
	if data, ok := s.cache.Get("minimax"); ok {
		json.NewEncoder(w).Encode(data)
		return
	}

	minimax, err := collector.CollectMiniMax()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.cache.Set("minimax", minimax)

	// 保存到数据库
	if s.storage != nil {
		s.saveMiniMaxMetrics(minimax)
	}

	json.NewEncoder(w).Encode(minimax)
}

func (s *Server) handleExternal(w http.ResponseWriter, r *http.Request) {
	if data, ok := s.cache.Get("external"); ok {
		json.NewEncoder(w).Encode(data)
		return
	}

	external, err := collector.CollectExternal()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.cache.Set("external", external)
	json.NewEncoder(w).Encode(external)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		http.Error(w, "storage not available", 500)
		return
	}

	hours := 24
	limit := 100
	if h := r.URL.Query().Get("hours"); h != "" {
		fmt.Sscanf(h, "%d", &hours)
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	history, err := s.storage.GetSystemHistory(hours, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"columns": []string{"timestamp", "cpu_percent", "memory_percent", "memory_used", "process_count", "uptime_seconds"},
		"rows":    history,
	})
}

// 数据库保存函数
func (s *Server) saveSystemMetrics(data *collector.SystemMetrics) {
	if s.storage == nil {
		return
	}

	diskData := make([]storage.DiskPartData, len(data.Disk))
	for i, d := range data.Disk {
		diskData[i] = storage.DiskPartData{
			Device:  d.Device,
			Mount:   d.Mount,
			Total:   d.Total,
			Used:    d.Used,
			Percent: d.Percent,
		}
	}

	metrics := &storage.SystemMetricsData{
		Timestamp:      data.Timestamp,
		CPUPercent:     data.CPU.UsagePercent,
		CPUPerCore:    data.CPU.PerCPU,
		MemoryPercent: data.Memory.Percent,
		MemoryTotal:   data.Memory.Total,
		MemoryUsed:    data.Memory.Used,
		DiskUsage:     diskData,
		Network:       storage.NetworkInfo{BytesSent: data.Network.BytesSent, BytesRecv: data.Network.BytesRecv},
		ProcessCount:  data.ProcessCount,
		UptimeSeconds: data.UptimeSeconds,
	}

	s.storage.SaveSystemMetrics(metrics)
}

func (s *Server) saveOpenClawMetrics(data *collector.OpenClawMetrics) {
	if s.storage == nil {
		return
	}

	metrics := &storage.OpenClawMetricsData{
		Timestamp:      data.Timestamp,
		GatewayRunning: data.Gateway.Running,
		AgentsCount:    data.Config.AgentsCount,
		PluginsCount:   data.Extensions.Count,
		SessionsCount:  data.Sessions.Count,
		ModelsCount:    data.Config.ModelsCount,
	}

	s.storage.SaveOpenClawMetrics(metrics)
}

func (s *Server) saveMiniMaxMetrics(data *collector.MiniMaxMetrics) {
	if s.storage == nil {
		return
	}

	byModel := make(map[string]storage.ModelStatsData)
	for name, model := range data.ByModel {
		byModel[name] = storage.ModelStatsData{
			Calls:        model.Calls,
			InputTokens:  model.InputTokens,
			OutputTokens: model.OutputTokens,
			EstimatedCost: model.EstimatedCost,
		}
	}

	metrics := &storage.MiniMaxMetricsData{
		Timestamp:        data.Timestamp,
		TotalCalls:       data.Total.Calls,
		InputTokens:      data.Total.InputTokens,
		OutputTokens:     data.Total.OutputTokens,
		EstimatedCost:   data.Total.EstimatedCost,
		TodayCalls:       data.Today.Calls,
		TodayInputTokens:  data.Today.InputTokens,
		TodayOutputTokens: data.Today.OutputTokens,
		ByModel:         byModel,
	}

	s.storage.SaveMiniMaxMetrics(metrics)
}
