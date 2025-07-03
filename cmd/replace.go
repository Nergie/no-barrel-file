package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nergie/no-barrel-file/internal/cmd_flag"
	"github.com/nergie/no-barrel-file/internal/data"
	"github.com/nergie/no-barrel-file/internal/ignorer"
	"github.com/nergie/no-barrel-file/internal/parser"
	"github.com/nergie/no-barrel-file/internal/resolver"

	"github.com/spf13/cobra"
)

var (
	// import type { ModuleName } from 'module' || import { ModuleName } from 'module'
	NamedImportLineRX = regexp.MustCompile(`import\s+(type {[^}]+}|{[^}]+})\s+from\s+(['"])([^'"]+)['"](;?)`)
	TypeImportRX      = regexp.MustCompile(`type\s+[{]?\s*(\w+)`) // type { exportName }
	AliasImportRX     = regexp.MustCompile(`(\w+)\s+as\s+\w+`)    // exportName as Alias
)

type ReplaceConfig struct {
	RootConfig
	aliasConfigPath string
	targetPath      string
	barrelPath      string
	verbose         bool
}

func NewReplaceConfig(cmd *cobra.Command) ReplaceConfig {
	return ReplaceConfig{
		RootConfig:      NewRootConfig(cmd),
		aliasConfigPath: cmd_flag.AliasConfigPath(cmd),
		targetPath:      cmd_flag.TargetPath(cmd),
		barrelPath:      cmd_flag.BarrelPath(cmd),
		verbose:         cmd_flag.Verbose(cmd),
	}
}

func normalizeImportPath(path string, extensions []string) string {
	// Remove various index file patterns using configured extensions
	for _, ext := range extensions {
		indexPattern := "/index" + ext
		if strings.HasSuffix(path, indexPattern) {
			return strings.TrimSuffix(path, indexPattern)
		}
	}

	// Also handle bare /index
	if strings.HasSuffix(path, "/index") {
		return strings.TrimSuffix(path, "/index")
	}

	return path
}

var replaceCmd = &cobra.Command{
	Use:   "replace",
	Short: "Replace barrel files imports",
	Run: func(cmd *cobra.Command, args []string) {
		config := NewReplaceConfig(cmd)
		updatedFilesTotal := replaceBarrelImports(cmd, config)
		fmt.Fprintf(cmd.OutOrStdout(), "%d files updated\n", updatedFilesTotal)
	},
}

func init() {
	cmd_flag.AddAliasConfigPath(replaceCmd)
	cmd_flag.AddTargetPath(replaceCmd)
	cmd_flag.AddBarrelPath(replaceCmd)
	cmd_flag.AddVerbose(replaceCmd)
}

func replaceBarrelImports(cmd *cobra.Command, config ReplaceConfig) int {
	resolver := resolver.New(config.rootPath, &config.aliasConfigPath)
	ignorer := ignorer.New(config.rootPath, config.ignorePaths, config.gitIgnorePath)
	parserRootPath := joinCrossPlatformPaths(config.rootPath, config.barrelPath)
	parser := parser.New(parserRootPath, ignorer, config.extensions)
	barrelResolvedPaths := data.NewBarrelResolvedPath(parser, resolver)
	targetFullPath := joinCrossPlatformPaths(config.rootPath, config.targetPath)
	updatedFilesTotal := 0

	filepath.Walk(targetFullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		if !parser.IsSupportedFileExtension(path) {
			return nil
		}

		if ignorer.IgnorePath(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		updatedContents := NamedImportLineRX.ReplaceAllStringFunc(string(contents), func(importStatement string) string {
			matches := NamedImportLineRX.FindStringSubmatch(importStatement)
			if len(matches) < 4 {
				return importStatement
			}

			importNames := strings.Split(matches[1], ",")
			quoteSymbol := matches[2]
			importPath := normalizeImportPath(matches[3], config.extensions)
			endSymbol := matches[4]
			isAliasPath := resolver.IsAliasPath(importPath)
			var resolvedPathKey string
			if isAliasPath {
				resolvedPathKey = importPath
			} else {
				resolvedPathKey = joinCrossPlatformPaths(filepath.Dir(path), importPath)
			}

			if !barrelResolvedPaths.IsResolved(resolvedPathKey) {
				return importStatement
			}

			replacedImports := []string{}
			importsByModule := make(map[string][]string)
			orderedImportPaths := []string{}

			for _, importName := range importNames {
				importName = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(importName, "{"), "}"))
				if importName == "" {
					continue
				}
				moduleName := getModuleName(importName)
				resolvedModulePath, exists := barrelResolvedPaths.ResolveModuleName(resolvedPathKey, moduleName)
				var newImportPath string
				if exists {
					newImportPath = joinCrossPlatformPaths(resolvedPathKey, resolvedModulePath)
					if !isAliasPath {
						newImportPath = joinCrossPlatformPaths(importPath, resolvedModulePath)
						if !strings.HasPrefix(newImportPath, "./") && !strings.HasPrefix(newImportPath, "../") {
							newImportPath = "./" + newImportPath
						}
					}
				} else {
					newImportPath = importPath
				}
				if _, exists := importsByModule[newImportPath]; !exists {
					orderedImportPaths = append(orderedImportPaths, newImportPath)
				}

				importsByModule[newImportPath] = append(importsByModule[newImportPath], importName)
			}

			for _, resolvedPath := range orderedImportPaths {
				importNames := importsByModule[resolvedPath]
				newImportStatement := "import "
				isTypeImport := strings.HasPrefix(matches[1], "type {") || (len(importNames) == 1 && strings.Contains(importNames[0], "type "))
				if isTypeImport {
					newImportStatement += "type { "
				} else {
					newImportStatement += "{ "
				}

				for _, importName := range importNames {
					if isTypeImport {
						class := getModuleName(importName)
						newImportStatement += class + ", "
					} else {
						newImportStatement += importName + ", "
					}
				}

				newImportStatement = strings.TrimSuffix(newImportStatement, ", ")
				newImportStatement += fmt.Sprintf(" } from %s%s%s%s", quoteSymbol, resolvedPath, quoteSymbol, endSymbol)
				replacedImports = append(replacedImports, newImportStatement)
			}

			if len(replacedImports) > 0 {
				replacedImportStatement := strings.Join(replacedImports, "\n")
				if config.verbose {
					cmd.Printf("Updating imports in %s:\nBefore:\n%s\nAfter:\n%s\n\n", path, importStatement, replacedImportStatement)
				}
				return replacedImportStatement
			}

			return importStatement
		})

		if updatedContents != string(contents) {
			os.WriteFile(path, []byte(updatedContents), info.Mode())
			updatedFilesTotal += 1
		}

		return nil
	})
	return updatedFilesTotal
}

func getModuleName(line string) string {
	matches := TypeImportRX.FindStringSubmatch(line)
	if len(matches) >= 2 {
		return matches[1]
	}

	matches = AliasImportRX.FindStringSubmatch(line)
	if len(matches) >= 2 {
		return matches[1]
	}

	return line
}

func joinCrossPlatformPaths(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}
