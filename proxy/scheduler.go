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
var ErrInsufficientHostRAM = errors.New("insufficient host ram for scheduling")

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

	gpuVramCapMB  uint64
	gpuVramCapsMB []uint64
	hostRamCapMB  uint64
}

type SchedulerOptions struct {
	GpuVramCapMB  uint64
	GpuVramCapsMB []uint64
	HostRamCapMB  uint64
}

func NewScheduler(allocator GPUAllocator, logger *LogMonitor, provider func() []*Process, opts SchedulerOptions) *Scheduler {
	return &Scheduler{
		allocator:     allocator,
		logger:        logger,
		provider:      provider,
		gpuVramCapMB:  opts.GpuVramCapMB,
		gpuVramCapsMB: append([]uint64(nil), opts.GpuVramCapsMB...),
		hostRamCapMB:  opts.HostRamCapMB,
	}
}

func (s *Scheduler) ScheduleProcess(process *Process) error {
	if err := s.ensureHostRamCapacity(process); err != nil {
		return err
	}
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
	gpus = s.applyVramCaps(gpus)
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

func (s *Scheduler) applyVramCaps(gpus []GPUInfo) []GPUInfo {
	if s.gpuVramCapMB == 0 && len(s.gpuVramCapsMB) == 0 {
		return gpus
	}
	capped := make([]GPUInfo, 0, len(gpus))
	for _, gpu := range gpus {
		capMB := s.vramCapForGPU(gpu.Index)
		if capMB > 0 {
			if gpu.TotalMB > capMB {
				gpu.TotalMB = capMB
			}
			if gpu.FreeMB > capMB {
				gpu.FreeMB = capMB
			}
			if gpu.FreeMB > gpu.TotalMB {
				gpu.FreeMB = gpu.TotalMB
			}
		}
		capped = append(capped, gpu)
	}
	return capped
}

func (s *Scheduler) vramCapForGPU(index int) uint64 {
	if index >= 0 && index < len(s.gpuVramCapsMB) && s.gpuVramCapsMB[index] > 0 {
		return s.gpuVramCapsMB[index]
	}
	return s.gpuVramCapMB
}

func (s *Scheduler) ensureHostRamCapacity(process *Process) error {
	if s.hostRamCapMB == 0 || !shouldAccountHostRam(process) {
		return nil
	}

	requiredMB := process.MeasuredCpuMB()
	if requiredMB == 0 {
		return ErrUnknownFootprint
	}

	running := s.provider()
	total, ok := sumCpuMB(running)
	if !ok {
		return ErrUnknownFootprint
	}

	if total+requiredMB > s.hostRamCapMB {
		return ErrInsufficientHostRAM
	}
	return nil
}

func shouldAccountHostRam(process *Process) bool {
	return !strings.EqualFold(process.FitPolicy(), "spill")
}

func sumCpuMB(processes []*Process) (uint64, bool) {
	var total uint64
	for _, process := range processes {
		if !shouldAccountHostRam(process) {
			continue
		}
		used := process.MeasuredCpuMB()
		if used == 0 {
			return 0, false
		}
		total += used
	}
	return total, true
}
