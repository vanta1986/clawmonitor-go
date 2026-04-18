package collector

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const MINIMAX_STATS_DIR = "$HOME/.openclaw/stats"

// MiniMaxMetrics MiniMax 使用统计
type MiniMaxMetrics struct {
	Timestamp   string           `json:"timestamp"`
	Source      string           `json:"source"`
	Total       TotalStats       `json:"total"`
	Today       DayStats         `json:"today"`
	ThisMonth   MonthStats       `json:"this_month"`
	ByModel     map[string]ModelStats `json:"by_model"`
	TokenPlan   []TokenPlanItem  `json:"token_plan,omitempty"`
}

type TotalStats struct {
	Calls        int64   `json:"calls"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	EstimatedCost float64 `json:"estimated_cost"`
}

type DayStats struct {
	Calls        int   `json:"calls"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

type MonthStats struct {
	Calls        int   `json:"calls"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

type ModelStats struct {
	Calls        int     `json:"calls"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	EstimatedCost float64 `json:"estimated_cost"`
}

type TokenPlanItem struct {
	ModelName    string `json:"model_name"`
	RemainTime   int64  `json:"remains_time"`
	TotalCount   int64  `json:"current_interval_total_count"`
	UsageCount   int64  `json:"current_interval_usage_count"`
	ResetTime    string `json:"reset_time,omitempty"`
}

// CollectMiniMax 采集 MiniMax 使用统计
func CollectMiniMax() (*MiniMaxMetrics, error) {
	metrics := &MiniMaxMetrics{
		Timestamp: time.Now().Format(time.RFC3339),
		ByModel:   make(map[string]ModelStats),
	}

	// 扫描本地统计文件
	stats := scanStatsFiles()
	if stats.Total.Calls > 0 {
		metrics.Source = "stats_files"
		metrics.Total = stats.Total
		metrics.ByModel = stats.ByModel

		// 计算今日统计
		today := time.Now().Format("2006-01-02")
		if todayStats, ok := stats.DailyStats[today]; ok {
			metrics.Today = DayStats{
				Calls:        todayStats.Calls,
				InputTokens:  todayStats.InputTokens,
				OutputTokens: todayStats.OutputTokens,
			}
		}

		// 计算本月统计
		metrics.ThisMonth = calculateMonthStats(stats.DailyStats)
	} else {
		metrics.Source = "no_data"
	}

	// 获取 Token Plan 数据
	tokenPlan := getTokenPlanUsage()
	if tokenPlan != nil {
		metrics.TokenPlan = tokenPlan
	}

	return metrics, nil
}

type LocalStats struct {
	Total       TotalStats
	DailyStats  map[string]DayStats
	ByModel     map[string]ModelStats
}

func scanStatsFiles() *LocalStats {
	stats := &LocalStats{
		DailyStats: make(map[string]DayStats),
		ByModel:    make(map[string]ModelStats),
	}

	statsDir := os.ExpandEnv(MINIMAX_STATS_DIR)
	if _, err := os.Stat(statsDir); os.IsNotExist(err) {
		return stats
	}

	entries, err := os.ReadDir(statsDir)
	if err != nil {
		return stats
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

			if len(entry.Name()) < 14 {
			continue
		}
		dateStr := entry.Name()[6:14]
		if _, err := time.Parse("20060102", dateStr); err != nil {
			continue // skip invalid date filenames
		}
		dayCalls := 0
		dayInputTokens := int64(0)
		dayOutputTokens := int64(0)

		file, err := os.Open(filepath.Join(statsDir, entry.Name()))
		if err != nil {
			continue
		}
		defer file.Close()

		for {
			line := make([]byte, 4096)
			n, err := file.Read(line)
			if n == 0 || err == io.EOF {
				break
			}
			if err != nil {
				break
			}

			var record struct {
				Model       string `json:"model"`
				InputTokens int64  `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			}
			if json.Unmarshal(line[:n], &record) != nil {
				continue
			}

			stats.Total.Calls++
			stats.Total.InputTokens += record.InputTokens
			stats.Total.OutputTokens += record.OutputTokens
			dayCalls++
			dayInputTokens += record.InputTokens
			dayOutputTokens += record.OutputTokens

			if record.Model == "" {
				record.Model = "unknown"
			}

			modelStats, ok := stats.ByModel[record.Model]
			if !ok {
				modelStats = ModelStats{}
			}
			modelStats.Calls++
			modelStats.InputTokens += record.InputTokens
			modelStats.OutputTokens += record.OutputTokens
			stats.ByModel[record.Model] = modelStats
		}

		if dayCalls > 0 {
			stats.DailyStats[dateStr] = DayStats{
				Calls:        dayCalls,
				InputTokens:  dayInputTokens,
				OutputTokens: dayOutputTokens,
			}
		}
	}

	// 估算成本
	estimateCost(stats)

	return stats
}

func estimateCost(stats *LocalStats) {
	// MiniMax 定价（元/千tokens）
	pricing := map[string]struct{ Input, Output float64 }{
		"MiniMax-M2.7": {0.01, 0.03},
		"MiniMax-M2.5": {0.005, 0.015},
	}
	defaultPricing := struct{ Input, Output float64 }{0.01, 0.03}

	for model, modelStats := range stats.ByModel {
		p := defaultPricing
		if mp, ok := pricing[model]; ok {
			p = mp
		}
		inputCost := float64(modelStats.InputTokens) / 1000 * p.Input
		outputCost := float64(modelStats.OutputTokens) / 1000 * p.Output
		modelStats.EstimatedCost = inputCost + outputCost
		stats.ByModel[model] = modelStats
	}

	// 计算总量
	var totalCost float64
	for _, ms := range stats.ByModel {
		totalCost += ms.EstimatedCost
	}
	stats.Total.EstimatedCost = totalCost
}

func calculateMonthStats(dailyStats map[string]DayStats) MonthStats {
	monthPrefix := time.Now().Format("2006-01")
	stats := MonthStats{}

	for dateStr, dayStats := range dailyStats {
		if len(dateStr) >= 6 && dateStr[:6] == monthPrefix {
			stats.Calls += dayStats.Calls
			stats.InputTokens += dayStats.InputTokens
			stats.OutputTokens += dayStats.OutputTokens
		}
	}

	return stats
}

func getTokenPlanUsage() []TokenPlanItem {
	apiKey := getAPIKey()
	if apiKey == "" {
		return nil
	}

	url := "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		BaseResp struct {
			StatusCode int `json:"status_code"`
		} `json:"base_resp"`
		ModelRemains []TokenPlanItem `json:"model_remains"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.BaseResp.StatusCode != 0 {
		return nil
	}

	// 处理每个条目
	for i := range result.ModelRemains {
		item := &result.ModelRemains[i]
		if item.RemainTime > 0 {
			hours := item.RemainTime / 3600000
			minutes := (item.RemainTime % 3600000) / 60000
			item.RemainTime = hours*60 + minutes // 转换为分钟
		}
	}

	return result.ModelRemains
}

func getAPIKey() string {
	// Try MiniMax API key from minimax relevant settings first
	settingsPath := os.ExpandEnv("$HOME/.openclaw/settings.json")
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		var settings struct {
			APIKey string `json:"MINIMAX_API_KEY"`
		}
		if json.Unmarshal(data, &settings) == nil && settings.APIKey != "" {
			return settings.APIKey
		}
	}

	// Fallback: check env variable
	if apiKey := os.Getenv("MINIMAX_API_KEY"); apiKey != "" {
		return apiKey
	}

	return ""
}
