package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeGPUAllocator struct {
	gpus  []GPUInfo
	calls int
	err   error
}

func (f *fakeGPUAllocator) GetGPUs() ([]GPUInfo, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	gpus := make([]GPUInfo, len(f.gpus))
	copy(gpus, f.gpus)
	return gpus, nil
}

type scenarioGPUAllocator struct {
	gpus     []GPUInfo
	provider func() []*Process
	calls    int
	err      error
}

func (s *scenarioGPUAllocator) GetGPUs() ([]GPUInfo, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}

	gpus := make([]GPUInfo, len(s.gpus))
	copy(gpus, s.gpus)

	for i := range gpus {
		used := uint64(0)
		for _, process := range s.provider() {
			if processUsesSchedulerCapacity(process) && process.AssignedGPU() == gpus[i].Index {
				used += process.MeasuredVramMB()
			}
		}
		if used >= gpus[i].TotalMB {
			gpus[i].FreeMB = 0
			continue
		}
		gpus[i].FreeMB = gpus[i].TotalMB - used
	}

	return gpus, nil
}
func newTestProcess(t *testing.T, id, fitPolicy string, vramMB, cpuMB uint64, tracker *MemoryTracker) *Process {
	t.Helper()
	config := getTestSimpleResponderConfig(id)
	config.FitPolicy = fitPolicy
	process := NewProcess(id, 1, config, testLogger, testLogger)
	if tracker != nil {
		signature := id + "-sig"
		process.SetMemoryTracker(tracker, signature)
		tracker.Set(signature, MemoryFootprint{VramMB: vramMB, CpuMB: cpuMB})
	}
	return process
}

func readyOnGPU(process *Process, gpuIndex int) {
	process.SetAssignedGPU(gpuIndex)
	process.forceState(StateReady)
}

func TestSchedulerScheduleProcess_FitPolicyHostRamOnly(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 500, TotalMB: 1000}}}
	running := newTestProcess(t, "running", "default", 0, 900, tracker)
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return []*Process{running} }, SchedulerOptions{HostRamCapMB: 1000})

	process := newTestProcess(t, "candidate", "default", 0, 200, tracker)
	err := scheduler.ScheduleProcess(process)
	require.ErrorIs(t, err, ErrInsufficientHostRAM)
	require.Equal(t, 0, allocator.calls)

	spill := newTestProcess(t, "spill", "spill", 0, 2000, tracker)
	err = scheduler.ScheduleProcess(spill)
	require.NoError(t, err)
	require.Equal(t, 1, allocator.calls)
	require.Equal(t, []string{"CUDA_VISIBLE_DEVICES=0"}, spill.runtimeEnv)
}

func TestSchedulerScheduleProcess_SpillAssignsAllGPUs(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 500, TotalMB: 1000}, {Index: 1, FreeMB: 400, TotalMB: 1000}}}
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return nil }, SchedulerOptions{GpuVramCapsMB: []uint64{300, 300}})

	spill := newTestProcess(t, "spill", "spill", 0, 200, tracker)
	err := scheduler.ScheduleProcess(spill)
	require.NoError(t, err)
	require.Equal(t, 1, allocator.calls)
	require.Equal(t, []string{"CUDA_VISIBLE_DEVICES=0,1"}, spill.runtimeEnv)
}

func TestSchedulerScheduleProcess_UnknownFootprint(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 500, TotalMB: 1000}}}

	cases := []struct {
		name      string
		process   *Process
		hostCapMB uint64
	}{
		{
			name:      "missing host ram measurement",
			process:   newTestProcess(t, "missing-cpu", "evict_to_fit", 100, 0, tracker),
			hostCapMB: 1000,
		},
		{
			name:      "missing vram measurement",
			process:   newTestProcess(t, "missing-vram", "evict_to_fit", 0, 100, tracker),
			hostCapMB: 0,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := NewScheduler(allocator, testLogger, func() []*Process { return nil }, SchedulerOptions{HostRamCapMB: tt.hostCapMB})
			err := scheduler.ScheduleProcess(tt.process)
			if tt.name == "missing host ram measurement" {
				require.NoError(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 0, tt.process.AssignedGPU())
		})
	}
}

