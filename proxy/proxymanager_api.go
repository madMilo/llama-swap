package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mostlygeek/llama-swap/event"
)

type Model struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	State          string `json:"state"`
	Unlisted       bool   `json:"unlisted"`
	PeerID         string `json:"peerID"`
	MeasuredVramMB uint64 `json:"measuredVramMB,omitempty"`
	MeasuredCpuMB  uint64 `json:"measuredCpuMB,omitempty"`
	FitPolicy      string `json:"fitPolicy,omitempty"`
	InitialVramMB  uint64 `json:"initialVramMB,omitempty"`
	InitialCpuMB   uint64 `json:"initialCpuMB,omitempty"`
}

func addApiHandlers(pm *ProxyManager) {
	// Add API endpoints for React to consume
	// Protected with API key authentication
	apiGroup := pm.ginEngine.Group("/api", pm.apiKeyAuth())
	{
		apiGroup.GET("/models", pm.apiGetModels)
		apiGroup.POST("/models/unload", pm.apiUnloadAllModels)
		apiGroup.POST("/models/unload/*model", pm.apiUnloadSingleModelHandler)
		apiGroup.POST("/models/load/*model", pm.apiLoadSingleModelHandler)
		apiGroup.GET("/events", pm.apiSendEvents)
		apiGroup.GET("/metrics", pm.apiGetMetrics)
		apiGroup.GET("/version", pm.apiGetVersion)
		apiGroup.GET("/captures/:id", pm.apiGetCapture)
		apiGroup.GET("/ws", pm.HandleWebSocket)
	}

}

func (pm *ProxyManager) apiUnloadAllModels(c *gin.Context) {
	pm.StopProcesses(StopImmediately)
	c.JSON(http.StatusOK, gin.H{"msg": "ok"})
}

func (pm *ProxyManager) apiGetModels(c *gin.Context) {
	c.JSON(http.StatusOK, pm.getModelStatus())
}

func (pm *ProxyManager) getModelStatus() []Model {
	// Extract keys and sort them
	models := []Model{}

	modelIDs := make([]string, 0, len(pm.config.Models))
	for modelID := range pm.config.Models {
		modelIDs = append(modelIDs, modelID)
	}
	sort.Strings(modelIDs)

	// Iterate over sorted keys
	for _, modelID := range modelIDs {
		// Get process state
		process := pm.findProcessByModelName(modelID)
		state := "unknown"
		var measuredVramMB uint64
		var measuredCpuMB uint64
		if process != nil {
			measuredVramMB = process.MeasuredVramMB()
			measuredCpuMB = process.MeasuredCpuMB()
			var stateStr string
			switch process.CurrentState() {
			case StateReady:
				stateStr = "ready"
			case StateStarting:
				stateStr = "starting"
			case StateStopping:
				stateStr = "stopping"
			case StateShutdown:
				stateStr = "shutdown"
			case StateStopped:
				stateStr = "stopped"
			default:
				stateStr = "unknown"
			}
			state = stateStr
		}
		models = append(models, Model{
			Id:             modelID,
			Name:           pm.config.Models[modelID].Name,
			Description:    pm.config.Models[modelID].Description,
			State:          state,
			Unlisted:       pm.config.Models[modelID].Unlisted,
			MeasuredVramMB: measuredVramMB,
			MeasuredCpuMB:  measuredCpuMB,
			FitPolicy:      pm.config.Models[modelID].FitPolicy,
			InitialVramMB:  pm.config.Models[modelID].InitialVramMB,
			InitialCpuMB:   pm.config.Models[modelID].InitialCpuMB,
		})
	}

	// Iterate over the peer models
	if pm.peerProxy != nil {
		for peerID, peer := range pm.peerProxy.ListPeers() {
			for _, modelID := range peer.Models {
				models = append(models, Model{
					Id:     modelID,
					PeerID: peerID,
				})
			}
		}
	}

	return models
}

type messageType string

const (
	msgTypeModelStatus messageType = "modelStatus"
	msgTypeLogData     messageType = "logData"
	msgTypeMetrics     messageType = "metrics"
)

type messageEnvelope struct {
	Type messageType `json:"type"`
	Data string      `json:"data"`
}

