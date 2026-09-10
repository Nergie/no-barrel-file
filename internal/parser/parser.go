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

// maxResolveHops caps how many re-export hops are followed when resolving a module name
// back to the file that declares it.
const maxResolveHops = 32

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

func (exportKind *ExportKind) IsReExport() bool {
	return *exportKind == KindReExport
}

// IsChainable reports whether the export forwards a name declared in another module,
// and so can be followed to the module that actually declares it.
func (exportKind *ExportKind) IsChainable() bool {
	return exportKind.IsRenamed() || exportKind.IsReExport()
}

const (
	KindDeclaration ExportKind = iota // class/function/const/...
	KindDefault                       // export default ...
	KindNamed                         // export { Foo }
	KindRenamed                       // export { Foo as Bar }
	KindNamespace                     // export * as ns from ...
	KindReExport                      // export { Foo } from './other'
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

						// Shadow the enclosing loop variable: an export line carrying its own
						// module path must not leak that path into the next iteration, and the
						// absence of a path here means "no from clause", not "reuse the last one".
						modulePath := modulePath
						if match[3] != "" {
							modulePath = normalizeIndexModulePath(filepath.Dir(path), match[3], parser.extensions, barrelDirsWithModulePaths)
							parts := strings.Split(moduleName, " as ")
							if len(parts) == 2 {
								moduleName = strings.TrimSpace(parts[1])
							}
						} else if match[5] != "" {
							modulePath = normalizeIndexModulePath(filepath.Dir(path), match[5], parser.extensions, barrelDirsWithModulePaths)
						}

						moduleNameBeforeRenaming := getModuleNameBeforeRenaming(exportKind, match[0], path, parser.extensions)
						if moduleNameBeforeRenaming != "" {
							exportKind = KindRenamed
						} else if exportKind == KindNamed && match[3] != "" && isIndexFile(path, parser.extensions) {
							// `export { Foo } from './other'` forwards a name declared elsewhere,
							// so it can be followed to the declaring module exactly like the
							// renamed form. `export { Foo };` carries no module path and must not
							// be followed -- the two are otherwise indistinguishable here, which
							// is why the kind is decided by the presence of match[3] rather than
							// by ModulePath being non-empty.
							exportKind = KindReExport
							moduleNameBeforeRenaming = moduleName
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
	// Seed with the starting key and mark before hopping, so that a re-export pointing at
	// its own barrel (`export { Foo } from '.'`) is caught on the first hop rather than
	// after taking it. maxResolveHops is a backstop for any cycle the key-based guard
	// cannot see; stopping there leaves the value unresolved rather than half-resolved.
	visitedDirs := map[string]struct{}{barrelDirWithModuleName: {}}

	for hop := 0; currentBarrelModuleResolverMapValue.Kind.IsChainable() && hop < maxResolveHops; hop++ {
		newBarrelDirWithModuleName := filepath.Join(filepath.Dir(barrelDirWithModuleName), currentBarrelModuleResolverMapValue.ModulePath, currentBarrelModuleResolverMapValue.OriginModuleName)
		if _, isVisited := visitedDirs[newBarrelDirWithModuleName]; isVisited {
			break
		}
		visitedDirs[newBarrelDirWithModuleName] = struct{}{}

		newBarrelModuleResolverMapValue, exists := (*barrelModuleResolverMap)[newBarrelDirWithModuleName]
		if !exists {
			break
		}

		// The declaring site wins: a chain landing on a default or namespace export has to
		// carry that kind back, otherwise the emitted import names a binding that does not
		// exist. ModulePath accumulates across hops; OriginModuleName comes from the hop we
		// just took and is meaningless once the chain reaches a plain declaration.
		newBarrelModuleResolverMapValue.ModulePath = filepath.Join(currentBarrelModuleResolverMapValue.ModulePath, newBarrelModuleResolverMapValue.ModulePath)
		currentBarrelModuleResolverMapValue = newBarrelModuleResolverMapValue
	}

	return currentBarrelModuleResolverMapValue
}

func (parser *Parser) IsSupportedFileExtension(path string) bool {
	return isSupportedFileExtension(path, parser.extensions)
}

func isSupportedFileExtension(path string, extensions []string) bool {
	for _, ext := range extensions {
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
	normalizeIndexModulePaths(&barrelDirsWithModulePaths, parser.extensions)
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
				} else if isSupportedFileExtension(path, extensions) {
					// The re-export spells out the extension ("./module.ts"), so the path
					// already resolves without one being appended.
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

// normalizeIndexModulePath rewrites a re-export path that points at a barrel's index file
// ("./services/index", "./services/index.ts") into the directory holding it ("./services"),
// so that from here on it is indistinguishable from the directory form. Without this a
// re-export written with an explicit /index suffix is recorded as a leaf module, the
// flattening in handleNestedBarrels never sees it, and none of the barrel's modules resolve.
// The importing side already strips /index in cmd.normalizeImportPath; this is the same
// normalisation applied to the exporting side.
//
// Two guards apply, and both matter:
//
//   - A directory literally named "index" keeps pointing at itself. Only a path that
//     resolves to a *file* is rewritten, so "./index/index" collapses to "./index" and
//     stops there rather than collapsing again to the barrel's own directory.
//
//   - The resulting directory must itself be a known barrel. An index.ts that merely holds
//     declarations is not a barrel, and rewriting toward its directory would widen
//     resolution to every symbol underneath it, including ones no barrel re-exports --
//     turning a path that fails to resolve into one that resolves to a broken import.
func normalizeIndexModulePath(sourceDir string, modulePath string, extensions []string, barrelDirsWithModulePaths map[string][]string) string {
	if !isIndexFile(modulePath, extensions) && filepath.Base(modulePath) != "index" {
		return modulePath
	}

	fullPath := filepath.Join(sourceDir, modulePath)
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		return modulePath
	}

	moduleDir := filepath.ToSlash(filepath.Dir(modulePath))
	if moduleDir == "." {
		// The barrel's own index file, either self-referenced or recorded by
		// getBarrelModulePaths so that named re-exports get scanned. Collapsing it to the
		// barrel directory would make the flattening treat it as an already-visited barrel
		// and drop it, taking every name the barrel re-exports directly with it.
		return modulePath
	}

	if _, isBarrel := barrelDirsWithModulePaths[filepath.ToSlash(filepath.Join(sourceDir, moduleDir))]; !isBarrel {
		return modulePath
	}

	return moduleDir
}

// normalizeIndexModulePaths runs normalizeIndexModulePath across every collected barrel.
// It has to run after the walk that builds barrelDirsWithModulePaths has finished, because
// the "is the target a known barrel" guard needs the completed set of barrel directories.
func normalizeIndexModulePaths(barrelDirsWithModulePaths *map[string][]string, extensions []string) {
	for dir, modulePaths := range *barrelDirsWithModulePaths {
		normalizedModulePaths := make([]string, 0, len(modulePaths))
		for _, modulePath := range modulePaths {
			normalizedModulePaths = append(normalizedModulePaths, normalizeIndexModulePath(dir, modulePath, extensions, *barrelDirsWithModulePaths))
		}
		(*barrelDirsWithModulePaths)[dir] = normalizedModulePaths
	}
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