func TestSchedulerScheduleProcess_InsufficientVRAM(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 200, TotalMB: 1000}}}
	p1 := newTestProcess(t, "p1", "evict_to_fit", 100, 100, tracker)
	p2 := newTestProcess(t, "p2", "evict_to_fit", 200, 100, tracker)
	readyOnGPU(p1, 0)
	readyOnGPU(p2, 0)

	candidate := newTestProcess(t, "candidate", "evict_to_fit", 600, 100, tracker)

	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return []*Process{p1, p2} }, SchedulerOptions{})
	err := scheduler.ScheduleProcess(candidate)
	require.ErrorIs(t, err, ErrInsufficientVRAM)
}

func TestSchedulerSelectEvictions(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{}
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return nil }, SchedulerOptions{})

	unknown := newTestProcess(t, "unknown", "evict_to_fit", 0, 0, tracker)
	unknown.forceState(StateReady)
	_, ok := scheduler.selectEvictions(newTestProcess(t, "candidate", "evict_to_fit", 100, 0, tracker), []*Process{unknown}, 0, 100)
	require.False(t, ok)

	busy := newTestProcess(t, "busy", "evict_to_fit", 100, 0, tracker)
	readyOnGPU(busy, 0)
	busy.inFlightRequestsCount.Add(1)
	_, ok = scheduler.selectEvictions(newTestProcess(t, "candidate2b", "evict_to_fit", 150, 0, tracker), []*Process{busy}, 75, 150)
	require.False(t, ok)
	busy.inFlightRequestsCount.Add(-1)

	free := newTestProcess(t, "free", "evict_to_fit", 100, 0, tracker)
	readyOnGPU(free, 0)
	evict, ok := scheduler.selectEvictions(newTestProcess(t, "candidate3", "evict_to_fit", 50, 0, tracker), []*Process{free}, 200, 50)
	require.True(t, ok)
	require.Empty(t, evict)

	older := newTestProcess(t, "older", "evict_to_fit", 30, 0, tracker)
	newer := newTestProcess(t, "newer", "evict_to_fit", 30, 0, tracker)
	readyOnGPU(older, 0)
	readyOnGPU(newer, 0)
	older.setLastRequestHandled(time.Now().Add(-2 * time.Hour))
	newer.setLastRequestHandled(time.Now().Add(-1 * time.Hour))
	evict2, ok2 := scheduler.selectEvictions(newTestProcess(t, "candidate4", "evict_to_fit", 60, 0, tracker), []*Process{newer, older}, 10, 60)
	require.True(t, ok2)
	require.Len(t, evict2, 2)
	require.Equal(t, older, evict2[0])
}

func TestSchedulerApplyVramCaps(t *testing.T) {
	allocator := &fakeGPUAllocator{}
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return nil }, SchedulerOptions{
		GpuVramCapMB:  800,
		GpuVramCapsMB: []uint64{600},
	})

	gpus := scheduler.applyVramCaps([]GPUInfo{
		{Index: 0, FreeMB: 900, TotalMB: 1000},
		{Index: 1, FreeMB: 900, TotalMB: 700},
	})

	require.Equal(t, uint64(600), gpus[0].TotalMB)
	require.Equal(t, uint64(600), gpus[0].FreeMB)
	require.Equal(t, uint64(700), gpus[1].TotalMB)
	require.Equal(t, uint64(700), gpus[1].FreeMB)
}

