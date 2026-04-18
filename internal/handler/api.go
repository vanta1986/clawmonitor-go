package handler

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"clawmonitor/internal/cache"
	"clawmonitor/internal/collector"
	"clawmonitor/internal/storage"
)

//go:embed web/index.html
var webUI embed.FS

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
	} else {
		log.Printf("storage init failed: %v (history queries will return error)", err)
	}

	return s
}

// Run 启动服务器
func (s *Server) Run() error {
	mux := http.NewServeMux()

	// 静态文件
	staticFS, _ := fs.Sub(webUI, "web")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

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
		"version": "v2.6",
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

	// 并行采集所有数据源，5秒超时，graceful degradation
	type result struct {
		name     string
		system   *collector.SystemMetrics
		openclaw *collector.OpenClawMetrics
		external *collector.ExternalMetrics
		minimax  *collector.MiniMaxMetrics
		err      error
	}

	resultCh := make(chan result, 4)
	done := make(chan struct{})

	// 启动4个goroutine并行采集
	go func() {
		m, err := collector.CollectSystem()
		resultCh <- result{name: "system", system: m, err: err}
	}()
	go func() {
		m, err := collector.CollectOpenClaw()
		resultCh <- result{name: "openclaw", openclaw: m, err: err}
	}()
	go func() {
		m, err := collector.CollectExternal()
		resultCh <- result{name: "external", external: m, err: err}
	}()
	go func() {
		m, err := collector.CollectMiniMax()
		resultCh <- result{name: "minimax", minimax: m, err: err}
	}()

	var results []result
	collecting := true
	for collecting {
		select {
		case res := <-resultCh:
			results = append(results, res)
			if len(results) == 4 {
				close(done)
				collecting = false
			}
		case <-done:
			collecting = false
		case <-time.After(5 * time.Second):
			// 超时：所有未返回的结果视为失败
			close(done)
			collecting = false
		}
	}

	// 如果在超时前只收到部分结果，将未完成的视为失败
	seen := make(map[string]bool)
	for _, res := range results {
		seen[res.name] = true
	}
	for _, name := range []string{"system", "openclaw", "external", "minimax"} {
		if !seen[name] {
			results = append(results, result{name: name, err: fmt.Errorf("timeout")})
		}
	}

	// 汇总结果，空结构保证响应仍含key
	system := &collector.SystemMetrics{}
	openclaw := &collector.OpenClawMetrics{}
	external := &collector.ExternalMetrics{}
	minimax := &collector.MiniMaxMetrics{}
	failCount := 0

	for _, res := range results {
		switch res.name {
		case "system":
			if res.err == nil {
				system = res.system
			} else {
				failCount++
			}
		case "openclaw":
			if res.err == nil {
				openclaw = res.openclaw
			} else {
				failCount++
			}
		case "external":
			if res.err == nil {
				external = res.external
			} else {
				external = &collector.ExternalMetrics{}
			}
		case "minimax":
			if res.err == nil {
				minimax = res.minimax
			} else {
				minimax = &collector.MiniMaxMetrics{}
			}
		}
	}

	// 只有全部失败才返回500
	if failCount == 4 {
		http.Error(w, "internal server error", 500)
		return
	}

	data := map[string]interface{}{
		"system":   system,
		"openclaw": openclaw,
		"external": external,
		"minimax":  minimax,
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
		http.Error(w, "internal server error", 500)
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
		http.Error(w, "internal server error", 500)
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
		http.Error(w, "internal server error", 500)
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
		http.Error(w, "internal server error", 500)
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
		http.Error(w, "internal server error", 500)
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
