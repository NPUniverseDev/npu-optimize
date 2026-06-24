package runtime

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/Ericson246/npu-optimize/internal/hwinfo"
)

type Backend int

const (
	BackendUnknown Backend = iota
	BackendCUDA
	BackendROCm
	BackendOpenVINO
	BackendVulkanDiscrete
	BackendVulkanIntegrated
	BackendOpenVINONPU
	BackendCPU
)

func backendFromString(s string) Backend {
	switch strings.ToLower(s) {
	case "cuda":
		return BackendCUDA
	case "rocm":
		return BackendROCm
	case "openvino":
		return BackendOpenVINO
	case "vulkan":
		return BackendVulkanDiscrete
	case "vulkan-integrated":
		return BackendVulkanIntegrated
	case "vulkan-npu":
		return BackendOpenVINONPU
	case "cpu":
		return BackendCPU
	default:
		return BackendUnknown
	}
}

func Select(hw *hwinfo.Info, prefer string, catalog *Catalog) (*RuntimeEntry, error) {
	platform := runtime.GOOS
	arch := runtime.GOARCH

	archStr := "x64"
	if strings.Contains(arch, "arm64") || strings.Contains(arch, "aarch64") {
		archStr = "arm64"
	}

	backends := priorityBackends(hw, prefer)

	for _, b := range backends {
		entry := findRuntime(catalog, platform, archStr, b, hw)
		if entry != nil {
			return entry, nil
		}
	}

	for _, src := range catalog.Sources {
		for _, id := range sortedKeys(src.Runtimes) {
			entry := src.Runtimes[id]
			if entry.Platform == platform && entry.Arch == archStr {
				entry.ID = id
				return &entry, nil
			}
		}
	}

	return nil, fmt.Errorf("no compatible runtime found for %s/%s", platform, arch)
}

func priorityBackends(hw *hwinfo.Info, prefer string) []Backend {
	if prefer != "" {
		b := backendFromString(prefer)
		if b != BackendUnknown {
			return []Backend{b}
		}
	}

	available := map[Backend]bool{}
	if hw.GPU != nil {
		for _, b := range hw.GPU.Backends {
			switch b.Name {
			case "cuda":
				available[BackendCUDA] = true
			case "rocm":
				available[BackendROCm] = true
			case "openvino":
				available[BackendOpenVINO] = true
				available[BackendOpenVINONPU] = true
			case "vulkan":
				if hw.GPU.Integrated {
					available[BackendVulkanIntegrated] = true
				} else {
					available[BackendVulkanDiscrete] = true
				}
			}
		}
	}
	available[BackendCPU] = true

	priority := []Backend{
		BackendCUDA,
		BackendROCm,
		BackendOpenVINO,
		BackendVulkanDiscrete,
		BackendVulkanIntegrated,
		BackendOpenVINONPU,
		BackendCPU,
	}

	var result []Backend
	for _, b := range priority {
		if available[b] {
			result = append(result, b)
		}
	}

	if len(result) == 0 {
		result = append(result, BackendCPU)
	}

	return result
}

func findRuntime(catalog *Catalog, platform, arch string, backend Backend, hw *hwinfo.Info) *RuntimeEntry {
	backendStr := backendString(backend)
	if backendStr == "" {
		return nil
	}

	var detectedLib string
	if hw != nil && hw.GPU != nil {
		for _, b := range hw.GPU.Backends {
			if b.Name == backendStr {
				detectedLib = b.DetectedLib
				break
			}
		}
	}

	for _, src := range catalog.Sources {
		for _, id := range sortedKeys(src.Runtimes) {
			entry := src.Runtimes[id]
			if entry.Platform != platform || entry.Arch != arch {
				continue
			}
			if !strings.HasPrefix(entry.Backend, backendStr) && !strings.Contains(entry.ID, backendStr) {
				continue
			}
			if detectedLib != "" && len(entry.RequiresLib) > 0 {
				for _, req := range entry.RequiresLib {
					if req == detectedLib {
						entry.ID = id
						return &entry
					}
				}
				continue
			}
			entry.ID = id
			return &entry
		}
	}

	return nil
}

func sortedKeys(m map[string]RuntimeEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func backendString(b Backend) string {
	switch b {
	case BackendCUDA:
		return "cuda"
	case BackendROCm:
		return "rocm"
	case BackendOpenVINO:
		return "openvino"
	case BackendVulkanDiscrete:
		return "vulkan"
	case BackendVulkanIntegrated:
		return "vulkan"
	case BackendOpenVINONPU:
		return "openvino"
	case BackendCPU:
		return "cpu"
	default:
		return ""
	}
}
