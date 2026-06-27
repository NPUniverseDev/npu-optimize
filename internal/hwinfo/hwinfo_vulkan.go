package hwinfo

import (
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

func detectGPU(info *Info) {
	if detectNvidiaGPU(info) {
		return
	}
	detectVulkanGPU(info)
}

func detectNvidiaGPU(info *Info) bool {
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total,memory.free",
		"--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		return false
	}
	parts := strings.Split(line, ", ")
	if len(parts) < 3 {
		return false
	}

	info.GPU = &GPUInfo{
		Vendor: "nvidia",
		Name:   parts[0],
	}

	if total, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
		info.GPU.VRAMTotalMB = total
	}
	if free, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
		info.GPU.VRAMFreeMB = free
	}
	return true
}

func detectVulkanGPU(info *Info) bool {
	if detectVulkanGPUWithVulkaninfo(info) {
		return true
	}

	slog.Debug("vulkaninfo not available, trying platform fallback")
	return detectVulkanGPUFallback(info)
}

func detectVulkanGPUWithVulkaninfo(info *Info) bool {
	cmd := exec.Command("vulkaninfo", "--summary")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	output := string(out)
	name, vendor := parseVulkanGPU(output)
	if name == "" {
		return false
	}

	vramTotalMB := parseVulkanVRAM(output)
	isIntegrated := isVulkanIntegrated(output)

	if vramTotalMB <= 0 {
		vramTotalMB = info.RAMTotalMB / 2
		isIntegrated = true
		slog.Debug("vulkan GPU detected; VRAM estimated from RAM",
			"vendor", vendor, "gpu", name,
			"estimated_mb", vramTotalMB,
		)
	} else {
		slog.Debug("vulkan GPU detected",
			"vendor", vendor, "gpu", name,
			"vram_mb", vramTotalMB,
			"integrated", isIntegrated,
		)
	}

	vramFreeMB := vramTotalMB
	if isIntegrated && info.RAMFreeMB < vramTotalMB {
		vramFreeMB = info.RAMFreeMB
	}

	info.GPU = &GPUInfo{
		Vendor:      vendor,
		Name:        name,
		VRAMTotalMB: vramTotalMB,
		VRAMFreeMB:  vramFreeMB,
		Integrated:  isIntegrated,
	}
	return true
}

func parseVulkanVRAM(output string) int64 {
	lines := strings.Split(output, "\n")
	inHeaps := false
	var totalVRAM int64
	var currentSize int64
	currentDeviceLocal := false

	flushHeap := func() {
		if currentDeviceLocal {
			totalVRAM += currentSize
		}
		currentSize = 0
		currentDeviceLocal = false
	}

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		lower := strings.ToLower(trimmed)

		if (strings.Contains(lower, "memory heaps") || strings.Contains(lower, "memoryheaps") ||
			strings.Contains(lower, "memoryheapcount")) && !inHeaps {
			inHeaps = true
			continue
		}
		if !inHeaps {
			continue
		}
		if strings.Contains(lower, "memory types") || strings.Contains(lower, "queue families") {
			break
		}
		if strings.HasPrefix(lower, "gpu") && strings.Contains(lower, ":") && !strings.HasPrefix(lower, "gpu name") {
			break
		}

		if strings.Contains(lower, "memoryheaps[") || strings.Contains(lower, "memory heaps[") {
			flushHeap()
			continue
		}

		if strings.HasPrefix(lower, "size") && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				sizeStr := strings.TrimSpace(parts[1])
				if idx := strings.Index(sizeStr, "("); idx > 0 {
					sizeStr = strings.TrimSpace(sizeStr[:idx])
				}
				if s, err := strconv.ParseInt(sizeStr, 10, 64); err == nil && s > 0 {
					currentSize = s
				}
			}
		}

		if strings.Contains(trimmed, "MEMORY_HEAP_DEVICE_LOCAL_BIT") {
			currentDeviceLocal = true
		}
	}

	flushHeap()

	return totalVRAM / 1024 / 1024
}

func isVulkanIntegrated(output string) bool {
	lower := strings.ToLower(output)

	if strings.Contains(lower, "integrated_gpu") {
		return true
	}
	if strings.Contains(lower, "discrete_gpu") {
		return false
	}

	vramMB := parseVulkanVRAM(output)
	return vramMB <= 0
}

func parseVulkanGPU(output string) (name, vendor string) {
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "device name") || strings.Contains(lower, "gpu name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name = strings.TrimSpace(parts[1])
			}
			parts = strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				name = strings.TrimSpace(parts[1])
			}
		}
	}
	if name == "" {
		return "", ""
	}
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "nvidia"):
		vendor = "nvidia"
	case strings.Contains(lower, "advanced micro devices") || strings.Contains(lower, "amd"):
		vendor = "amd"
	case strings.Contains(lower, "intel"):
		vendor = "intel"
	case strings.Contains(lower, "apple"):
		vendor = "apple"
	default:
		vendor = "unknown"
	}
	return name, vendor
}
