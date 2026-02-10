package proxy

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/mostlygeek/llama-swap/proxy/config"
)

type ProcessGroup struct {
	sync.Mutex

	id string

	proxyLogger    *LogMonitor
	upstreamLogger *LogMonitor

	// map of current processes
	processes       map[string]*Process
	lastUsedProcess string

	scheduler *Scheduler
	tracker   *MemoryTracker
}

func NewProcessGroup(modelID string, cfg config.Config, proxyLogger *LogMonitor, upstreamLogger *LogMonitor) *ProcessGroup {
	pg := &ProcessGroup{
		id:             modelID,
		proxyLogger:    proxyLogger,
		upstreamLogger: upstreamLogger,
		processes:      make(map[string]*Process),
	}

	modelConfig, _, found := cfg.FindConfig(modelID)
	if !found {
		panic("Unable to find model configuration for model id: " + modelID)
	}

	processLogger := NewLogMonitorWriter(upstreamLogger)
	process := NewProcess(modelID, cfg.HealthCheckTimeout, modelConfig, processLogger, pg.proxyLogger)
	if pg.tracker != nil {
		process.SetMemoryTracker(pg.tracker, signatureForModel(modelID, modelConfig.Cmd))
	}
	pg.processes[modelID] = process

	return pg
}

func (pg *ProcessGroup) SetScheduler(scheduler *Scheduler) {
	pg.scheduler = scheduler
	if scheduler == nil {
		for _, process := range pg.processes {
			process.SetPreStartHook(nil)
		}
		return
	}
	for _, process := range pg.processes {
		process.SetPreStartHook(func(proc *Process) error {
			return scheduler.ScheduleProcess(proc)
		})
	}
}

func (pg *ProcessGroup) SetMemoryTracker(tracker *MemoryTracker) {
	pg.tracker = tracker
	for _, process := range pg.processes {
		process.SetMemoryTracker(tracker, signatureForModel(process.ID, process.config.Cmd))
	}
}

// ProxyRequest proxies a request to the specified model
func (pg *ProcessGroup) ProxyRequest(modelID string, writer http.ResponseWriter, request *http.Request) error {
	if !pg.HasMember(modelID) {
		return fmt.Errorf("model %s not found", modelID)
	}
	pg.processes[modelID].ProxyRequest(writer, request)
	return nil
}

func (pg *ProcessGroup) HasMember(modelName string) bool {
	_, ok := pg.processes[modelName]
	return ok
}

func (pg *ProcessGroup) GetMember(modelName string) (*Process, bool) {
	if process, ok := pg.processes[modelName]; ok {
		return process, true
	}
	return nil, false
}

func (pg *ProcessGroup) StopProcess(modelID string, strategy StopStrategy) error {
	pg.Lock()

	process, exists := pg.processes[modelID]
	if !exists {
		pg.Unlock()
		return fmt.Errorf("process not found for %s", modelID)
	}

	if pg.lastUsedProcess == modelID {
		pg.lastUsedProcess = ""
	}
	pg.Unlock()

	switch strategy {
	case StopImmediately:
		process.StopImmediately()
	default:
		process.Stop()
	}
	return nil
}

func (pg *ProcessGroup) StopProcesses(strategy StopStrategy) {
	pg.Lock()
	defer pg.Unlock()

	if len(pg.processes) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, process := range pg.processes {
		wg.Add(1)
		go func(process *Process) {
			defer wg.Done()
			switch strategy {
			case StopImmediately:
				process.StopImmediately()
			default:
				process.Stop()
			}
		}(process)
	}
	wg.Wait()
}

func (pg *ProcessGroup) Shutdown() {
	var wg sync.WaitGroup
	for _, process := range pg.processes {
		wg.Add(1)
		go func(process *Process) {
			defer wg.Done()
			process.Shutdown()
		}(process)
	}
	wg.Wait()
}
