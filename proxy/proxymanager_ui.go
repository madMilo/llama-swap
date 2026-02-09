package proxy

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type UINavigationItem struct {
	Label  string
	Path   string
	Active bool
}

type UIVersionInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

type UIModel struct {
	ID          string
	Name        string
	Description string
	Source      string
	Aliases     []string
}

type UIRunningProcess struct {
	Model string
	Name  string
	State string
	Proxy string
	TTL   string
}

type UIPageData struct {
	NavItems            []UINavigationItem
	VersionInfo         UIVersionInfo
	Models              []UIModel
	RunningProcesses    []UIRunningProcess
	Logs                string
	PlaygroundTab       string
	ActivityMetrics     []UIActivityMetric
	ActivityCapture     *UIActivityCapture
	ActivityCaptureNote string
	ProxyLogs           string
	UpstreamLogs        string
	LogViewerMode       string
}

func (pm *ProxyManager) uiIndexHandler(c *gin.Context) {
	c.Redirect(http.StatusFound, "/ui/models")
}

func (pm *ProxyManager) uiModelsPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/models")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "pages/models", data)
}

func (pm *ProxyManager) uiRunningPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/running")
	data.RunningProcesses = pm.uiRunningList()
	pm.renderUITemplate(c, "pages/running", data)
}

func (pm *ProxyManager) uiLogsPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/logs")
	data.Logs = string(pm.muxLogger.GetHistory())
	pm.renderUITemplate(c, "pages/logs", data)
}

func (pm *ProxyManager) uiActivityPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/activity")
	data.ActivityMetrics = pm.uiActivityMetrics()
	pm.renderUITemplate(c, "pages/activity", data)
}

func (pm *ProxyManager) uiLogViewerPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/logviewer")
	data.ProxyLogs = string(pm.proxyLogger.GetHistory())
	data.UpstreamLogs = string(pm.upstreamLogger.GetHistory())
	data.LogViewerMode = uiLogViewerMode(c.Query("view"))
	pm.renderUITemplate(c, "pages/logviewer", data)
}

func (pm *ProxyManager) uiPlaygroundPageHandler(c *gin.Context) {
	tab := uiPlaygroundTab(c.Query("tab"))
	data := pm.uiPageData("/ui/playground")
	data.PlaygroundTab = tab
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "pages/playground", data)
}

func (pm *ProxyManager) uiModelsPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/models")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "partials/models", data)
}

func (pm *ProxyManager) uiRunningPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/running")
	data.RunningProcesses = pm.uiRunningList()
	pm.renderUITemplate(c, "partials/running", data)
}

func (pm *ProxyManager) uiLogsPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/logs")
	data.Logs = string(pm.muxLogger.GetHistory())
	pm.renderUITemplate(c, "partials/logs", data)
}

func (pm *ProxyManager) uiActivityPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/activity")
	data.ActivityMetrics = pm.uiActivityMetrics()
	pm.renderUITemplate(c, "partials/activity", data)
}

func (pm *ProxyManager) uiActivityCapturePartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/activity")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		data.ActivityCaptureNote = "Invalid capture ID."
		pm.renderUITemplate(c, "partials/activity_capture", data)
		return
	}
	capture := pm.metricsMonitor.getCaptureByID(id)
	if capture == nil {
		data.ActivityCaptureNote = "Capture not found."
		pm.renderUITemplate(c, "partials/activity_capture", data)
		return
	}
	data.ActivityCapture = uiActivityCapture(capture)
	pm.renderUITemplate(c, "partials/activity_capture", data)
}

func (pm *ProxyManager) uiActivityCaptureClearPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/activity")
	data.ActivityCaptureNote = "Select a capture to view request and response details."
	pm.renderUITemplate(c, "partials/activity_capture", data)
}

func (pm *ProxyManager) uiLogViewerPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/logviewer")
	data.ProxyLogs = string(pm.proxyLogger.GetHistory())
	data.UpstreamLogs = string(pm.upstreamLogger.GetHistory())
	data.LogViewerMode = uiLogViewerMode(c.Query("view"))
	pm.renderUITemplate(c, "partials/logviewer", data)
}

