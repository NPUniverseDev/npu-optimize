package hwinfo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVRAMString_GB(t *testing.T) {
	assert.Equal(t, int64(36864), parseVRAMString("36 GB"))
	assert.Equal(t, int64(8192), parseVRAMString("8 GB"))
	assert.Equal(t, int64(1024), parseVRAMString("1 GB"))
}

func TestParseVRAMString_MB(t *testing.T) {
	assert.Equal(t, int64(8192), parseVRAMString("8192 MB"))
	assert.Equal(t, int64(1024), parseVRAMString("1024 MB"))
}

func TestParseVRAMString_KB(t *testing.T) {
	assert.Equal(t, int64(1), parseVRAMString("1024 KB"))
}

func TestParseVRAMString_Empty(t *testing.T) {
	assert.Equal(t, int64(0), parseVRAMString(""))
}

func TestParseVRAMString_Invalid(t *testing.T) {
	assert.Equal(t, int64(0), parseVRAMString("unknown"))
	assert.Equal(t, int64(0), parseVRAMString("GB"))
}

func TestParseVRAMString_CaseInsensitive(t *testing.T) {
	assert.Equal(t, int64(16384), parseVRAMString("16 GB"))
	assert.Equal(t, int64(16384), parseVRAMString("16 gb"))
}

func TestDarwinDisplayParsing_AppleSilicon(t *testing.T) {
	data := []byte(`{
		"SPDisplaysDataType": [
			{
				"_name": "Apple M3 Max",
				"spdisplays_vram": "36 GB",
				"spdisplays_vendor": "apple",
				"spdisplays_metal_family": "Apple M3 Max"
			}
		]
	}`)
	var sp darwinSPData
	err := json.Unmarshal(data, &sp)
	assert.NoError(t, err)
	assert.Len(t, sp.Displays, 1)
	assert.Equal(t, "Apple M3 Max", sp.Displays[0].Name)
	assert.Equal(t, "36 GB", sp.Displays[0].VRAM)
	assert.Equal(t, "apple", sp.Displays[0].Vendor)
}

func TestDarwinDisplayParsing_AMDdGPU(t *testing.T) {
	data := []byte(`{
		"SPDisplaysDataType": [
			{
				"_name": "AMD Radeon Pro 5600M",
				"spdisplays_vram": "8 GB",
				"spdisplays_vendor": "amd"
			}
		]
	}`)
	var sp darwinSPData
	err := json.Unmarshal(data, &sp)
	assert.NoError(t, err)
	assert.Len(t, sp.Displays, 1)
	assert.Equal(t, "AMD Radeon Pro 5600M", sp.Displays[0].Name)
	assert.Equal(t, "amd", sp.Displays[0].Vendor)
}
