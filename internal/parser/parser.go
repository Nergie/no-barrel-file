package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nergie/no-barrel-file/internal/ignorer"
	"github.com/nergie/no-barrel-file/internal/resolver"
)

var (
	// export * from './module' || export * as ModuleName from './module' || export type { ModuleName } from './module' || export { ModuleName } from './module'
	ExportLineWithPathRX = regexp.MustCompile(`(?i)export\s+(\*\s+from|\*\s+as\s+\w+\s+from|type\s+{[^}]+}\s+from|{[^}]+}\s+from)\s+['"]([^'"]+)['"]`)

	// ExportLineWithModuleRX is a regular expression that matches TypeScript/JavaScript export statements
	// and captures the exported identifier names and optional module paths.
	//
	// The regex matches the following export patterns:
	//   - Named exports: export class/function/const/let/var/enum/type/interface <name>
	//   - Default exports: export default class/function/const/let/var/enum/type/interface <name>
	//   - Destructured exports: export { <name> } [from '<module>']
	//   - Type-only exports: export type { <name> } [from '<module>']
	//   - Namespace exports: export * as <name> [from '<module>']
	//
	// Capture groups:
	//   1. Identifier name from direct exports (class, function, const, etc.)
	//   2. Identifier name from destructured exports
	//   3. Module path from destructured exports with 'from' clause
	//   4. Namespace alias from 'export * as' statements
	//   5. Module path from namespace exports with 'from' clause
	ExportLineWithModuleRX = regexp.MustCompile(`\bexport\s+(?:default\s+)?(?:class|function|const|let|var|enum|type|interface)\s+([a-zA-Z_$][a-zA-Z0-9_$]*)|\bexport\s+(?:type\s+)?\{[^}]*\b([a-zA-Z_$][a-zA-Z0-9_$]*)\b[^}]*\}\s*(?:from\s+['"]([^'"]+)['"])?|\bexport\s+\*\s+as\s+([a-zA-Z_$][a-zA-Z0-9_$]*)\b\s*(?:from\s+['"]([^'"]+)['"])?`)

	// This regex is used to identify default module exports in JavaScript/TypeScript files.
	ExportLineWithDefaultModuleRX = regexp.MustCompile(`.+\s*(default)\s+.+`)
)

type Parser struct {
	ignorer    ignorer.Ignorer
	rootPath   string
	extensions []string
}

type ExportKind int

const (
	KindDeclaration ExportKind = iota
	KindDefault
	KindNamed
	KindNamespace
)

func (exportKind *ExportKind) IsDefault() bool {
	return *exportKind == KindDefault
}
func (exportKind *ExportKind) IsNamespace() bool {
	return *exportKind == KindNamespace
}

type ModuleResolverMapValue struct {
	Kind       ExportKind
	ModuleName string
}

func New(rootPath string, ignorer ignorer.Ignorer, extensions []string) Parser {
	return Parser{
		ignorer:    ignorer,
		rootPath:   rootPath,
		extensions: extensions,
	}
}

func (parser *Parser) BarrelFilePaths() []string {
	barrelFilePaths := []string{}
	filepath.Walk(parser.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to open file %s: %v\n", path, err)
			return nil
		}

		if parser.ignorer.IgnorePath(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() && isIndexFile(path, parser.extensions) {
			modulePaths := getBarrelModulePaths(path, parser.extensions)
			if len(modulePaths) > 0 {
				barrelFilePaths = append(barrelFilePaths, path)
			}
		}
		return nil
	})

	return barrelFilePaths
}

func (parser *Parser) BarrelMaps(resolver resolver.Resolver) (map[string]struct{}, map[string]ModuleResolverMapValue) {
	barrelDirsWithModulePaths := parser.getBarrelDirsWithModulePaths()
	barrelPathExistenceMap := make(map[string]struct{})
	barrelModuleResolverMap := make(map[string]ModuleResolverMapValue)
	for barrelDir, modulePaths := range barrelDirsWithModulePaths {
		barrelDirAlias := resolver.AliasPath(barrelDir)
		for _, modulePath := range modulePaths {
			moduleRelativePath := filepath.Join(barrelDir, modulePath)
			filepath.Walk(moduleRelativePath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !parser.IsSupportedFileExtension(path) {
					return nil
				}

				contents, err := os.ReadFile(path)
				if err != nil {
					return nil
				}

				matches := ExportLineWithModuleRX.FindAllStringSubmatch(string(contents), -1)
				for _, match := range matches {
					if len(match) > 1 {
						barrelPathExistenceMap[barrelDirAlias.FullPath] = struct{}{}
						barrelPathExistenceMap[barrelDir] = struct{}{}

						moduleName := match[1]
						exportKind := KindDeclaration
						if moduleName == "" {
							moduleName = match[2]
							exportKind = KindNamed
						}
						if moduleName == "" {
							moduleName = match[4]
							exportKind = KindNamespace
						} else if ExportLineWithDefaultModuleRX.MatchString(match[0]) {
							exportKind = KindDefault
						}
						aliasKey := filepath.Join(barrelDirAlias.FullPath, moduleName)
						directKey := filepath.Join(barrelDir, moduleName)

						if match[3] != "" {
							modulePath = match[3]
						}
						if match[5] != "" {
							modulePath = match[5]
						}

						moduleExtension := filepath.Ext(modulePath)
						modulePathWithoutExtension := modulePath[0 : len(modulePath)-len(moduleExtension)]
						barrelModuleResolverMapValue := ModuleResolverMapValue{
							Kind:       exportKind,
							ModuleName: filepath.Join(modulePathWithoutExtension),
						}

						barrelModuleResolverMap[aliasKey] = barrelModuleResolverMapValue
						barrelModuleResolverMap[directKey] = barrelModuleResolverMapValue
					}
				}
				return nil
			})
		}
	}

	return barrelPathExistenceMap, barrelModuleResolverMap
}

