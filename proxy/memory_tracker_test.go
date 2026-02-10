package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMemoryFromLog(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		expected  MemoryFootprint
		expectHit bool
	}{
		{
			name:      "json vram/cpu used mb",
			line:      `{"vram_used_mb":12345,"cpu_used_mb":67890}`,
			expected:  MemoryFootprint{VramMB: 12345, CpuMB: 67890},
			expectHit: true,
		},
		{
			name:      "json gpu/ram mb",
			line:      `{"gpu_mb":2048,"ram_mb":4096}`,
			expected:  MemoryFootprint{VramMB: 2048, CpuMB: 4096},
			expectHit: true,
		},
		{
			name:      "plain text gib",
			line:      "VRAM used: 12.5 GiB CPU used: 64 GiB",
			expected:  MemoryFootprint{VramMB: 12800, CpuMB: 65536},
			expectHit: true,
		},
		{
			name:      "plain text mib",
			line:      "GPU memory: 8000 MiB RAM used: 16000 MiB",
			expected:  MemoryFootprint{VramMB: 8000, CpuMB: 16000},
			expectHit: true,
		},
		{
			name:      "empty line",
			line:      "",
			expectHit: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			footprint, ok := parseMemoryFromLog(test.line)
			assert.Equal(t, test.expectHit, ok)
			if ok {
				assert.Equal(t, test.expected.VramMB, footprint.VramMB)
				assert.Equal(t, test.expected.CpuMB, footprint.CpuMB)
			}
		})
	}
}
