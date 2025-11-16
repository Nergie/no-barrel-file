package cmd

import (
	"testing"

	"github.com/nergie/no-barrel-file/internal/tests"

	"github.com/stretchr/testify/assert"
)

func TestDisplayCommand(t *testing.T) {
	output, err := tests.ExecuteCommand(rootCmd, "display", "--root-path", "../tests/data/input", "--ignore-paths", "ignored")
	assert.NoError(t, err)
	assert.Contains(t, output, "20 barrel files found\nbarrel-basic/index.ts\nbarrel-circular/index.ts\nbarrel-default-as/pkg/index.ts\nbarrel-nested/Buttons/index.ts\nbarrel-nested/index.ts\nbarrel-nested/nested/index.ts\nbarrel-path-alias-complex/components/index.ts\nbarrel-path-alias-complex/libs/feature-a/src/index.ts\nbarrel-path-alias-complex/libs/feature-b/src/index.ts\nbarrel-path-alias-complex/libs/shared/src/index.ts\nbarrel-path-alias-complex/types/index.ts\nbarrel-path-alias-complex/utils/index.ts\nbarrel-rename-as/pkg/another-level-1/another-level-0/index.ts\nbarrel-rename-as/pkg/another-level-1/index.ts\nbarrel-rename-as/pkg/index.ts\nbarrel-very-nested/index.ts\nbarrel-very-nested/level1/index.ts\nbarrel-very-nested/level1/level2/index.ts\nbarrel-very-nested/level1/level2/level3/index.ts\nbarrel-very-nested/level1/level2/level3/level4/index.ts\n")
}
