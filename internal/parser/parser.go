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
	ExportLineWithPathRX  = regexp.MustCompile(`(?i)export\s+(\*\s+from|\*\s+as\s+\w+\s+from|type\s+({[^}]+})\s+from|({[^}]+})\s+from)\s+['"]([^'"]+)['"]`)
	ExportLineWithRenamed = regexp.MustCompile(`.*\s+{[^}]+}\s+.*`)

	// ExportLineWithModuleRX is a regular expression that matches TypeScript/JavaScript export statements
	// and captures the exported identifier names and optional module paths.
	//
	// The regex matches the following export patterns:
	//   - Named exports: export class/function/const/let/var/enum/type/interface <name>
	//   - Default exports: export default class/function/const/let/var/enum/type/interface <name>
	//   - Modified declarations: zero or more of the abstract, declare and async modifiers,
	//     placed after any default (e.g. export default abstract class <name>). The regex is
	//     deliberately permissive here and does not reject combinations TypeScript forbids.
	//   - Generator functions: export function* <name>
	//   - Constant enums: export const enum <name>
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
	// The order of the declaration keyword branches is load-bearing: const enum must precede
	// const (otherwise "export const enum Color" captures the name "enum") and function* must
	// precede function. Each branch carries its own trailing separator so that the star in
	// "function *gen" can absorb the space while "const" still requires one.
	ExportLineWithModuleRX = regexp.MustCompile(`\bexport\s+(?:default\s+)?(?:(?:abstract|declare|async)\s+)*(?:class\s+|function\s*\*\s*|function\s+|const\s+enum\s+|const\s+|let\s+|var\s+|enum\s+|type\s+|interface\s+)([a-zA-Z_$][a-zA-Z0-9_$]*)|\bexport\s+(?:type\s+)?\{[^}]*\b([a-zA-Z_$][a-zA-Z0-9_$]*)\b[^}]*\}\s*(?:from\s+['"]([^'"]+)['"])?|\bexport\s+\*\s+as\s+([a-zA-Z_$][a-zA-Z0-9_$]*)\b\s*(?:from\s+['"]([^'"]+)['"])?`)

	// ExportAliasRX = regexp.MustCompile(`\bexport\s+(?:type\s+)?\{\s*(\w+)\s+as\s+\w+\b[^}]*\}\s*(?:from\s+['"](?:[^'"]+)['"])?`) // exportName as Alias
	ExportAliasRX = regexp.MustCompile(`\bexport\s+(?:type\s+)?\{\s*(\w+)\s+as\s+\w+\b[^}]*\}\s*(?:from\s+['"]([^'"]+)['"])?`) // exportName as Alias

	// This regex is used to identify default module exports in JavaScript/TypeScript files.
	ExportLineWithDefaultModuleRX = regexp.MustCompile(`.+\s*(default)\s+.+`)
)

type Parser struct {
	ignorer    ignorer.Ignorer
	rootPath   string
	extensions []string
}

type ExportKind int

func (exportKind *ExportKind) IsDefault() bool {
	return *exportKind == KindDefault
}

func (exportKind *ExportKind) IsNamespace() bool {
	return *exportKind == KindNamespace
}

func (exportKind *ExportKind) IsRenamed() bool {
	return *exportKind == KindRenamed
}

const (
	KindDeclaration ExportKind = iota // class/function/const/...
	KindDefault                       // export default ...
	KindNamed                         // export { Foo }
	KindRenamed                       // export { Foo as Bar }
	KindNamespace                     // export * as ns from ...
)

type ModuleResolverMapValue struct {
	Kind             ExportKind
	OriginModuleName string
	ModulePath       string
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

						if match[3] != "" {
							modulePath = match[3]
							parts := strings.Split(moduleName, " as ")
							if len(parts) == 2 {
								moduleName = strings.TrimSpace(parts[1])
							}
						} else if match[5] != "" {
							modulePath = match[5]
						}

						moduleNameBeforeRenaming := getModuleNameBeforeRenaming(exportKind, match[0], path, parser.extensions)
						if moduleNameBeforeRenaming != "" {
							exportKind = KindRenamed
						}

						aliasKey := filepath.Join(barrelDirAlias.FullPath, moduleName)
						directKey := filepath.Join(barrelDir, moduleName)
						moduleExtension := filepath.Ext(modulePath)
						modulePathWithoutExtension := modulePath[0 : len(modulePath)-len(moduleExtension)]
						barrelModuleResolverMapValue := ModuleResolverMapValue{
							Kind:             exportKind,
							OriginModuleName: moduleNameBeforeRenaming,
							ModulePath:       filepath.Join(modulePathWithoutExtension),
						}

						barrelModuleResolverMap[aliasKey] = barrelModuleResolverMapValue
						barrelModuleResolverMap[directKey] = barrelModuleResolverMapValue
					}
				}
				return nil
			})
		}
	}

	handleNestedBarrelModules(&barrelModuleResolverMap)

	return barrelPathExistenceMap, barrelModuleResolverMap
}

func getModuleNameBeforeRenaming(exportKind ExportKind, exportLine string, filePath string, extensions []string) string {
	moduleNameBeforeRenaming := ""
	if exportKind == KindNamed && isIndexFile(filePath, extensions) {
		matches := ExportAliasRX.FindAllStringSubmatch(exportLine, -1)
		if len(matches) >= 1 {
			moduleNameBeforeRenaming = matches[0][1]
		}
	}

	return moduleNameBeforeRenaming
}

func handleNestedBarrelModules(barrelModuleResolverMap *map[string]ModuleResolverMapValue) {
	for barrelDirWithModuleName, barrelModuleResolverMapValue := range *barrelModuleResolverMap {
		(*barrelModuleResolverMap)[barrelDirWithModuleName] = getBarrelModuleResolverMapValue(barrelModuleResolverMap, barrelDirWithModuleName, barrelModuleResolverMapValue)
	}
}

func getBarrelModuleResolverMapValue(barrelModuleResolverMap *map[string]ModuleResolverMapValue, barrelDirWithModuleName string, currentBarrelModuleResolverMapValue ModuleResolverMapValue) ModuleResolverMapValue {
	visitedDirs := map[string]struct{}{}

	for currentBarrelModuleResolverMapValue.Kind.IsRenamed() {
		newBarrelDirWithModuleName := filepath.Join(filepath.Dir(barrelDirWithModuleName), currentBarrelModuleResolverMapValue.ModulePath, currentBarrelModuleResolverMapValue.OriginModuleName)
		_, isVisited := visitedDirs[newBarrelDirWithModuleName]
		if newBarrelModuleResolverMapValue, exists := (*barrelModuleResolverMap)[newBarrelDirWithModuleName]; exists && !isVisited {
			newBarrelModuleResolverMapValue.ModulePath = filepath.Join(currentBarrelModuleResolverMapValue.ModulePath, newBarrelModuleResolverMapValue.ModulePath)
			currentBarrelModuleResolverMapValue = newBarrelModuleResolverMapValue
			visitedDirs[newBarrelDirWithModuleName] = struct{}{}
		} else {
			break
		}
	}

	return currentBarrelModuleResolverMapValue
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
			isNamedExport := match[3] != ""
			if isNamedExport {
				modulePaths = append(modulePaths, filepath.Base(filePath))
			}
			modulePath := match[4]
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
