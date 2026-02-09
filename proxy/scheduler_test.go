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
	require.Equal(t, 0, allocator.calls)
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
			require.ErrorIs(t, err, ErrUnknownFootprint)
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

func TestSchedulerScheduleProcess_GroupExclusive(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{gpus: []GPUInfo{{Index: 0, FreeMB: 100, TotalMB: 1000}}}
	p1 := newTestProcess(t, "p1", "evict_to_fit", 200, 100, tracker)
	p2 := newTestProcess(t, "p2", "evict_to_fit", 200, 100, tracker)
	readyOnGPU(p1, 0)
	readyOnGPU(p2, 0)

	candidate := newTestProcess(t, "candidate", "evict_to_fit", 400, 100, tracker)
	candidate.GroupExclusive = true

	p2.inFlightRequestsCount.Add(1)
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return []*Process{p1, p2} }, SchedulerOptions{})
	err := scheduler.ScheduleProcess(candidate)
	require.ErrorIs(t, err, ErrInsufficientVRAM)

	p2.inFlightRequestsCount.Add(-1)
	err = scheduler.ScheduleProcess(candidate)
	require.NoError(t, err)
	require.Equal(t, 0, candidate.AssignedGPU())
}

func TestSchedulerSelectEvictions(t *testing.T) {
	tracker := NewMemoryTracker()
	allocator := &fakeGPUAllocator{}
	scheduler := NewScheduler(allocator, testLogger, func() []*Process { return nil }, SchedulerOptions{})

	unknown := newTestProcess(t, "unknown", "evict_to_fit", 0, 0, tracker)
	unknown.forceState(StateReady)
	_, ok := scheduler.selectEvictions(newTestProcess(t, "candidate", "evict_to_fit", 100, 0, tracker), []*Process{unknown}, 0, 100)
	require.False(t, ok)

	exclusive := newTestProcess(t, "exclusive", "evict_to_fit", 100, 0, tracker)
	exclusive.GroupExclusive = true
	readyOnGPU(exclusive, 0)
	evict, ok := scheduler.selectEvictions(newTestProcess(t, "candidate2", "evict_to_fit", 150, 0, tracker), []*Process{exclusive}, 75, 150)
	require.True(t, ok)
	require.Len(t, evict, 1)

	busy := newTestProcess(t, "busy", "evict_to_fit", 100, 0, tracker)
	readyOnGPU(busy, 0)
	busy.inFlightRequestsCount.Add(1)
	_, ok = scheduler.selectEvictions(newTestProcess(t, "candidate2b", "evict_to_fit", 150, 0, tracker), []*Process{busy}, 75, 150)
	require.False(t, ok)
	busy.inFlightRequestsCount.Add(-1)

	free := newTestProcess(t, "free", "evict_to_fit", 100, 0, tracker)
	readyOnGPU(free, 0)
	evict, ok = scheduler.selectEvictions(newTestProcess(t, "candidate3", "evict_to_fit", 50, 0, tracker), []*Process{free}, 200, 50)
	require.True(t, ok)
	require.Empty(t, evict)

	older := newTestProcess(t, "older", "evict_to_fit", 30, 0, tracker)
	newer := newTestProcess(t, "newer", "evict_to_fit", 30, 0, tracker)
	readyOnGPU(older, 0)
	readyOnGPU(newer, 0)
	older.setLastRequestHandled(time.Now().Add(-2 * time.Hour))
	newer.setLastRequestHandled(time.Now().Add(-1 * time.Hour))
	evict, ok = scheduler.selectEvictions(newTestProcess(t, "candidate4", "evict_to_fit", 60, 0, tracker), []*Process{newer, older}, 10, 60)
	require.True(t, ok)
	require.Len(t, evict, 2)
	require.Equal(t, older, evict[0])
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
	require.ErrorIs(t, err, ErrUnknownFootprint)

	unknown := newTestProcess(t, "unknown", "default", 0, 0, tracker)
	err = scheduler.ensureHostRamCapacity(unknown)
	require.ErrorIs(t, err, ErrUnknownFootprint)
}