func TestSchedulerEnsureHostRamCapacity(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{}

	running := newTestProcess(t, "running", "default", 0, 400, tracker)
	spill := newTestProcess(t, "spill", "spill", 0, 9000, tracker)

	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return []*Process{running, spill} }, SchedulerOptions{HostRamCapMB: 1000})
	candidate := newTestProcess(t, "candidate", "default", 0, 500, tracker)
	err := scheduler.ensureHostRamCapacity(candidate)
	require.NoError(t, err)

	candidate = newTestProcess(t, "candidate2", "default", 0, 700, tracker)
	err = scheduler.ensureHostRamCapacity(candidate)
	require.ErrorIs(t, err, ErrInsufficientHostRAM)

	unknownRunning := newTestProcess(t, "unknown-running", "default", 0, 0, tracker)
	scheduler = NewScheduler(allocator, testLogger, func() []*Process { return []*Process{unknownRunning} }, SchedulerOptions{HostRamCapMB: 1000})
	candidate = newTestProcess(t, "candidate3", "default", 0, 100, tracker)
	err = scheduler.ensureHostRamCapacity(candidate)
	require.NoError(t, err)

	unknown := newTestProcess(t, "unknown", "default", 0, 0, tracker)
	err = scheduler.ensureHostRamCapacity(unknown)
	require.NoError(t, err)
}

func TestSchedulerScheduleProcess_CpuMoeWithVramHint(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 1000, TotalMB: 2000}}}
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return nil }, SchedulerOptions{})

	// Test 1: cpu_moe with VRAM hint gets GPU assignment
	cpuMoeWithVram := newTestProcess(t, "cpu-moe-vram", "cpu_moe", 500, 100, tracker)
	err := scheduler.ScheduleProcess(cpuMoeWithVram)
	require.NoError(t, err)
	require.Equal(t, 0, cpuMoeWithVram.AssignedGPU())
	require.Equal(t, []string{"CUDA_VISIBLE_DEVICES=0"}, cpuMoeWithVram.runtimeEnv)

	// Test 2: cpu_moe without VRAM hint stays CPU-only
	cpuMoeWithoutVram := newTestProcess(t, "cpu-moe-no-vram", "cpu_moe", 0, 100, tracker)
	err = scheduler.ScheduleProcess(cpuMoeWithoutVram)
	require.NoError(t, err)
	require.Equal(t, -1, cpuMoeWithoutVram.AssignedGPU())
	require.Nil(t, cpuMoeWithoutVram.runtimeEnv)
}

func TestSchedulerScheduleProcess_CpuMoeEviction(t *testing.T) {
	tracker := NewMemoryTracker()

	existing := newTestProcess(t, "existing", "evict_to_fit", 1200, 100, tracker)
	readyOnGPU(existing, 0)

	allocator := &scenarioGPUAllocator{
		gpus:     []GPUInfo{{Index: 0, FreeMB: 1000, TotalMB: 2000}},
		provider: func() []*Process { return []*Process{existing} },
	}
	scheduler := NewScheduler(allocator, testLogger, allocator.provider, SchedulerOptions{})

	cpuMoe := newTestProcess(t, "cpu-moe-evict", "cpu_moe", 1500, 100, tracker)
	err := scheduler.ScheduleProcess(cpuMoe)
	require.NoError(t, err)
	require.Equal(t, 0, cpuMoe.AssignedGPU())
	require.Equal(t, StateStopped, existing.CurrentState()) // Verify eviction occurred
}

func TestSchedulerScheduleProcess_CpuMoeHostRamAccounting(t *testing.T) {
	tracker := NewMemoryTracker()

	running := newTestProcess(t, "running", "cpu_moe", 200, 600, tracker)
	readyOnGPU(running, 0)

	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 1000, TotalMB: 2000}}}
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return []*Process{running} }, SchedulerOptions{HostRamCapMB: 1000})

	candidate := newTestProcess(t, "candidate", "cpu_moe", 300, 500, tracker)
	err := scheduler.ScheduleProcess(candidate)
	require.ErrorIs(t, err, ErrInsufficientHostRAM)
}

