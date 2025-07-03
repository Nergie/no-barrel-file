package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nergie/no-barrel-file/internal/tests"

	"github.com/stretchr/testify/assert"
)

func TestReplaceCommand(t *testing.T) {
	tmpDir := t.TempDir()
	inputDirPath := "../tests/data/input"
	initialRootPath := filepath.Join(tmpDir, inputDirPath)
	expectedDirPath := "../tests/data/expected"
	expectedRootPath := filepath.Join(tmpDir, expectedDirPath)
	defer os.RemoveAll(tmpDir)
	tests.CopyDir(inputDirPath, initialRootPath)
	tests.CopyDir(expectedDirPath, expectedRootPath)

	output, err := tests.ExecuteCommand(rootCmd, "replace", "--root-path", initialRootPath, "--ignore-paths", "ignored", "--alias-config-path", "tsconfig.json")

	assert.NoError(t, err)
	assert.Contains(t, output, "11 files updated\n")
	tests.CompareDirs(t, initialRootPath, expectedRootPath)
}
