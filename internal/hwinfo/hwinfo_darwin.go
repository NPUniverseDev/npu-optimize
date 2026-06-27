package hwinfo

import (
	"encoding/json"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

func detect() (*Info, error) {
	info := &Info{}
	detectCPU(info)
	detectRAM(info)
	detectGPU(info)
	detectBackends(info)
	return info, nil
}

func detectCPU(info *Info) {
	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		info.CPU.Name = cpuInfo[0].ModelName
	}
	info.CPU.Cores, _ = cpu.Counts(false)
	info.CPU.Threads, _ = cpu.Counts(true)
}

func detectRAM(info *Info) {
	vmem, err := mem.VirtualMemory()
	if err == nil {
		info.RAMTotalMB = int64(vmem.Total / 1024 / 1024)
		info.RAMFreeMB = int64(vmem.Available / 1024 / 1024)
	}
}

func detectBackends(info *Info) {
	backends := []BackendInfo{
		{Name: "metal", DetectedLib: "Metal.framework"},
	}

	if lib, _, ok := detectVulkanRuntime(); ok {
		backends = append(backends, BackendInfo{Name: "vulkan", DetectedLib: lib})
	}

	if info.GPU != nil {
		info.GPU.Backends = backends
	}

	slog.Debug("detected backends", "backends", backends)
}

func detectVulkanRuntime() (lib, version string, ok bool) {
	cmd := exec.Command("vulkaninfo", "--summary")
	if cmd.Run() != nil {
		return "", "", false
	}
	return "vulkaninfo", "", true
}

type darwinDisplay struct {
	Name   string `json:"_name"`
	Model  string `json:"sppci_model"`
	VRAM   string `json:"spdisplays_vram"`
	Vendor string `json:"spdisplays_vendor"`
}

type darwinSPData struct {
	Displays []darwinDisplay `json:"SPDisplaysDataType"`
}

func parseVRAMString(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		return 0
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	unit := strings.TrimSpace(parts[1])
	switch unit {
	case "GB", "G":
		return int64(val * 1024)
	case "MB", "M":
		return int64(val)
	case "KB", "K":
		return int64(val / 1024)
	default:
		return 0
	}
}

func detectVulkanGPUFallback(info *Info) bool {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("vulkan fallback system_profiler failed", "err", err)
		return false
	}

	var sp darwinSPData
	if err := json.Unmarshal(out, &sp); err != nil {
		slog.Debug("vulkan fallback: failed to parse system_profiler output", "err", err)
		return false
	}

	if len(sp.Displays) == 0 {
		return false
	}

	display := sp.Displays[0]
	name := display.Name
	if name == "" {
		name = display.Model
	}
	if name == "" {
		return false
	}

	vendor := display.Vendor
	if vendor == "" {
		vendor = "apple"
	}

	integrated := vendor == "apple"
	if !integrated {
		integrated = strings.Contains(strings.ToLower(vendor), "intel")
	}

	vramMB := parseVRAMString(display.VRAM)
	if vramMB <= 0 {
		if integrated {
			vramMB = info.RAMTotalMB
		} else {
			vramMB = info.RAMTotalMB / 2
		}
	}

	vramFreeMB := vramMB
	if integrated && info.RAMFreeMB < vramMB {
		vramFreeMB = info.RAMFreeMB
	}

	info.GPU = &GPUInfo{
		Vendor:      vendor,
		Name:        name,
		VRAMTotalMB: vramMB,
		VRAMFreeMB:  vramFreeMB,
		Integrated:  integrated,
	}

	slog.Debug("vulkan GPU detected via system_profiler fallback (vulkaninfo not found)",
		"vendor", vendor, "gpu", name,
		"vram_mb", vramMB,
	)
	return true
}
