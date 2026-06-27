package hwinfo

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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
	_, err := os.Stat("/proc/driver/nvidia")
	if err != nil {
		return "", "", false
	}
	return detectSoVersion("libcudart.so")
}

func detectROCmRuntime() (lib, version string, ok bool) {
	_, err := os.Stat("/sys/class/kfd")
	if err != nil {
		return "", "", false
	}
	return detectSoVersion("libamdhip64.so")
}

func detectOpenVINORuntime() (lib, version string, ok bool) {
	dirs := []string{"/opt/intel/openvino", "/opt/intel/openvino_2026"}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			return dir, "", true
		}
	}
	return detectSoVersion("libopenvino.so")
}

func detectVulkanRuntime() (lib, version string, ok bool) {
	paths := []string{
		"/usr/lib/x86_64-linux-gnu/libvulkan.so",
		"/usr/lib/x86_64-linux-gnu/libvulkan.so.1",
		"/usr/lib/aarch64-linux-gnu/libvulkan.so",
		"/usr/lib/aarch64-linux-gnu/libvulkan.so.1",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, "", true
		}
	}
	return detectSoVersion("libvulkan.so")
}

func detectSoVersion(soPrefix string) (lib, version string, ok bool) {
	cmd := exec.Command("ldconfig", "-p")
	out, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	return parseSoVersion(soPrefix, string(out))
}

func parseSoVersion(soPrefix, ldconfigOutput string) (lib, version string, ok bool) {
	for _, line := range strings.Split(ldconfigOutput, "\n") {
		if !strings.Contains(line, soPrefix) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		soname := parts[0]
		ver := strings.TrimPrefix(soname, soPrefix+".")
		return soname, ver, true
	}
	return "", "", false
}

var vulkanDrivers = map[string]bool{
	"amdgpu":   true,
	"i915":     true,
	"xe":       true,
	"nvidia":   true,
	"nouveau":  true,
	"msm":      true,
	"panfrost": true,
	"v3d":      true,
}

var vendorMap = map[string]string{
	"0x1002": "amd",
	"0x8086": "intel",
	"0x10de": "nvidia",
}

const (
	pciClassVGA     = "0x030000"
	pciClassDisplay = "0x038000"
)

func isCardDevice(name string) bool {
	trimmed := strings.TrimPrefix(name, "card")
	if trimmed == "" {
		return false
	}
	for _, c := range trimmed {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func readPCIIDs(vendorID, deviceID string) string {
	idsPaths := []string{
		"/usr/share/hwdata/pci.ids",
		"/usr/share/misc/pci.ids",
	}
	for _, path := range idsPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		inVendor := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !inVendor {
				if strings.HasPrefix(line, vendorID[2:]) && (len(line) == 4 || line[4] == ' ') {
					inVendor = true
					continue
				}
				continue
			}
			if trimmed == "" {
				break
			}
			if line[0] != '\t' && line[0] != ' ' {
				break
			}
			trimmed = strings.TrimSpace(trimmed)
			if strings.HasPrefix(trimmed, deviceID[2:]) && (len(trimmed) == 4 || trimmed[4] == ' ') {
				parts := strings.SplitN(trimmed, " ", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return ""
}

type sysfsCandidate struct {
	Vendor      string
	Name        string
	Driver      string
	VRAMTotalMB int64
	Integrated  bool
}

func detectVulkanGPUFallback(info *Info) bool {
	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		slog.Debug("vulkan fallback sysfs: cannot read /sys/class/drm", "err", err)
		return false
	}

	var candidates []sysfsCandidate

	for _, entry := range entries {
		if !isCardDevice(entry.Name()) {
			continue
		}

		devicePath := filepath.Join("/sys/class/drm", entry.Name(), "device")

		driverPath, err := os.Readlink(filepath.Join(devicePath, "driver"))
		if err != nil {
			continue
		}
		driverName := filepath.Base(driverPath)
		if !vulkanDrivers[driverName] {
			continue
		}

		vendorBytes, err := os.ReadFile(filepath.Join(devicePath, "vendor"))
		if err != nil {
			continue
		}
		vendorID := strings.TrimSpace(string(vendorBytes))
		vendor, ok := vendorMap[vendorID]
		if !ok {
			continue
		}

		deviceBytes, err := os.ReadFile(filepath.Join(devicePath, "device"))
		if err != nil {
			continue
		}
		deviceID := strings.TrimSpace(string(deviceBytes))

		gpuName := readPCIIDs(vendorID, deviceID)
		if gpuName == "" {
			gpuName = fmt.Sprintf("GPU %s:%s", vendorID, deviceID)
		}

		classBytes, err := os.ReadFile(filepath.Join(devicePath, "class"))
		integrated := false
		if err == nil {
			classStr := strings.TrimSpace(string(classBytes))
			integrated = classStr == pciClassDisplay
		} else if vendor == "intel" {
			integrated = true
		}

		var vramTotalMB int64
		vramBytes, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_total"))
		if err == nil {
			vramStr := strings.TrimSpace(string(vramBytes))
			if vram, parseErr := strconv.ParseInt(vramStr, 10, 64); parseErr == nil {
				vramTotalMB = vram / 1024 / 1024
			}
		}
		if vramTotalMB <= 0 {
			vramTotalMB = info.RAMTotalMB / 2
			integrated = true
		}

		candidates = append(candidates, sysfsCandidate{
			Vendor:      vendor,
			Name:        gpuName,
			Driver:      driverName,
			VRAMTotalMB: vramTotalMB,
			Integrated:  integrated,
		})
	}

	if len(candidates) == 0 {
		return false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.Integrated && !best.Integrated {
			continue
		}
		if !c.Integrated && best.Integrated {
			best = c
			continue
		}
		if c.VRAMTotalMB > best.VRAMTotalMB {
			best = c
		}
	}

	vramFreeMB := best.VRAMTotalMB
	if best.Integrated && info.RAMFreeMB < best.VRAMTotalMB {
		vramFreeMB = info.RAMFreeMB
	}

	info.GPU = &GPUInfo{
		Vendor:      best.Vendor,
		Name:        best.Name,
		VRAMTotalMB: best.VRAMTotalMB,
		VRAMFreeMB:  vramFreeMB,
		Integrated:  best.Integrated,
	}

	slog.Warn("vulkan GPU detected via sysfs fallback (vulkaninfo not found)",
		"vendor", best.Vendor, "gpu", best.Name,
		"driver", best.Driver,
		"vram_mb", best.VRAMTotalMB,
		"candidates", len(candidates),
	)
	return true
}
