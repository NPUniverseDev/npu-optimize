package benchmark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBandwidthGBs(t *testing.T) {
	bw := ComputeBandwidthGBs(1_000_000_000, 10)
	assert.InDelta(t, 10, bw, 0.0001)
}

func TestBytesPerToken(t *testing.T) {
	bpt := BytesPerToken(1_000_000, 1000, 500_000)
	assert.InDelta(t, 1500, bpt, 0.0001)
}

func TestEstimateTSFromBandwidth(t *testing.T) {
	ts, err := EstimateTSFromBandwidth(80, 2_000_000)
	require.NoError(t, err)
	assert.InDelta(t, 40000, ts, 1)
}
