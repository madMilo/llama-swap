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
	warnMu    sync.Mutex

	gpuVramCapMB  uint64
	gpuVramCapsMB []uint64
	hostRamCapMB  uint64

	missingHostRAMWarned map[string]struct{}
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
		missingHostRAMWarned: make(map[string]struct{}),
	}
}

func (s *Scheduler) ScheduleProcess(process *Process) error {
	fitPolicy := strings.ToLower(process.FitPolicy())
	s.logger.Infof("<%s> scheduling decision start: fit_policy=%s", process.ID, fitPolicy)

	if err := s.ensureHostRamCapacity(process); err != nil {
		s.logger.Infof("<%s> scheduling decision: not scheduled (%v)", process.ID, err)
		return err
	}

	if fitPolicy == "spill" {
		gpus, err := s.allocator.GetGPUs()
		if err != nil {
			s.logger.Infof("<%s> scheduling decision: not scheduled (unable to inspect GPUs: %v)", process.ID, err)
			return err
		}
		gpus = s.applyVramCaps(gpus)
		if len(gpus) == 0 {
			err := fmt.Errorf("no GPUs detected for scheduling")
			s.logger.Infof("<%s> scheduling decision: not scheduled (%v)", process.ID, err)
			return err
		}

		visible := make([]string, 0, len(gpus))
		for _, gpu := range gpus {
			visible = append(visible, fmt.Sprintf("%d", gpu.Index))
		}
		process.SetRuntimeEnv([]string{fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", strings.Join(visible, ","))})
		s.logger.Infof("<%s> scheduling decision: scheduled with fit_policy=spill visible_gpus=%s", process.ID, strings.Join(visible, ","))
		return nil
	}

	if fitPolicy != "evict_to_fit" {
		s.logger.Infof("<%s> scheduling decision: scheduled without GPU placement (fit_policy=%s)", process.ID, fitPolicy)
		return nil
	}

	requiredMB := process.MeasuredVramMB()

	s.mu.Lock()
	defer s.mu.Unlock()

	gpus, err := s.allocator.GetGPUs()
	if err != nil {
		s.logger.Infof("<%s> scheduling decision: not scheduled (unable to inspect GPUs: %v)", process.ID, err)
		return err
	}
	gpus = s.applyVramCaps(gpus)
	if len(gpus) == 0 {
		err := fmt.Errorf("no GPUs detected for scheduling")
		s.logger.Infof("<%s> scheduling decision: not scheduled (%v)", process.ID, err)
		return err
	}

	if requiredMB == 0 {
		s.logger.Warnf("<%s> missing VRAM footprint; selecting GPU with most free memory", process.ID)
		chosen := gpus[0]
		for _, gpu := range gpus[1:] {
			if gpu.FreeMB > chosen.FreeMB {
				chosen = gpu
			}
		}
		process.SetAssignedGPU(chosen.Index)
		process.SetRuntimeEnv([]string{fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", chosen.Index)})
		s.logger.Infof("<%s> scheduling decision: scheduled on GPU %d (missing VRAM footprint)", process.ID, chosen.Index)
		return nil
	}

	type candidate struct {
		gpuIndex int
		evict    []*Process
		freeMB   uint64
		assigned int
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
			assigned: len(assigned),
		})
	}

	if len(candidates) == 0 {
		s.logger.Infof("<%s> scheduling decision: not scheduled (%v required_vram_mb=%d)", process.ID, ErrInsufficientVRAM, requiredMB)
		return ErrInsufficientVRAM
	}

	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].evict) != len(candidates[j].evict) {
			return len(candidates[i].evict) < len(candidates[j].evict)
		}
		if candidates[i].assigned != candidates[j].assigned {
			return candidates[i].assigned < candidates[j].assigned
		}
		if candidates[i].freeMB != candidates[j].freeMB {
			return candidates[i].freeMB > candidates[j].freeMB
		}
		return candidates[i].gpuIndex < candidates[j].gpuIndex
	})

	chosen := candidates[0]
	for _, evicted := range chosen.evict {
		evicted.StopImmediately()
	}

	process.SetAssignedGPU(chosen.gpuIndex)
	process.SetRuntimeEnv([]string{fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", chosen.gpuIndex)})
	s.logger.Infof("<%s> scheduling decision: scheduled on GPU %d evicted=%d required_vram_mb=%d", process.ID, chosen.gpuIndex, len(chosen.evict), requiredMB)

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
		if process.AssignedGPU() == gpuIndex && processUsesSchedulerCapacity(process) {
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
		if s.shouldWarnMissingHostRAM(process.ID) {
			s.logger.Warnf("<%s> missing host RAM footprint; skipping host RAM cap check", process.ID)
		}
		s.logger.Infof("<%s> host RAM scheduling decision: allow (missing host RAM footprint)", process.ID)
		return nil
	}

	running := s.provider()
	total, ok := sumCpuMB(running)
	if !ok {
		s.logger.Warnf("unable to fully account host RAM usage for running models; skipping host RAM cap check")
		s.logger.Infof("<%s> host RAM scheduling decision: allow (running host RAM usage incomplete)", process.ID)
		return nil
	}

	if total+requiredMB > s.hostRamCapMB {
		s.logger.Infof("<%s> host RAM scheduling decision: deny used_mb=%d required_mb=%d cap_mb=%d", process.ID, total, requiredMB, s.hostRamCapMB)
		return ErrInsufficientHostRAM
	}
	s.logger.Infof("<%s> host RAM scheduling decision: allow used_mb=%d required_mb=%d cap_mb=%d", process.ID, total, requiredMB, s.hostRamCapMB)
	return nil
}

func (s *Scheduler) shouldWarnMissingHostRAM(processID string) bool {
	s.warnMu.Lock()
	defer s.warnMu.Unlock()
	if _, ok := s.missingHostRAMWarned[processID]; ok {
		return false
	}
	s.missingHostRAMWarned[processID] = struct{}{}
	return true
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
