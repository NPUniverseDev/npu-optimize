package hwinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCardDevice_Valid(t *testing.T) {
	assert.True(t, isCardDevice("card0"))
	assert.True(t, isCardDevice("card1"))
	assert.True(t, isCardDevice("card128"))
}

func TestIsCardDevice_Invalid(t *testing.T) {
	assert.False(t, isCardDevice("card0-HDMI-A-1"))
	assert.False(t, isCardDevice("card0-DVI-I-1"))
	assert.False(t, isCardDevice("renderD128"))
	assert.False(t, isCardDevice("card"))
	assert.False(t, isCardDevice("controlD64"))
}

func TestVendorMap_AMD(t *testing.T) {
	v, ok := vendorMap["0x1002"]
	assert.True(t, ok)
	assert.Equal(t, "amd", v)
}

func TestVendorMap_Intel(t *testing.T) {
	v, ok := vendorMap["0x8086"]
	assert.True(t, ok)
	assert.Equal(t, "intel", v)
}

func TestVendorMap_NVIDIA(t *testing.T) {
	v, ok := vendorMap["0x10de"]
	assert.True(t, ok)
	assert.Equal(t, "nvidia", v)
}

func TestVulkanDrivers_AMD(t *testing.T) {
	assert.True(t, vulkanDrivers["amdgpu"])
}

func TestVulkanDrivers_Intel(t *testing.T) {
	assert.True(t, vulkanDrivers["i915"])
	assert.True(t, vulkanDrivers["xe"])
}

func TestVulkanDrivers_NVIDIA(t *testing.T) {
	assert.True(t, vulkanDrivers["nvidia"])
	assert.True(t, vulkanDrivers["nouveau"])
}

func TestVulkanDrivers_Unsupported(t *testing.T) {
	assert.False(t, vulkanDrivers["efi-framebuffer"])
	assert.False(t, vulkanDrivers["simplefb"])
}

func TestPCIClassConstants(t *testing.T) {
	assert.Equal(t, "0x030000", pciClassVGA)
	assert.Equal(t, "0x038000", pciClassDisplay)
}

func TestParseSoVersion_CUDA(t *testing.T) {
	output := `libcudart.so.12 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libcudart.so.12.4`
	soname, ver, ok := parseSoVersion("libcudart.so", output)
	assert.True(t, ok)
	assert.Equal(t, "libcudart.so.12", soname)
	assert.Equal(t, "12", ver)
}

func TestParseSoVersion_ROCm(t *testing.T) {
	output := `libamdhip64.so.6 (libc6,x86-64) => /opt/rocm/lib/libamdhip64.so.6.2.0`
	soname, ver, ok := parseSoVersion("libamdhip64.so", output)
	assert.True(t, ok)
	assert.Equal(t, "libamdhip64.so.6", soname)
	assert.Equal(t, "6", ver)
}

func TestParseSoVersion_OpenVINO(t *testing.T) {
	output := `libopenvino.so.2026 (libc6,x86-64) => /opt/intel/openvino/lib/libopenvino.so.2026.0.0`
	soname, ver, ok := parseSoVersion("libopenvino.so", output)
	assert.True(t, ok)
	assert.Equal(t, "libopenvino.so.2026", soname)
	assert.Equal(t, "2026", ver)
}

func TestParseSoVersion_Vulkan(t *testing.T) {
	output := `libvulkan.so.1 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libvulkan.so.1.3.275`
	soname, ver, ok := parseSoVersion("libvulkan.so", output)
	assert.True(t, ok)
	assert.Equal(t, "libvulkan.so.1", soname)
	assert.Equal(t, "1", ver)
}

func TestParseSoVersion_NoMatch(t *testing.T) {
	output := `libcudart.so.12 (libc6,x86-64) => /usr/lib/libcudart.so.12.4`
	_, _, ok := parseSoVersion("libnonexistent.so", output)
	assert.False(t, ok)
}

func TestParseSoVersion_EmptyOutput(t *testing.T) {
	_, _, ok := parseSoVersion("libcudart.so", "")
	assert.False(t, ok)
}
