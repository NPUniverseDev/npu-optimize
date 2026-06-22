package hwinfo

import (
	"log/slog"
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

func detectBackends(info *Info) {
	backends := []string{}

	if hasCUDARuntime() {
		backends = append(backends, "cuda")
	}
	if hasROCmRuntime() && info.GPU != nil && info.GPU.Vendor == "amd" {
		backends = append(backends, "rocm")
	}
	if hasOpenVINORuntime() {
		backends = append(backends, "openvino")
	}
	if hasVulkanRuntime() {
		backends = append(backends, "vulkan")
	}

	if len(backends) == 0 {
		backends = append(backends, "cpu")
	}

	if info.GPU != nil {
		info.GPU.Backends = backends
	}

	slog.Debug("detected backends", "backends", backends)
}

func hasCUDARuntime() bool {
	for _, name := range []string{"cudart64_12.dll", "cudart64_13.dll", "cudart64_11.dll"} {
		lib, err := syscall.LoadLibrary(name)
		if err == nil {
			_ = syscall.FreeLibrary(lib)
			return true
		}
	}
	return false
}

func hasROCmRuntime() bool {
	for _, name := range []string{"amdhip64_7.dll", "amdhip64_6.dll"} {
		lib, err := syscall.LoadLibrary(name)
		if err == nil {
			_ = syscall.FreeLibrary(lib)
			return true
		}
	}
	return false
}

func hasOpenVINORuntime() bool {
	lib, err := syscall.LoadLibrary("openvino.dll")
	if err != nil {
		return false
	}
	_ = syscall.FreeLibrary(lib)
	return true
}

func hasVulkanRuntime() bool {
	lib, err := syscall.LoadLibrary("vulkan-1.dll")
	if err != nil {
		return false
	}
	_ = syscall.FreeLibrary(lib)
	return true
}
