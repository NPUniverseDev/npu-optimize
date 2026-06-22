package hwinfo

import (
	"log/slog"
	"os/exec"

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
	backends := []string{"metal"}

	if hasVulkanRuntime() {
		backends = append(backends, "vulkan")
	}

	if info.GPU != nil {
		info.GPU.Backends = backends
	}

	slog.Debug("detected backends", "backends", backends)
}

func hasVulkanRuntime() bool {
	cmd := exec.Command("vulkaninfo", "--summary")
	return cmd.Run() == nil
}
