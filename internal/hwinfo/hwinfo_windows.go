package hwinfo

import (
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"

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

type cudaLib struct{ name, version string }

var cudaLibs = []cudaLib{
	{"cudart64_12.dll", "12"},
	{"cudart64_13.dll", "13"},
	{"cudart64_11.dll", "11"},
}

type rocmLib struct{ name, version string }

var rocmLibs = []rocmLib{
	{"amdhip64_7.dll", "7"},
	{"amdhip64_6.dll", "6"},
}

func detectBackends(info *Info) {
	backends := []BackendInfo{}

	if lib, ver, ok := detectCUDARuntime(); ok {
		backends = append(backends, BackendInfo{Name: "cuda", Version: ver, DetectedLib: lib})
	}
	if lib, ver, ok := detectROCmRuntime(); ok && info.GPU != nil && info.GPU.Vendor == "amd" {
		backends = append(backends, BackendInfo{Name: "rocm", Version: ver, DetectedLib: lib})
	}
	if lib, _, ok := detectOpenVINORuntime(); ok {
		backends = append(backends, BackendInfo{Name: "openvino", DetectedLib: lib})
	}
	if lib, _, ok := detectVulkanRuntime(); ok {
		backends = append(backends, BackendInfo{Name: "vulkan", DetectedLib: lib})
	}

	if len(backends) == 0 {
		backends = append(backends, BackendInfo{Name: "cpu"})
	}

	if info.GPU != nil {
		info.GPU.Backends = backends
	}

	slog.Debug("detected backends", "backends", backends)
}

func detectCUDARuntime() (lib, version string, ok bool) {
	for _, dll := range cudaLibs {
		h, err := syscall.LoadLibrary(dll.name)
		if err == nil {
			_ = syscall.FreeLibrary(h)
			return dll.name, dll.version, true
		}
	}
	return "", "", false
}

func detectROCmRuntime() (lib, version string, ok bool) {
	for _, dll := range rocmLibs {
		h, err := syscall.LoadLibrary(dll.name)
		if err == nil {
			_ = syscall.FreeLibrary(h)
			return dll.name, dll.version, true
		}
	}
	return "", "", false
}

func detectOpenVINORuntime() (lib, version string, ok bool) {
	h, err := syscall.LoadLibrary("openvino.dll")
	if err != nil {
		return "", "", false
	}
	_ = syscall.FreeLibrary(h)
	return "openvino.dll", "", true
}

func detectVulkanRuntime() (lib, version string, ok bool) {
	h, err := syscall.LoadLibrary("vulkan-1.dll")
	if err != nil {
		return "", "", false
	}
	_ = syscall.FreeLibrary(h)
	return "vulkan-1.dll", "", true
}

func detectVulkanGPUFallback(info *Info) bool {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_VideoController | Select-Object Name, AdapterRAM | ConvertTo-Json -Compress")
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("vulkan fallback WMI command failed", "err", err)
		return false
	}

	gpu, ok := parseWMIJSON(out)
	if !ok {
		return false
	}

	gpu.VRAMFreeMB = gpu.VRAMTotalMB
	if gpu.Integrated && info.RAMFreeMB < gpu.VRAMTotalMB {
		gpu.VRAMFreeMB = info.RAMFreeMB
	}

	if gpu.VRAMTotalMB <= 0 {
		gpu.VRAMTotalMB = info.RAMTotalMB / 2
		gpu.Integrated = true
		gpu.VRAMFreeMB = info.RAMFreeMB
	}

	info.GPU = gpu
	slog.Debug("vulkan GPU detected via WMI fallback (vulkaninfo not found)",
		"vendor", gpu.Vendor, "gpu", gpu.Name,
		"vram_mb", gpu.VRAMTotalMB,
	)
	return true
}

type wmiGPU struct {
	Name       string `json:"Name"`
	AdapterRAM int64  `json:"AdapterRAM"`
}

func parseWMIJSON(data []byte) (*GPUInfo, bool) {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return nil, false
	}

	var single wmiGPU
	if err := json.Unmarshal(data, &single); err == nil && single.Name != "" {
		return buildFromWMI(single), true
	}

	var multiple []wmiGPU
	if err := json.Unmarshal(data, &multiple); err != nil {
		return nil, false
	}

	for _, g := range multiple {
		if g.Name == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(g.Name), "microsoft basic display") {
			return buildFromWMI(g), true
		}
	}

	for _, g := range multiple {
		if g.Name != "" {
			return buildFromWMI(g), true
		}
	}

	return nil, false
}

func buildFromWMI(g wmiGPU) *GPUInfo {
	lower := strings.ToLower(g.Name)
	vendor := "unknown"
	switch {
	case strings.Contains(lower, "nvidia"):
		vendor = "nvidia"
	case strings.Contains(lower, "advanced micro devices"), strings.Contains(lower, "amd"), strings.Contains(lower, "radeon"):
		vendor = "amd"
	case strings.Contains(lower, "intel"):
		vendor = "intel"
	case strings.Contains(lower, "apple"):
		vendor = "apple"
	}

	integrated := vendor == "intel" || vendor == "apple"

	vramMB := g.AdapterRAM / 1024 / 1024

	return &GPUInfo{
		Vendor:      vendor,
		Name:        g.Name,
		VRAMTotalMB: vramMB,
		Integrated:  integrated,
	}
}
