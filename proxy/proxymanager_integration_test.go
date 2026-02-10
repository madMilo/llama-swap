package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mostlygeek/llama-swap/proxy/config"
	"github.com/stretchr/testify/assert"
)

type integrationGPUAllocator struct {
	baseFree []uint64
	total    []uint64
	provider func() []*Process
}

func (f *integrationGPUAllocator) GetGPUs() ([]GPUInfo, error) {
	usage := make(map[int]uint64)
	if f.provider != nil {
		for _, process := range f.provider() {
			if process.AssignedGPU() >= 0 {
				usage[process.AssignedGPU()] += process.MeasuredVramMB()
			}
		}
	}

	gpus := make([]GPUInfo, 0, len(f.total))
	for index, total := range f.total {
		free := total
		if index < len(f.baseFree) {
			free = f.baseFree[index]
		}
		if used := usage[index]; used > 0 {
			if used >= free {
				free = 0
			} else {
				free -= used
			}
		}
		gpus = append(gpus, GPUInfo{
			Index:   index,
			TotalMB: total,
			FreeMB:  free,
		})
	}
	return gpus, nil
}

func TestProxyManager_DualGPUIntegration(t *testing.T) {
	const (
		gpuCapMB          = 24576
		hostCapMB         = 245760
		singleInitialVRAM = 23347
		dualInitialVRAM   = 46759
		dualInitialCPU    = 245760
	)

	modelA := getTestSimpleResponderConfig("model-a")
	modelA.Name = "Single GPU A"
	modelA.FitPolicy = "evict_to_fit"
	modelA.InitialVramMB = singleInitialVRAM

	modelB := getTestSimpleResponderConfig("model-b")
	modelB.Name = "Single GPU B"
	modelB.FitPolicy = "evict_to_fit"
	modelB.InitialVramMB = singleInitialVRAM

	modelDual := getTestSimpleResponderConfig("model-dual")
	modelDual.Name = "Dual GPU"
	modelDual.FitPolicy = "spill"
	modelDual.InitialVramMB = dualInitialVRAM
	modelDual.InitialCpuMB = dualInitialCPU
	modelDual.Env = []string{"CUDA_VISIBLE_DEVICES=0,1"}

	testConfig := config.AddDefaultGroupToConfig(config.Config{
		HealthCheckTimeout: 15,
		LogLevel:           "error",
		GpuVramCapsMB:      []uint64{gpuCapMB, gpuCapMB},
		HostRamCapMB:       hostCapMB,
		Models: map[string]config.ModelConfig{
			"model-a":    modelA,
			"model-b":    modelB,
			"model-dual": modelDual,
		},
		Groups: map[string]config.GroupConfig{
			"all": {
				Swap:      false,
				Exclusive: false,
				Members:   []string{"model-a", "model-b", "model-dual"},
			},
		},
	})

	allocator := &integrationGPUAllocator{
		total:    []uint64{gpuCapMB, gpuCapMB},
		baseFree: []uint64{gpuCapMB, 1000},
	}
	proxy := NewWithAllocator(testConfig, allocator)
	defer proxy.StopProcesses(StopWaitForInflightRequest)
	allocator.provider = proxy.runningProcesses

	proxy.memoryTracker.Set(signatureForModel("model-a", modelA.Cmd), MemoryFootprint{
		VramMB:     20000,
		CpuMB:      150000,
		RecordedAt: time.Now(),
	})
	proxy.memoryTracker.Set(signatureForModel("model-b", modelB.Cmd), MemoryFootprint{
		VramMB:     23347,
		CpuMB:      150000,
		RecordedAt: time.Now(),
	})
	proxy.memoryTracker.Set(signatureForModel("model-dual", modelDual.Cmd), MemoryFootprint{
		VramMB:     46759,
		CpuMB:      245760,
		RecordedAt: time.Now(),
	})

	processGroup := proxy.findGroupByModelName("model-a")
	if processGroup == nil {
		t.Fatal("expected process group for model-a")
	}

	modelAProcess := processGroup.processes["model-a"]
	modelBProcess := processGroup.processes["model-b"]
	modelDualProcess := processGroup.processes["model-dual"]

	assert.NoError(t, proxy.scheduler.ScheduleProcess(modelAProcess))
	modelAProcess.forceState(StateReady)
	modelAProcess.setLastRequestHandled(time.Now().Add(-10 * time.Minute))

	assert.NoError(t, proxy.scheduler.ScheduleProcess(modelBProcess))
	modelBProcess.forceState(StateReady)
	assert.Equal(t, StateStopping, modelAProcess.CurrentState())
	assert.Equal(t, 0, modelBProcess.AssignedGPU())

	assert.NoError(t, proxy.scheduler.ScheduleProcess(modelDualProcess))
	modelDualProcess.forceState(StateReady)

	apiReq := httptest.NewRequest("GET", "/api/models", nil)
	apiResp := CreateTestResponseRecorder()
	proxy.ServeHTTP(apiResp, apiReq)
	assert.Equal(t, http.StatusOK, apiResp.Code)

	var apiModels []Model
	assert.NoError(t, json.Unmarshal(apiResp.Body.Bytes(), &apiModels))

	apiByID := map[string]Model{}
	for _, model := range apiModels {
		apiByID[model.Id] = model
	}

	assert.Equal(t, uint64(20000), apiByID["model-a"].MeasuredVramMB)
	assert.Equal(t, uint64(150000), apiByID["model-a"].MeasuredCpuMB)
	assert.Equal(t, uint64(23347), apiByID["model-b"].MeasuredVramMB)
	assert.Equal(t, uint64(150000), apiByID["model-b"].MeasuredCpuMB)

	uiReq := httptest.NewRequest("GET", "/ui/recommendations", nil)
	uiResp := CreateTestResponseRecorder()
	proxy.ServeHTTP(uiResp, uiReq)
	assert.Equal(t, http.StatusOK, uiResp.Code)

	body := uiResp.Body.String()
	assert.Contains(t, body, "Host RAM cap is 245760 MB, but measured host usage totals 300000 MB for non-spill models.")
	assert.Contains(t, body, "20000 MB")
	assert.Contains(t, body, "23347 MB")
	assert.Contains(t, body, "46759 MB")
}
