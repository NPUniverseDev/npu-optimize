package runtime

import (
	"runtime"
	"testing"

	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/stretchr/testify/assert"
)

func testCatalog() *Catalog {
	return &Catalog{
		Version: "1",
		Sources: []Source{
			{
				Name: "ggml-org/llama.cpp",
				Repo: "ggml-org/llama.cpp",
				Runtimes: map[string]RuntimeEntry{
					"windows-cuda-12.4-x64": {
						ID:          "windows-cuda-12.4-x64",
						Platform:    "windows",
						Arch:        "x64",
						Backend:     "cuda",
						Version:     "b9704",
						DownloadURL: "https://example.com/cuda.zip",
						SizeBytes:   1000,
						Format:      "zip",
						SourceName:  "ggml-org/llama.cpp",
					},
					"windows-vulkan-x64": {
						ID:          "windows-vulkan-x64",
						Platform:    "windows",
						Arch:        "x64",
						Backend:     "vulkan",
						Version:     "b9704",
						DownloadURL: "https://example.com/vulkan.zip",
						SizeBytes:   500,
						Format:      "zip",
						SourceName:  "ggml-org/llama.cpp",
					},
					"windows-cpu-x64": {
						ID:          "windows-cpu-x64",
						Platform:    "windows",
						Arch:        "x64",
						Backend:     "cpu",
						Version:     "b9704",
						DownloadURL: "https://example.com/cpu.zip",
						SizeBytes:   200,
						Format:      "zip",
						SourceName:  "ggml-org/llama.cpp",
					},
					"linux-vulkan-x64": {
						ID:          "linux-vulkan-x64",
						Platform:    "linux",
						Arch:        "x64",
						Backend:     "vulkan",
						Version:     "b9704",
						DownloadURL: "https://example.com/linux-vulkan.tar.gz",
						SizeBytes:   600,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
					"linux-cuda-x64": {
						ID:          "linux-cuda-x64",
						Platform:    "linux",
						Arch:        "x64",
						Backend:     "cuda",
						Version:     "b9704",
						DownloadURL: "https://example.com/linux-cuda.tar.gz",
						SizeBytes:   1100,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
					"linux-cpu-x64": {
						ID:          "linux-cpu-x64",
						Platform:    "linux",
						Arch:        "x64",
						Backend:     "cpu",
						Version:     "b9704",
						DownloadURL: "https://example.com/linux-cpu.tar.gz",
						SizeBytes:   250,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
					"darwin-metal-arm64": {
						ID:          "darwin-metal-arm64",
						Platform:    "darwin",
						Arch:        "arm64",
						Backend:     "metal",
						Version:     "b9704",
						DownloadURL: "https://example.com/macos.tar.gz",
						SizeBytes:   300,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
				},
			},
		},
	}
}

func nvidiaHW() *hwinfo.Info {
	return &hwinfo.Info{
		GPU: &hwinfo.GPUInfo{
			Vendor:      "nvidia",
			Name:        "RTX 4060",
			VRAMTotalMB: 8192,
			VRAMFreeMB:  7000,
			Integrated:  false,
			Backends:    []string{"cuda", "vulkan"},
		},
		CPU: hwinfo.CPUInfo{
			Name:    "Intel",
			Cores:   8,
			Threads: 16,
		},
		RAMTotalMB: 32768,
		RAMFreeMB:  24000,
	}
}

func integratedGPUHW() *hwinfo.Info {
	return &hwinfo.Info{
		GPU: &hwinfo.GPUInfo{
			Vendor:      "intel",
			Name:        "Intel UHD Graphics",
			VRAMTotalMB: 0,
			VRAMFreeMB:  4000,
			Integrated:  true,
			Backends:    []string{"vulkan"},
		},
		CPU: hwinfo.CPUInfo{
			Name:    "Intel",
			Cores:   4,
			Threads: 8,
		},
		RAMTotalMB: 16384,
		RAMFreeMB:  8000,
	}
}

func cpuOnlyHW() *hwinfo.Info {
	return &hwinfo.Info{
		CPU: hwinfo.CPUInfo{
			Name:    "Intel",
			Cores:   4,
			Threads: 8,
		},
		RAMTotalMB: 8192,
		RAMFreeMB:  4000,
	}
}

func TestSelect_CUDAPriority(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
	assert.Equal(t, runtime.GOOS, entry.Platform)
}

func TestSelect_PreferVulkan(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "vulkan", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "vulkan", entry.Backend)
}

func TestSelect_PreferCPU(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "cpu", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cpu", entry.Backend)
}

func TestSelect_IntegratedGPU(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(integratedGPUHW(), "", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "vulkan", entry.Backend)
}

func TestSelect_CPUOnly(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(cpuOnlyHW(), "", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cpu", entry.Backend)
}

func TestSelect_NoCatalog(t *testing.T) {
	cat := &Catalog{Version: "1", Sources: []Source{}}
	_, err := Select(nvidiaHW(), "", cat)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no compatible runtime found")
}

func TestSelect_InvalidPreferBackend(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "nonexistent", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
}

func TestSelect_PopulatesEntryID(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "vulkan", cat)
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.NotEmpty(t, entry.ID)
	assert.NotEmpty(t, entry.SourceName)
	assert.Equal(t, "ggml-org/llama.cpp", entry.SourceName)
}
