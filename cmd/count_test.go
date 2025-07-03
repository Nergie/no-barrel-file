package cmd

import (
	"testing"

	"github.com/nergie/no-barrel-file/internal/tests"

	"github.com/stretchr/testify/assert"
)

func TestCountCommand(t *testing.T) {
	output, err := tests.ExecuteCommand(rootCmd, "count", "--root-path", "../tests/data/input", "--ignore-paths", "ignored")
	assert.NoError(t, err)
	assert.Contains(t, output, "16\n")
}