func (parser *Parser) IsSupportedFileExtension(path string) bool {
	for _, ext := range parser.extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

func (parser *Parser) getBarrelDirsWithModulePaths() map[string][]string {
	barrelDirsWithModulePaths := make(map[string][]string)
	filepath.Walk(parser.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to open file %s: %v\n", path, err)
			return nil
		}

		if parser.ignorer.IgnorePath(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() && isIndexFile(path, parser.extensions) {
			modulePaths := getBarrelModulePaths(path, parser.extensions)
			if len(modulePaths) > 0 {
				dirPath := filepath.ToSlash(filepath.Dir(path))
				barrelDirsWithModulePaths[dirPath] = modulePaths
			}
		}
		return nil
	})
	handleNestedBarrels(&barrelDirsWithModulePaths)
	return barrelDirsWithModulePaths
}

func getBarrelModulePaths(filePath string, extensions []string) []string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening barrel file %s: %v\n", filePath, err)
		return nil
	}

	var modulePaths []string
	matches := ExportLineWithPathRX.FindAllStringSubmatch(string(content), -1)
	for _, match := range matches {
		if len(match) > 1 {
			modulePath := match[2]
			path := filepath.Join(filepath.Dir(filePath), modulePath)
			if info, err := os.Stat(path); err == nil {
				if info.IsDir() {
					modulePaths = append(modulePaths, filepath.ToSlash(modulePath))
				}
			} else {
				for _, extension := range extensions {
					pathWithExtension := path + extension
					if _, err := os.Stat(pathWithExtension); err == nil {
						modulePaths = append(modulePaths, filepath.ToSlash(modulePath+extension))
						break
					}
				}
			}
		}
	}

	return modulePaths
}

func isIndexFile(path string, extensions []string) bool {
	for _, ext := range extensions {
		if filepath.Base(path) == "index"+ext {
			return true
		}
	}
	return false
}

func handleNestedBarrels(barrelDirsWithModulePaths *map[string][]string) {
	for dir, modulePaths := range *barrelDirsWithModulePaths {
		visitedDirs := map[string]struct{}{}
		visitedDirs[dir] = struct{}{}
		resolvedModulePaths := []string{}
		for _, modulePath := range modulePaths {
			path := filepath.Join(dir, modulePath)
			if _, exists := (*barrelDirsWithModulePaths)[path]; exists {
				resolvedNestedModulePaths := getResolvedModulePaths(path, modulePath, *barrelDirsWithModulePaths, visitedDirs)
				resolvedModulePaths = append(resolvedModulePaths, resolvedNestedModulePaths...)
			} else {
				resolvedModulePaths = append(resolvedModulePaths, modulePath)
			}
		}
		(*barrelDirsWithModulePaths)[dir] = resolvedModulePaths
	}
}

func getResolvedModulePaths(fullDirPath string, relDirPath string, barrelDirsWithModulePaths map[string][]string, visitedDirs map[string]struct{}) []string {
	if _, exists := visitedDirs[fullDirPath]; exists {
		return []string{}
	}
	nestedPaths, exists := barrelDirsWithModulePaths[fullDirPath]
	if !exists {
		return []string{}
	}

	visitedDirs[fullDirPath] = struct{}{}
	resolvedModulePaths := []string{}
	for _, modulePath := range nestedPaths {
		path := filepath.Join(fullDirPath, modulePath)
		resolvedModulePath := filepath.Join(relDirPath, modulePath)
		if _, exists := barrelDirsWithModulePaths[path]; exists {
			resolvedNestedModulePaths := getResolvedModulePaths(path, resolvedModulePath, barrelDirsWithModulePaths, visitedDirs)
			resolvedModulePaths = append(resolvedModulePaths, resolvedNestedModulePaths...)
		} else {
			resolvedModulePaths = append(resolvedModulePaths, resolvedModulePath)
		}
	}
	return resolvedModulePaths
}
