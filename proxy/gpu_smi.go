package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type NvidiaSMIAllocator struct{}

func (a NvidiaSMIAllocator) GetGPUs() ([]GPUInfo, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.free,memory.total", "--format=csv,nounits,noheader")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nvidia-smi unavailable: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	gpus := make([]GPUInfo, 0, len(lines))
	for index, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected nvidia-smi output: %q", line)
		}
		freeMB, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid free memory %q: %w", parts[0], err)
		}
		totalMB, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid total memory %q: %w", parts[1], err)
		}
		gpus = append(gpus, GPUInfo{
			Index:   index,
			FreeMB:  freeMB,
			TotalMB: totalMB,
		})
	}
	return gpus, nil
}
