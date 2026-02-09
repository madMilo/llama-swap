package proxy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrInsufficientVRAM = errors.New("insufficient vram for scheduling")
var ErrUnknownFootprint = errors.New("unknown model memory footprint")

type GPUInfo struct {
	Index   int
	FreeMB  uint64
	TotalMB uint64
}

type GPUAllocator interface {
	GetGPUs() ([]GPUInfo, error)
}

type Scheduler struct {
	allocator GPUAllocator
	logger    *LogMonitor
	provider  func() []*Process
	mu        sync.Mutex
}

func NewScheduler(allocator GPUAllocator, logger *LogMonitor, provider func() []*Process) *Scheduler {
	return &Scheduler{
		allocator: allocator,
		logger:    logger,
		provider:  provider,
	}
}

func (s *Scheduler) ScheduleProcess(process *Process) error {
	if strings.ToLower(process.FitPolicy()) != "evict_to_fit" {
		return nil
	}

	requiredMB := process.MeasuredVramMB()
	if requiredMB == 0 {
		return ErrUnknownFootprint
	}

	if requiredMB == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	gpus, err := s.allocator.GetGPUs()
	if err != nil {
		return err
	}
	if len(gpus) == 0 {
		return fmt.Errorf("no GPUs detected for scheduling")
	}

	type candidate struct {
		gpuIndex int
		evict    []*Process
		freeMB   uint64
	}

	running := s.provider()
	var candidates []candidate
	for _, gpu := range gpus {
		assigned := processesOnGPU(running, gpu.Index)
		evictable, ok := s.selectEvictions(process, assigned, gpu.FreeMB, requiredMB)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{
			gpuIndex: gpu.Index,
			evict:    evictable,
			freeMB:   gpu.FreeMB,
		})
	}

	if len(candidates) == 0 {
		return ErrInsufficientVRAM
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].freeMB > candidates[j].freeMB
	})

	chosen := candidates[0]
	for _, evicted := range chosen.evict {
		evicted.StopImmediately()
	}

	process.SetAssignedGPU(chosen.gpuIndex)
	process.SetRuntimeEnv([]string{fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", chosen.gpuIndex)})

	return nil
}

func (s *Scheduler) selectEvictions(process *Process, assigned []*Process, freeMB, requiredMB uint64) ([]*Process, bool) {
	if hasUnknownFootprint(assigned) {
		return nil, false
	}

	if process.GroupExclusive {
		evict, ok := evictAllIdle(assigned)
		if !ok {
			return nil, false
		}
		if freeMB+sumVram(evict) < requiredMB {
			return nil, false
		}
		return evict, true
	}

	if hasExclusiveProcess(assigned) {
		evict, ok := evictAllIdle(assigned)
		if !ok {
			return nil, false
		}
		if freeMB+sumVram(evict) < requiredMB {
			return nil, false
		}
		return evict, true
	}

	if freeMB >= requiredMB {
		return nil, true
	}

	evictable := idleProcesses(assigned)
	sort.Slice(evictable, func(i, j int) bool {
		return evictable[i].LastRequestHandled().Before(evictable[j].LastRequestHandled())
	})

	var evict []*Process
	currentFree := freeMB
	for _, candidate := range evictable {
		evict = append(evict, candidate)
		currentFree += candidate.MeasuredVramMB()
		if currentFree >= requiredMB {
			return evict, true
		}
	}

	return nil, false
}

func processesOnGPU(processes []*Process, gpuIndex int) []*Process {
	var assigned []*Process
	for _, process := range processes {
		if process.AssignedGPU() == gpuIndex && process.CurrentState() == StateReady {
			assigned = append(assigned, process)
		}
	}
	return assigned
}

func hasExclusiveProcess(processes []*Process) bool {
	for _, process := range processes {
		if process.GroupExclusive {
			return true
		}
	}
	return false
}

func idleProcesses(processes []*Process) []*Process {
	var idle []*Process
	for _, process := range processes {
		if process.InFlightRequestsCount() == 0 {
			idle = append(idle, process)
		}
	}
	return idle
}

func evictAllIdle(processes []*Process) ([]*Process, bool) {
	var evict []*Process
	for _, process := range processes {
		if process.InFlightRequestsCount() != 0 {
			return nil, false
		}
		evict = append(evict, process)
	}
	return evict, true
}

func sumVram(processes []*Process) uint64 {
	var total uint64
	for _, process := range processes {
		total += process.MeasuredVramMB()
	}
	return total
}

func hasUnknownFootprint(processes []*Process) bool {
	for _, process := range processes {
		if process.MeasuredVramMB() == 0 {
			return true
		}
	}
	return false
}