func (pm *ProxyManager) uiPlaygroundChatPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/playground")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "partials/playground_chat", data)
}

func (pm *ProxyManager) uiPlaygroundImagesPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/playground")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "partials/playground_images", data)
}

func (pm *ProxyManager) uiPlaygroundSpeechPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/playground")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "partials/playground_speech", data)
}

func (pm *ProxyManager) uiPlaygroundAudioPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/playground")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "partials/playground_audio", data)
}

func (pm *ProxyManager) uiPageData(activePath string) UIPageData {
	return UIPageData{
		NavItems: []UINavigationItem{
			{Label: "Models", Path: "/ui/models", Active: activePath == "/ui/models"},
			{Label: "Running", Path: "/ui/running", Active: activePath == "/ui/running"},
			{Label: "Activity", Path: "/ui/activity", Active: activePath == "/ui/activity"},
			{Label: "Log Viewer", Path: "/ui/logviewer", Active: activePath == "/ui/logviewer"},
			{Label: "Logs", Path: "/ui/logs", Active: activePath == "/ui/logs"},
			{Label: "Playground", Path: "/ui/playground", Active: activePath == "/ui/playground"},
		},
		VersionInfo: UIVersionInfo{
			Version:   pm.version,
			Commit:    pm.commit,
			BuildDate: pm.buildDate,
		},
	}
}

func uiPlaygroundTab(tab string) string {
	switch tab {
	case "chat", "images", "speech", "audio":
		return tab
	default:
		return "chat"
	}
}

func uiLogViewerMode(mode string) string {
	switch mode {
	case "proxy", "upstream", "panels":
		return mode
	default:
		return "proxy"
	}
}

type UIActivityMetric struct {
	ID               int
	DisplayID        string
	TimeAgo          string
	Model            string
	CachedTokens     string
	InputTokens      string
	OutputTokens     string
	PromptSpeed      string
	GenerationSpeed  string
	Duration         string
	HasCapture       bool
	HasCachedTokens  bool
	CachedTokenValue int
}

type UIActivityCapture struct {
	ID             int
	DisplayID      string
	ReqPath        string
	ReqHeaders     string
	ReqBody        string
	ReqBodyLabel   string
	RespHeaders    string
	RespBody       string
	RespBodyLabel  string
	HasReqBody     bool
	HasRespBody    bool
	HasReqHeaders  bool
	HasRespHeaders bool
}

func (pm *ProxyManager) uiModelsList() []UIModel {
	models := make([]UIModel, 0, len(pm.config.Models))
	for id, modelConfig := range pm.config.Models {
		if modelConfig.Unlisted {
			continue
		}
		aliases := []string{}
		if pm.config.IncludeAliasesInList {
			for _, alias := range modelConfig.Aliases {
				alias = strings.TrimSpace(alias)
				if alias != "" {
					aliases = append(aliases, alias)
				}
			}
		}
		models = append(models, UIModel{
			ID:          id,
			Name:        strings.TrimSpace(modelConfig.Name),
			Description: strings.TrimSpace(modelConfig.Description),
			Source:      "local",
			Aliases:     aliases,
		})
	}

	if pm.peerProxy != nil {
		for peerID, peer := range pm.peerProxy.ListPeers() {
			for _, modelID := range peer.Models {
				models = append(models, UIModel{
					ID:     modelID,
					Name:   fmt.Sprintf("%s: %s", peerID, modelID),
					Source: fmt.Sprintf("peer:%s", peerID),
				})
			}
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return models
}

func (pm *ProxyManager) uiRunningList() []UIRunningProcess {
	processes := make([]UIRunningProcess, 0)
	for _, processGroup := range pm.processGroups {
		for _, process := range processGroup.processes {
			if process.CurrentState() != StateReady {
				continue
			}
			ttl := ""
			if process.config.UnloadAfter > 0 {
				ttl = fmt.Sprintf("%ds", process.config.UnloadAfter)
			}
			processes = append(processes, UIRunningProcess{
				Model: process.ID,
				Name:  strings.TrimSpace(process.config.Name),
				State: string(process.CurrentState()),
				Proxy: strings.TrimSpace(process.config.Proxy),
				TTL:   ttl,
			})
		}
	}

	sort.Slice(processes, func(i, j int) bool {
		return processes[i].Model < processes[j].Model
	})

	return processes
}

func (pm *ProxyManager) uiActivityMetrics() []UIActivityMetric {
	metrics := pm.metricsMonitor.getMetrics()
	if len(metrics) == 0 {
		return nil
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].ID > metrics[j].ID
	})

	result := make([]UIActivityMetric, 0, len(metrics))
	for _, metric := range metrics {
		cachedTokens := "-"
		if metric.CachedTokens > 0 {
			cachedTokens = formatNumber(metric.CachedTokens)
		}
		result = append(result, UIActivityMetric{
			ID:              metric.ID,
			DisplayID:       formatNumber(metric.ID + 1),
			TimeAgo:         formatRelativeTime(metric.Timestamp),
			Model:           metric.Model,
			CachedTokens:    cachedTokens,
			InputTokens:     formatNumber(metric.InputTokens),
			OutputTokens:    formatNumber(metric.OutputTokens),
			PromptSpeed:     formatSpeed(metric.PromptPerSecond),
			GenerationSpeed: formatSpeed(metric.TokensPerSecond),
			Duration:        formatDuration(metric.DurationMs),
			HasCapture:      metric.HasCapture,
		})
	}
	return result
}

