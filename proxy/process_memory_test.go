package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProcess_MemoryTrackerUpdates(t *testing.T) {
	expectedMessage := "memory-check"
	port := getTestPort()
	config := getTestSimpleResponderConfigPortArgs(expectedMessage, port,
		"--memory-log-interval", "25ms",
		"--memory-log-format", "both",
		"--memory-log-vram-mb", "46759",
		"--memory-log-cpu-mb", "248000",
		"--memory-log-count", "4",
	)
	config.InitialVramMB = 23347
	config.InitialCpuMB = 245760

	process := NewProcess("memory-process", 5, config, debugLogger, debugLogger)
	defer process.Stop()

	tracker := NewMemoryTracker()
	signature := signatureForModel("memory-model", config.Cmd)
	process.SetMemoryTracker(tracker, signature)

	assert.Equal(t, config.InitialVramMB, process.MeasuredVramMB())
	assert.Equal(t, config.InitialCpuMB, process.MeasuredCpuMB())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	process.ProxyRequest(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), expectedMessage)

	assert.Eventually(t, func() bool {
		return process.MeasuredVramMB() == 46759 && process.MeasuredCpuMB() == 248000
	}, 2*time.Second, 25*time.Millisecond)

	assert.NotEqual(t, config.InitialVramMB, process.MeasuredVramMB())
	assert.NotEqual(t, config.InitialCpuMB, process.MeasuredCpuMB())
	assert.Greater(t, process.MeasuredVramMB(), config.InitialVramMB)
	assert.Greater(t, process.MeasuredCpuMB(), config.InitialCpuMB)

	if footprint, ok := tracker.Get(signature); ok {
		assert.Equal(t, uint64(46759), footprint.VramMB)
		assert.Equal(t, uint64(248000), footprint.CpuMB)
	} else {
		t.Fatalf("expected tracker footprint for signature %s", signature)
	}
}

func TestProcess_RuntimeFootprint_ExcludesInitialHintsUntilObserved(t *testing.T) {
	cfg := getTestSimpleResponderConfig("runtime-footprint")
	cfg.InitialVramMB = 22000
	cfg.InitialCpuMB = 120000

	process := NewProcess("runtime-footprint", 1, cfg, testLogger, testLogger)
	tracker := NewMemoryTracker()
	signature := signatureForModel("runtime-footprint", cfg.Cmd)
	process.SetMemoryTracker(tracker, signature)

	_, ok := process.RuntimeFootprint()
	assert.False(t, ok)

	tracker.Set(signature, MemoryFootprint{VramMB: 1616, CpuMB: 0, RecordedAt: time.Now()})
	footprint, ok := process.RuntimeFootprint()
	assert.True(t, ok)
	assert.Equal(t, uint64(1616), footprint.VramMB)
	assert.Equal(t, uint64(0), footprint.CpuMB)
}
