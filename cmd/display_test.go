package cmd

import (
	"strings"
	"testing"

	"github.com/nergie/no-barrel-file/internal/tests"

	"github.com/stretchr/testify/assert"
)

func TestDisplayCommand(t *testing.T) {
	output, err := tests.ExecuteCommand(rootCmd, "display", "--root-path", "../tests/data/input", "--ignore-paths", "ignored")
	assert.NoError(t, err)
	unix := strings.ReplaceAll(output, "\\", "/")
	assert.Contains(t, unix, "19 barrel files found\nbarrel-basic/index.ts\nbarrel-circular/index.ts\nbarrel-default-as/pkg/index.ts\nbarrel-nested/Buttons/index.ts\nbarrel-nested/index.ts\nbarrel-nested/nested/index.ts\n")
}