func uiActivityCapture(capture *ReqRespCapture) *UIActivityCapture {
	reqBody, reqBodyLabel := formatBody(capture.ReqBody)
	respBody, respBodyLabel := formatBody(capture.RespBody)
	reqHeaders := formatHeaders(capture.ReqHeaders)
	respHeaders := formatHeaders(capture.RespHeaders)
	return &UIActivityCapture{
		ID:             capture.ID,
		DisplayID:      formatNumber(capture.ID + 1),
		ReqPath:        capture.ReqPath,
		ReqHeaders:     reqHeaders,
		ReqBody:        reqBody,
		ReqBodyLabel:   reqBodyLabel,
		RespHeaders:    respHeaders,
		RespBody:       respBody,
		RespBodyLabel:  respBodyLabel,
		HasReqBody:     len(capture.ReqBody) > 0,
		HasRespBody:    len(capture.RespBody) > 0,
		HasReqHeaders:  len(capture.ReqHeaders) > 0,
		HasRespHeaders: len(capture.RespHeaders) > 0,
	}
}

func formatNumber(value int) string {
	s := strconv.Itoa(value)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteString(",")
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteString(",")
		}
	}
	return b.String()
}

func formatSpeed(speed float64) string {
	if speed < 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.2f t/s", speed)
}

func formatDuration(ms int) string {
	return fmt.Sprintf("%.2fs", float64(ms)/1000)
}

func formatRelativeTime(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "unknown"
	}
	now := time.Now()
	diffSeconds := int(now.Sub(timestamp).Seconds())
	if diffSeconds < 5 {
		return "now"
	}
	if diffSeconds < 60 {
		return fmt.Sprintf("%ds ago", diffSeconds)
	}
	diffMinutes := diffSeconds / 60
	if diffMinutes < 60 {
		return fmt.Sprintf("%dm ago", diffMinutes)
	}
	diffHours := diffMinutes / 60
	if diffHours < 24 {
		return fmt.Sprintf("%dh ago", diffHours)
	}
	return "a while ago"
}

func formatHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", key, headers[key]))
	}
	return strings.Join(lines, "\n")
}

func formatBody(body []byte) (string, string) {
	if len(body) == 0 {
		return "", "Body"
	}
	if utf8.Valid(body) {
		return string(body), "Body"
	}
	return base64.StdEncoding.EncodeToString(body), "Body (base64)"
}

func (pm *ProxyManager) renderUITemplate(c *gin.Context, name string, data UIPageData) {
	if pm.uiTemplates == nil {
		c.String(http.StatusInternalServerError, "UI templates unavailable")
		return
	}
	tmpl := pm.uiTemplates.Template(name)
	if tmpl == nil {
		c.String(http.StatusInternalServerError, "UI template not found")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
	}
}
