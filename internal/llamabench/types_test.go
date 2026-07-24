package llamabench

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJSON_OK(t *testing.T) {
	data := []byte(`[
{"build_commit":"b9180","model_size":396705472,"n_batch":512,"n_ubatch":128,"n_threads":8,"n_gpu_layers":30,"flash_attn":true,"fit_target":0,"fit_min_ctx":4096,"avg_ts":42.5,"stddev_ts":0.2,"samples_ns":[1],"samples_ts":[2]}
]`)

	entries, err := ParseJSON(data)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "b9180", entries[0].BuildCommit)
	assert.Equal(t, int64(396705472), entries[0].ModelSize)
	assert.InDelta(t, 42.5, entries[0].AvgTS, 0.0001)
	assert.True(t, entries[0].FlashAttnBool())
}

func TestParseJSON_FlashAttnAsNumber(t *testing.T) {
	data := []byte(`[
{"build_commit":"b9180","model_size":396705472,"n_batch":512,"n_ubatch":128,"n_threads":8,"n_gpu_layers":30,"flash_attn":1,"fit_target":0,"fit_min_ctx":4096,"avg_ts":42.5}
]`)

	entries, err := ParseJSON(data)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].FlashAttnBool())
}

func TestParseJSON_FlashAttnAsStringNumber(t *testing.T) {
	data := []byte(`[
{"build_commit":"b9180","model_size":396705472,"n_batch":512,"n_ubatch":128,"n_threads":8,"n_gpu_layers":30,"flash_attn":"0","fit_target":0,"fit_min_ctx":4096,"avg_ts":42.5}
]`)

	entries, err := ParseJSON(data)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.False(t, entries[0].FlashAttnBool())
}

func TestParseJSON_Empty(t *testing.T) {
	_, err := ParseJSON([]byte(`[]`))
	assert.Error(t, err)
}

func TestParseJSON_Invalid(t *testing.T) {
	_, err := ParseJSON([]byte(`{}`))
	assert.Error(t, err)
}
