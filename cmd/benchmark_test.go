package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeBenchmarkSchemaVersion(t *testing.T) {
	assert.Equal(t, 4, normalizeBenchmarkSchemaVersion(1))
	assert.Equal(t, 4, normalizeBenchmarkSchemaVersion(4))
	assert.Equal(t, 4, normalizeBenchmarkSchemaVersion(99))
}
