package tests

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func CopyDir(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create the destination directory if it doesn't exist
	err = os.MkdirAll(dest, srcInfo.Mode())
	if err != nil {
		return err
	}

	// Open the source directory
	files, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each file into the destination directory
	for _, file := range files {
		srcFile := filepath.Join(src, file.Name())
		destFile := filepath.Join(dest, file.Name())

		if file.IsDir() {
			// Recursively copy subdirectories
			err = CopyDir(srcFile, destFile)
			if err != nil {
				return err
			}
		} else {
			err = CopyFile(srcFile, destFile)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyFile copies a single file from src to dest
func CopyFile(src, dest string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

// CompareDirs compares the contents of two directories recursively
func CompareDirs(t *testing.T, actualDir, expectedDir string) {
	actualFile, err := os.ReadDir(actualDir)
	assert.NoError(t, err)

	expectedFile, err := os.ReadDir(expectedDir)
	assert.NoError(t, err)

	// Assert that the number of files is the same
	assert.Len(t, actualFile, len(expectedFile))

	for i, file1 := range actualFile {
		file2 := expectedFile[i]

		// Compare if file names match
		assert.Equal(t, file1.Name(), file2.Name())

		// Compare if files are directories or files
		if file1.IsDir() {
			// Recursively compare subdirectories
			CompareDirs(t, filepath.Join(actualDir, file1.Name()), filepath.Join(expectedDir, file2.Name()))
		} else {
			// Compare file content if it's a regular file
			content1, err := os.ReadFile(filepath.Join(actualDir, file1.Name()))
			assert.NoError(t, err)

			content2, err := os.ReadFile(filepath.Join(expectedDir, file2.Name()))
			assert.NoError(t, err)

			assert.Equal(t, string(content2), string(content1))
		}
	}
}
