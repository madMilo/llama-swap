package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MemoryFootprint struct {
	VramMB     uint64
	CpuMB      uint64
	RecordedAt time.Time
}

type MemoryTracker struct {
	mu         sync.RWMutex
	footprints map[string]MemoryFootprint
}

func NewMemoryTracker() *MemoryTracker {
	return &MemoryTracker{
		footprints: make(map[string]MemoryFootprint),
	}
}

func (t *MemoryTracker) Set(signature string, footprint MemoryFootprint) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.footprints[signature] = footprint
}

func (t *MemoryTracker) Get(signature string) (MemoryFootprint, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	footprint, ok := t.footprints[signature]
	return footprint, ok
}

func (t *MemoryTracker) ObserveLog(signature string, line string) (MemoryFootprint, bool) {
	footprint, ok := parseMemoryFromLog(line)
	if !ok {
		return MemoryFootprint{}, false
	}
	if existing, ok := t.Get(signature); ok {
		if footprint.VramMB == 0 {
			footprint.VramMB = existing.VramMB
		}
		if footprint.CpuMB == 0 {
			footprint.CpuMB = existing.CpuMB
		}
	}
	footprint.RecordedAt = time.Now()
	t.Set(signature, footprint)
	return footprint, true
}

var (
	plainVRAMRegex = regexp.MustCompile(`(?i)\b(vram|gpu)\b\s+(used|memory)\s*[:=]\s*([0-9.]+)\s*(mi?b|gi?b)`)
	plainCPURex    = regexp.MustCompile(`(?i)\b(cpu|ram)\b\s+(used|memory)\s*[:=]\s*([0-9.]+)\s*(mi?b|gi?b)`)
	llamaVRAMRegex = regexp.MustCompile(`(?i)\b(cuda|vram|gpu)\b[^\n]*?([0-9]+(?:\.[0-9]+)?)\s*(mi?b|gi?b)`)
	llamaCPURex    = regexp.MustCompile(`(?i)\b(cpu|host|ram)\b[^\n]*?([0-9]+(?:\.[0-9]+)?)\s*(mi?b|gi?b)`)
)

func parseMemoryFromLog(line string) (MemoryFootprint, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return MemoryFootprint{}, false
	}

	if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err == nil {
			vram := findMB(payload, []string{"vram_used_mb", "gpu_used_mb", "vram_mb", "gpu_mb"})
			cpu := findMB(payload, []string{"cpu_used_mb", "ram_used_mb", "cpu_mb", "ram_mb"})
			if vram > 0 || cpu > 0 {
				return MemoryFootprint{VramMB: vram, CpuMB: cpu}, true
			}
		}
	}

	if matches := plainVRAMRegex.FindStringSubmatch(line); len(matches) == 5 {
		if vram, ok := parseSizeToMB(matches[3], matches[4]); ok {
			cpu := uint64(0)
			if cpuMatch := plainCPURex.FindStringSubmatch(line); len(cpuMatch) == 5 {
				if parsed, ok := parseSizeToMB(cpuMatch[3], cpuMatch[4]); ok {
					cpu = parsed
				}
			}
			return MemoryFootprint{VramMB: vram, CpuMB: cpu}, true
		}
	}

	vram := uint64(0)
	cpu := uint64(0)
	if matches := llamaVRAMRegex.FindStringSubmatch(line); len(matches) == 4 {
		if parsed, ok := parseSizeToMB(matches[2], matches[3]); ok {
			vram = parsed
		}
	}
	if matches := llamaCPURex.FindStringSubmatch(line); len(matches) == 4 {
		if parsed, ok := parseSizeToMB(matches[2], matches[3]); ok {
			cpu = parsed
		}
	}
	if vram > 0 || cpu > 0 {
		return MemoryFootprint{VramMB: vram, CpuMB: cpu}, true
	}

	return MemoryFootprint{}, false
}

func findMB(payload map[string]any, keys []string) uint64 {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if parsed, ok := parseAnyToMB(value); ok {
				return parsed
			}
		}
	}
	return 0
}

func parseAnyToMB(value any) (uint64, bool) {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return 0, false
		}
		return uint64(v), true
	case int:
		if v <= 0 {
			return 0, false
		}
		return uint64(v), true
	case int64:
		if v <= 0 {
			return 0, false
		}
		return uint64(v), true
	case string:
		return parseSizeStringToMB(v)
	default:
		return 0, false
	}
}

func parseSizeStringToMB(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	parts := strings.Fields(value)
	if len(parts) == 1 {
		if parsed, err := strconv.ParseFloat(parts[0], 64); err == nil {
			if parsed <= 0 {
				return 0, false
			}
			return uint64(parsed), true
		}
		return 0, false
	}
	if len(parts) >= 2 {
		return parseSizeToMB(parts[0], parts[1])
	}
	return 0, false
}

func parseSizeToMB(value string, unit string) (uint64, bool) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	unit = strings.ToLower(strings.TrimSpace(unit))
	switch unit {
	case "mb", "mib":
		return uint64(parsed), true
	case "gb", "gib":
		return uint64(parsed * 1024), true
	default:
		return 0, false
	}
}

func signatureForModel(modelID string, cmd string) string {
	return fmt.Sprintf("%s|%s", modelID, cmd)
}