// sends a stream of different message types that happen on the server
func (pm *ProxyManager) apiSendEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Content-Type-Options", "nosniff")
	// prevent nginx from buffering SSE
	c.Header("X-Accel-Buffering", "no")

	sendBuffer := make(chan messageEnvelope, 25)
	ctx, cancel := context.WithCancel(c.Request.Context())
	sendModels := func() {
		data, err := json.Marshal(pm.getModelStatus())
		if err == nil {
			msg := messageEnvelope{Type: msgTypeModelStatus, Data: string(data)}
			select {
			case sendBuffer <- msg:
			case <-ctx.Done():
				return
			default:
			}

		}
	}

	sendLogData := func(source string, data []byte) {
		data, err := json.Marshal(gin.H{
			"source": source,
			"data":   string(data),
		})
		if err == nil {
			select {
			case sendBuffer <- messageEnvelope{Type: msgTypeLogData, Data: string(data)}:
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	sendMetrics := func(metrics []TokenMetrics) {
		jsonData, err := json.Marshal(metrics)
		if err == nil {
			select {
			case sendBuffer <- messageEnvelope{Type: msgTypeMetrics, Data: string(jsonData)}:
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	/**
	 * Send updated models list
	 */
	defer event.On(func(e ProcessStateChangeEvent) {
		sendModels()
	})()
	defer event.On(func(e ConfigFileChangedEvent) {
		sendModels()
	})()

	/**
	 * Send Log data
	 */
	defer pm.proxyLogger.OnLogData(func(data []byte) {
		sendLogData("proxy", data)
	})()
	defer pm.upstreamLogger.OnLogData(func(data []byte) {
		sendLogData("upstream", data)
	})()

	/**
	 * Send Metrics data
	 */
	defer event.On(func(e TokenMetricsEvent) {
		sendMetrics([]TokenMetrics{e.Metrics})
	})()

	// send initial batch of data
	sendLogData("proxy", pm.proxyLogger.GetHistory())
	sendLogData("upstream", pm.upstreamLogger.GetHistory())
	sendModels()
	sendMetrics(pm.metricsMonitor.getMetrics())

	for {
		select {
		case <-c.Request.Context().Done():
			cancel()
			return
		case <-pm.shutdownCtx.Done():
			cancel()
			return
		case msg := <-sendBuffer:
			c.SSEvent("message", msg)
			c.Writer.Flush()
		}
	}
}

func (pm *ProxyManager) apiGetMetrics(c *gin.Context) {
	jsonData, err := pm.metricsMonitor.getMetricsJSON()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get metrics"})
		return
	}
	c.Data(http.StatusOK, "application/json", jsonData)
}

func (pm *ProxyManager) apiUnloadSingleModelHandler(c *gin.Context) {
	requestedModel := strings.TrimPrefix(c.Param("model"), "/")
	realModelName, found := pm.config.RealModelName(requestedModel)
	if !found {
		pm.sendErrorResponse(c, http.StatusNotFound, "Model not found")
		return
	}

	process := pm.findProcessByModelName(realModelName)
	if process == nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("process not found for model %s", requestedModel))
		return
	}

	process.StopImmediately()
	c.String(http.StatusOK, "OK")
}

func (pm *ProxyManager) apiLoadSingleModelHandler(c *gin.Context) {
	requestedModel := strings.TrimPrefix(c.Param("model"), "/")
	realModelName, found := pm.config.RealModelName(requestedModel)
	if !found {
		pm.sendErrorResponse(c, http.StatusNotFound, "Model not found")
		return
	}

	// Use swapProcessGroup to load the model
	_, err := pm.swapProcessGroup(realModelName)
	if err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("failed to load model: %v", err))
		return
	}

	c.String(http.StatusOK, "OK")
}

func (pm *ProxyManager) apiGetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]string{
		"version":    pm.version,
		"commit":     pm.commit,
		"build_date": pm.buildDate,
	})
}

func (pm *ProxyManager) apiGetCapture(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid capture ID"})
		return
	}

	capture := pm.metricsMonitor.getCaptureByID(id)
	if capture == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "capture not found"})
		return
	}

	c.JSON(http.StatusOK, capture)
}
