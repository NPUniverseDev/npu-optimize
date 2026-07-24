package cmd

import (
	"testing"

	"github.com/Ericson246/npu-optimize/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeBenchmarkSchemaVersion(t *testing.T) {
	assert.Equal(t, 4, normalizeBenchmarkSchemaVersion(1))
	assert.Equal(t, 4, normalizeBenchmarkSchemaVersion(4))
	assert.Equal(t, 4, normalizeBenchmarkSchemaVersion(99))
}

func TestDefaultMinTS(t *testing.T) {
	assert.Equal(t, 8.0, constants.DefaultMinTS)
}
