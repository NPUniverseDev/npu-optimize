package runtime

import (
	"testing"

	"github.com/Ericson246/npu-optimize/internal/hwinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
						ID:             "windows-cuda-12.4-x64",
						Platform:       "windows",
						Arch:           "x64",
						Backend:        "cuda",
						BackendVersion: "12.4",
						Version:        "b9704",
						DownloadURL:    "https://example.com/cuda-12.zip",
						SizeBytes:      1000,
						Format:         "zip",
						SourceName:     "ggml-org/llama.cpp",
						RequiresLib:    []string{"cudart64_12.dll"},
					},
					"windows-cuda-11.8-x64": {
						ID:             "windows-cuda-11.8-x64",
						Platform:       "windows",
						Arch:           "x64",
						Backend:        "cuda",
						BackendVersion: "11.8",
						Version:        "b9704",
						DownloadURL:    "https://example.com/cuda-11.zip",
						SizeBytes:      950,
						Format:         "zip",
						SourceName:     "ggml-org/llama.cpp",
						RequiresLib:    []string{"cudart64_11.dll"},
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
					"linux-cuda-12.4-x64": {
						ID:             "linux-cuda-12.4-x64",
						Platform:       "linux",
						Arch:           "x64",
						Backend:        "cuda",
						BackendVersion: "12.4",
						Version:        "b9704",
						DownloadURL:    "https://example.com/linux-cuda-12.tar.gz",
						SizeBytes:      1100,
						Format:         "tar.gz",
						SourceName:     "ggml-org/llama.cpp",
						RequiresLib:    []string{"libcudart.so.12"},
					},
					"linux-cuda-11.8-x64": {
						ID:             "linux-cuda-11.8-x64",
						Platform:       "linux",
						Arch:           "x64",
						Backend:        "cuda",
						BackendVersion: "11.8",
						Version:        "b9704",
						DownloadURL:    "https://example.com/linux-cuda-11.tar.gz",
						SizeBytes:      1050,
						Format:         "tar.gz",
						SourceName:     "ggml-org/llama.cpp",
						RequiresLib:    []string{"libcudart.so.11"},
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
						DownloadURL: "https://example.com/macos-arm64.tar.gz",
						SizeBytes:   300,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
					"darwin-cpu-arm64": {
						ID:          "darwin-cpu-arm64",
						Platform:    "darwin",
						Arch:        "arm64",
						Backend:     "cpu",
						Version:     "b9704",
						DownloadURL: "https://example.com/macos-cpu-arm64.tar.gz",
						SizeBytes:   250,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
					"darwin-cuda-12.4-x64": {
						ID:             "darwin-cuda-12.4-x64",
						Platform:       "darwin",
						Arch:           "x64",
						Backend:        "cuda",
						BackendVersion: "12.4",
						Version:        "b9704",
						DownloadURL:    "https://example.com/darwin-cuda-12.tar.gz",
						SizeBytes:      1100,
						Format:         "tar.gz",
						SourceName:     "ggml-org/llama.cpp",
						RequiresLib:    []string{"libcudart.so.12"},
					},
					"darwin-cuda-11.8-x64": {
						ID:             "darwin-cuda-11.8-x64",
						Platform:       "darwin",
						Arch:           "x64",
						Backend:        "cuda",
						BackendVersion: "11.8",
						Version:        "b9704",
						DownloadURL:    "https://example.com/darwin-cuda-11.tar.gz",
						SizeBytes:      1050,
						Format:         "tar.gz",
						SourceName:     "ggml-org/llama.cpp",
						RequiresLib:    []string{"libcudart.so.11"},
					},
					"darwin-vulkan-x64": {
						ID:          "darwin-vulkan-x64",
						Platform:    "darwin",
						Arch:        "x64",
						Backend:     "vulkan",
						Version:     "b9704",
						DownloadURL: "https://example.com/darwin-vulkan.tar.gz",
						SizeBytes:   600,
						Format:      "tar.gz",
						SourceName:  "ggml-org/llama.cpp",
					},
					"darwin-cpu-x64": {
						ID:          "darwin-cpu-x64",
						Platform:    "darwin",
						Arch:        "x64",
						Backend:     "cpu",
						Version:     "b9704",
						DownloadURL: "https://example.com/darwin-cpu.tar.gz",
						SizeBytes:   250,
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
			Backends:    []hwinfo.BackendInfo{{Name: "cuda"}, {Name: "vulkan"}},
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
			Backends:    []hwinfo.BackendInfo{{Name: "vulkan"}},
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

func nvidiaCUDA12HW() *hwinfo.Info {
	return &hwinfo.Info{
		GPU: &hwinfo.GPUInfo{
			Vendor:      "nvidia",
			Name:        "RTX 4060",
			VRAMTotalMB: 8192,
			VRAMFreeMB:  7000,
			Integrated:  false,
			Backends: []hwinfo.BackendInfo{
				{Name: "cuda", Version: "12", DetectedLib: "libcudart.so.12"},
				{Name: "vulkan"},
			},
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

func nvidiaCUDA11HW() *hwinfo.Info {
	return &hwinfo.Info{
		GPU: &hwinfo.GPUInfo{
			Vendor:      "nvidia",
			Name:        "GTX 1080",
			VRAMTotalMB: 8192,
			VRAMFreeMB:  6000,
			Integrated:  false,
			Backends: []hwinfo.BackendInfo{
				{Name: "cuda", Version: "11", DetectedLib: "libcudart.so.11"},
			},
		},
		CPU: hwinfo.CPUInfo{
			Name:    "Intel",
			Cores:   4,
			Threads: 8,
		},
		RAMTotalMB: 16384,
		RAMFreeMB:  12000,
	}
}

func TestSelect_CUDAPriority(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaCUDA12HW(), "", cat, "linux", "amd64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
	assert.Equal(t, "linux", entry.Platform)
}

func TestSelect_CUDAPicksExactLib(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaCUDA12HW(), "", cat, "linux", "amd64")
	assert.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
	assert.Equal(t, "12.4", entry.BackendVersion)
}

func TestSelect_CUDA11PicksCUDA11(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaCUDA11HW(), "", cat, "linux", "amd64")
	assert.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
	assert.Equal(t, "11.8", entry.BackendVersion)
	assert.Contains(t, entry.DownloadURL, "cuda-11")
}

func TestSelect_CUDAVersionFallbackToFirst(t *testing.T) {
	hw := nvidiaHW()
	cat := testCatalog()
	entry, err := Select(hw, "", cat, "linux", "amd64")
	assert.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
	// No detected lib → first deterministic match
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, "b9704", entry.Version)
}

func TestSelect_PreferVulkan(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "vulkan", cat, "linux", "amd64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "vulkan", entry.Backend)
}

func TestSelect_PreferCPU(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "cpu", cat, "darwin", "arm64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cpu", entry.Backend)
}

func TestSelect_IntegratedGPU(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(integratedGPUHW(), "", cat, "linux", "amd64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "vulkan", entry.Backend)
}

func TestSelect_CPUOnly(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(cpuOnlyHW(), "", cat, "darwin", "arm64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cpu", entry.Backend)
}

func TestSelect_NoCatalog(t *testing.T) {
	cat := &Catalog{Version: "1", Sources: []Source{}}
	_, err := Select(nvidiaHW(), "", cat, "linux", "amd64")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no compatible runtime found")
}

func TestSelect_InvalidPreferBackend(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "nonexistent", cat, "linux", "amd64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.Equal(t, "cuda", entry.Backend)
}

func TestSelect_PopulatesEntryID(t *testing.T) {
	cat := testCatalog()
	entry, err := Select(nvidiaHW(), "vulkan", cat, "linux", "amd64")
	assert.NoError(t, err)
	assert.NotNil(t, entry)
	assert.NotEmpty(t, entry.ID)
	assert.NotEmpty(t, entry.SourceName)
	assert.Equal(t, "ggml-org/llama.cpp", entry.SourceName)
}
