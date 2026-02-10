package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// PlaygroundChatRequest represents a chat request from the playground
type PlaygroundChatRequest struct {
	Model       string `json:"model"`
	Message     string `json:"message"`
	System      string `json:"system"`
	Temperature float64 `json:"temperature"`
	SessionID   string `json:"sessionId"`
}

// apiPlaygroundChat handles playground chat requests
func (pm *ProxyManager) apiPlaygroundChat(c *gin.Context) {
	var req PlaygroundChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate inputs
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	if req.SessionID == "" {
		req.SessionID = "default"
	}

	// Get or create session
	session := pm.playgroundSessions.GetOrCreateSession(req.SessionID)

	// Build messages array
	messages := make([]map[string]interface{}, 0)

	// Add system message if provided
	if req.System != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": req.System,
		})
	}

	// Add conversation history
	for _, msg := range session.Messages {
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	// Add new user message
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": req.Message,
	})

	// Save user message to session
	pm.playgroundSessions.AddMessage(req.SessionID, PlaygroundChatMessage{
		Role:    "user",
		Content: req.Message,
	})

	// Build request to model
	chatReq := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   true,
	}

	if req.Temperature > 0 {
		chatReq["temperature"] = req.Temperature
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Create request to upstream
	httpReq, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Get the process group and make the request
	processGroup, err := pm.swapProcessGroup(req.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load model: %v", err)})
		return
	}

	// Create a response writer to capture the streaming response
	recorder := &streamRecorder{
		header:    make(http.Header),
		body:      new(bytes.Buffer),
		sessionID: req.SessionID,
		pm:        pm,
		c:         c,
	}

	// Make the request
	processGroup.ProxyRequest(req.Model, recorder, httpReq)
}

// streamRecorder captures streaming responses and broadcasts to WebSocket
type streamRecorder struct {
	header    http.Header
	body      *bytes.Buffer
	sessionID string
	pm        *ProxyManager
	c         *gin.Context
	wroteHeader bool
}

func (r *streamRecorder) Header() http.Header {
	return r.header
}

func (r *streamRecorder) Write(data []byte) (int, error) {
	// Set headers if not already set
	if !r.wroteHeader {
		r.c.Header("Content-Type", "text/event-stream")
		r.c.Header("Cache-Control", "no-cache")
		r.c.Header("Connection", "keep-alive")
		r.c.Status(http.StatusOK)
		r.wroteHeader = true
	}

	// Write data to client
	n, err := r.c.Writer.Write(data)
	if err != nil {
		return n, err
	}

	// Also accumulate for session
	r.body.Write(data)

	// Flush to client
	r.c.Writer.Flush()

	return n, nil
}

func (r *streamRecorder) WriteHeader(statusCode int) {
	if !r.wroteHeader {
		for key, values := range r.header {
			for _, value := range values {
				r.c.Header(key, value)
			}
		}
		r.c.Status(statusCode)
		r.wroteHeader = true
	}
}

// After streaming is complete, extract assistant message and save
func (r *streamRecorder) Finish() {
	// Parse SSE stream to extract assistant response
	content := extractContentFromSSE(r.body.String())
	if content != "" {
		r.pm.playgroundSessions.AddMessage(r.sessionID, PlaygroundChatMessage{
			Role:    "assistant",
			Content: content,
		})
	}
}

// extractContentFromSSE extracts the content from an SSE stream
func extractContentFromSSE(sseData string) string {
	var fullContent strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(sseData))

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "[DONE]" {
			break
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// Extract delta content
		if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if content, ok := delta["content"].(string); ok {
						fullContent.WriteString(content)
					}
				}
			}
		}
	}

	return fullContent.String()
}

// apiPlaygroundClearChat clears the chat history
func (pm *ProxyManager) apiPlaygroundClearChat(c *gin.Context) {
	sessionID := c.DefaultQuery("sessionId", "default")
	pm.playgroundSessions.ClearSession(sessionID)
	c.JSON(http.StatusOK, gin.H{"message": "chat cleared"})
}

// apiPlaygroundGetHistory returns the chat history
func (pm *ProxyManager) apiPlaygroundGetHistory(c *gin.Context) {
	sessionID := c.DefaultQuery("sessionId", "default")
	messages := pm.playgroundSessions.GetMessages(sessionID)
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// apiPlaygroundGenerateImage handles image generation
func (pm *ProxyManager) apiPlaygroundGenerateImage(c *gin.Context) {
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Model == "" || req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model and prompt are required"})
		return
	}

	// Build request
	imageReq := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}

	if req.Size != "" {
		imageReq["size"] = req.Size
	}

	reqBody, err := json.Marshal(imageReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", "/v1/images/generations", bytes.NewReader(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Get process group
	processGroup, err := pm.swapProcessGroup(req.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load model: %v", err)})
		return
	}

	// Proxy the request
	processGroup.ProxyRequest(req.Model, c.Writer, httpReq)
}

// apiPlaygroundGenerateSpeech handles TTS generation
func (pm *ProxyManager) apiPlaygroundGenerateSpeech(c *gin.Context) {
	var req struct {
		Model string `json:"model"`
		Input string `json:"input"`
		Voice string `json:"voice"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Model == "" || req.Input == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model and input are required"})
		return
	}

	// Build request
	speechReq := map[string]interface{}{
		"model": req.Model,
		"input": req.Input,
	}

	if req.Voice != "" {
		speechReq["voice"] = req.Voice
	}

	reqBody, err := json.Marshal(speechReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", "/v1/audio/speech", bytes.NewReader(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Get process group
	processGroup, err := pm.swapProcessGroup(req.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load model: %v", err)})
		return
	}

	// Proxy the request
	processGroup.ProxyRequest(req.Model, c.Writer, httpReq)
}

// apiPlaygroundTranscribeAudio handles audio transcription
func (pm *ProxyManager) apiPlaygroundTranscribeAudio(c *gin.Context) {
	model := c.PostForm("model")
	if model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}

	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}
	defer src.Close()

	// Read file content
	fileContent, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	// Create multipart form data
	body := new(bytes.Buffer)
	writer := createMultipartFormData(body, "file", file.Filename, fileContent, model)

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", "/v1/audio/transcriptions", body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	httpReq.Header.Set("Content-Type", writer)

	// Get process group
	processGroup, err := pm.swapProcessGroup(model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load model: %v", err)})
		return
	}

	// Proxy the request
	processGroup.ProxyRequest(model, c.Writer, httpReq)
}

// Helper to create multipart form data
func createMultipartFormData(body *bytes.Buffer, fieldName, fileName string, fileContent []byte, model string) string {
	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n", fieldName, fileName))
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(fileContent)
	body.WriteString("\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"model\"\r\n\r\n")
	body.WriteString(model)
	body.WriteString("\r\n")

	body.WriteString("--" + boundary + "--\r\n")

	return "multipart/form-data; boundary=" + boundary
}