func TestSchedulerScheduleProcess_ScenarioSmallToLargeTwiceTracksExpectedResidentSet(t *testing.T) {
	tracker := NewMemoryTracker()

	type modelShape struct {
		id     string
		vramMB uint64
		cpuMB  uint64
	}

	// Scenario progression expected by the dual-3090 capacity tuning discussion:
	// first three models fit concurrently, then residency slides as 3&4, 4&5, 5&6.
	shapes := []modelShape{
		{id: "glm-flash-q4", vramMB: 1000, cpuMB: 0},
		{id: "glm-flash-q8", vramMB: 5000, cpuMB: 0},
		{id: "qwen-30b-gpu1", vramMB: 9000, cpuMB: 0},
		{id: "qwen-next", vramMB: 11000, cpuMB: 0},
		{id: "glm-flash-q8-dual", vramMB: 11500, cpuMB: 0},
		{id: "qwen-next-dual", vramMB: 12000, cpuMB: 0},
	}

	models := make([]*Process, 0, len(shapes))
	for _, shape := range shapes {
		models = append(models, newTestProcess(t, shape.id, "evict_to_fit", shape.vramMB, shape.cpuMB, tracker))
	}

	allocator := &scenarioGPUAllocator{
		gpus: []GPUInfo{{Index: 0, TotalMB: 24576}},
		provider: func() []*Process {
			return models
		},
	}

	scheduler := NewScheduler(allocator, testLogger, func() []*Process {
		return models
	}, SchedulerOptions{HostRamCapMB: 0})

	expectedResidentByStep := [][]string{
		{"glm-flash-q4"},
		{"glm-flash-q4", "glm-flash-q8"},
		{"glm-flash-q4", "glm-flash-q8", "qwen-30b-gpu1"},
		{"qwen-30b-gpu1", "qwen-next"},
		{"qwen-next", "glm-flash-q8-dual"},
		{"glm-flash-q8-dual", "qwen-next-dual"},
	}

	for pass := 0; pass < 2; pass++ {
		// Reset all processes before each pass (except first)
		if pass > 0 {
			for _, model := range models {
				if model.CurrentState() == StateReady {
					model.StopImmediately()
				}
			}
		}

		for idx, model := range models {
			err := scheduler.ScheduleProcess(model)
			require.NoErrorf(t, err, "pass=%d model=%s", pass+1, model.ID)
			readyOnGPU(model, model.AssignedGPU())

			var resident []string
			for _, process := range models {
				if process.CurrentState() == StateReady && process.AssignedGPU() == 0 {
					resident = append(resident, process.ID)
				}
			}

			require.Equalf(t, expectedResidentByStep[idx], resident, "pass=%d step=%d", pass+1, idx+1)
		}
	}
}

func TestSchedulerScheduleProcess_StartingProcessOccupiesGPU(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 13000, TotalMB: 24576}, {Index: 1, FreeMB: 16000, TotalMB: 24576}}}

	starting := newTestProcess(t, "starting", "evict_to_fit", 0, 100, tracker)
	starting.SetAssignedGPU(1)
	starting.forceState(StateStarting)

	candidate := newTestProcess(t, "candidate", "evict_to_fit", 12000, 100, tracker)

	scheduler := NewScheduler(allocator, testLogger, func() []*Process {
		return []*Process{starting}
	}, SchedulerOptions{})

	err := scheduler.ScheduleProcess(candidate)
	require.NoError(t, err)
	require.Equal(t, 0, candidate.AssignedGPU())
}

func TestSchedulerScheduleProcess_PrefersGPUWithoutEviction(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{
		{Index: 0, FreeMB: 9000, TotalMB: 24576},
		{Index: 1, FreeMB: 11000, TotalMB: 24576},
	}}

	running := newTestProcess(t, "running", "evict_to_fit", 4000, 0, tracker)
	readyOnGPU(running, 1)

	candidate := newTestProcess(t, "candidate", "evict_to_fit", 10000, 0, tracker)

	scheduler := NewScheduler(allocator, testLogger, func() []*Process {
		return []*Process{running}
	}, SchedulerOptions{})

	err := scheduler.ScheduleProcess(candidate)
	require.NoError(t, err)
	require.Equal(t, 1, candidate.AssignedGPU())
}

func TestSchedulerShouldWarnMissingHostRAM_OncePerProcess(t *testing.T) {
	scheduler := NewScheduler(&fakeGPUAllocator{}, testLogger, func() []*Process { return nil }, SchedulerOptions{})

	require.True(t, scheduler.shouldWarnMissingHostRAM("qwen-30b"))
	require.False(t, scheduler.shouldWarnMissingHostRAM("qwen-30b"))
	require.True(t, scheduler.shouldWarnMissingHostRAM("glm-flash-q4"))
}
