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
	// export default class ModuleName || export class ModuleName || export function ModuleName || export const ModuleName || export let ModuleName || export enum ModuleName || export type ModuleName || export interface ModuleName || export { ModuleName } || export type { ModuleName }
	ExportLineWithModuleRX = regexp.MustCompile(`export\s+(?:default\s+)?(?:class|function|const|let|var|enum|type|interface)\s+([a-zA-Z_$][a-zA-Z0-9_$]*)|\bexport\s+(?:type\s+)?\{[^}]*\b([a-zA-Z_$][a-zA-Z0-9_$]*)\b[^}]*\}`)
	// New regexes for parsing barrel re-export specifiers directly from barrel files
	NamedReExportLineRX = regexp.MustCompile(`(?i)export\s+(?:type\s+)?\{([^}]*)\}\s+from\s+['"]([^'"]+)['"]`)
	AllReExportLineRX   = regexp.MustCompile(`(?i)export\s+\*\s+from\s+['"]([^'"]+)['"]`)
	AllAsReExportLineRX = regexp.MustCompile(`(?i)export\s+\*\s+as\s+\w+\s+from\s+['"]([^'"]+)['"]`)
	// Robust spec parsing helpers
	DefaultAsSpecRX = regexp.MustCompile(`(?i)^\s*default\s+as\s+([a-zA-Z_$][a-zA-Z0-9_$]*)\s*$`)
	RenameAsSpecRX  = regexp.MustCompile(`(?i)^\s*([a-zA-Z_$][a-zA-Z0-9_$]*)\s+as\s+([a-zA-Z_$][a-zA-Z0-9_$]*)\s*$`)
	LeadingTypeRX   = regexp.MustCompile(`(?i)^\s*type\s+`)
)

type Parser struct {
	ignorer    ignorer.Ignorer
	rootPath   string
	extensions []string
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

func (parser *Parser) BarrelMaps(resolver resolver.Resolver) (map[string]struct{}, map[string]string) {
	// First, use the original approach for standard barrel exports
	barrelDirsWithModulePaths := parser.getBarrelDirsWithModulePaths()
	barrelPathExistenceMap := make(map[string]struct{})
	barrelModuleResolverMap := make(map[string]string)

	// Process standard barrel exports using the original logic
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
						if moduleName == "" && len(match) > 2 {
							moduleName = match[2]
						}
						aliasKey := filepath.Join(barrelDirAlias.FullPath, moduleName)
						directKey := filepath.Join(barrelDir, moduleName)
						moduleExtension := filepath.Ext(modulePath)
						modulePathWithoutExtension := modulePath[0 : len(modulePath)-len(moduleExtension)]
						aliasValue := filepath.Join(modulePathWithoutExtension)
						directValue := filepath.Join(modulePathWithoutExtension)
						barrelModuleResolverMap[aliasKey] = aliasValue
						barrelModuleResolverMap[directKey] = directValue
					}
				}
				return nil
			})
		}
	}

	// Now overlay the default-as detection on top
	barrelFiles := parser.BarrelFilePaths()
	for _, barrelFile := range barrelFiles {
		barrelDir := filepath.ToSlash(filepath.Dir(barrelFile))
		barrelDirAlias := resolver.AliasPath(barrelDir)

		content, err := os.ReadFile(barrelFile)
		if err != nil {
			continue
		}
		text := string(content)

		// Handle named re-exports directly from the barrel file
		namedMatches := NamedReExportLineRX.FindAllStringSubmatch(text, -1)
		for _, m := range namedMatches {
			if len(m) < 3 {
				continue
			}
			specList := m[1]
			modulePath := strings.TrimSpace(m[2])

			specs := strings.Split(specList, ",")
			for _, spec := range specs {
				name := strings.TrimSpace(spec)
				if name == "" {
					continue
				}
				// remove optional inline "type" modifier with flexible spaces
				name = LeadingTypeRX.ReplaceAllString(name, "")
				exportedName := ""
				isDefaultReExport := false
				isRenameReExport := false
				sourceName := ""
				if m := DefaultAsSpecRX.FindStringSubmatch(name); m != nil {
					// export { default as X } from './mod' (robust whitespace)
					exportedName = m[1]
					isDefaultReExport = true
				} else if m := RenameAsSpecRX.FindStringSubmatch(name); m != nil {
					// export { A as X } from './mod' (robust whitespace)
					sourceName = m[1]
					exportedName = m[2]
					isRenameReExport = true
				} else {
					// export { A } from './mod'
					exportedName = strings.TrimSpace(name)
				}
				if exportedName == "" {
					continue
				}

				// Process all named re-exports (default-as, rename-as, and plain named)
				barrelPathExistenceMap[barrelDirAlias.FullPath] = struct{}{}
				barrelPathExistenceMap[barrelDir] = struct{}{}

				aliasKey := filepath.Join(barrelDirAlias.FullPath, exportedName)
				directKey := filepath.Join(barrelDir, exportedName)

				// Normalize module path and strip extension
				moduleValue := filepath.ToSlash(modulePath)
				moduleExtension := filepath.Ext(moduleValue)
				if moduleExtension != "" {
					moduleValue = moduleValue[0 : len(moduleValue)-len(moduleExtension)]
				}

				value := moduleValue
				if isDefaultReExport {
					value = "default|" + value
				} else if isRenameReExport {
					value = "rename|" + sourceName + "|" + value
				}
				barrelModuleResolverMap[aliasKey] = value
				barrelModuleResolverMap[directKey] = value
			}
		}

		// Handle star re-exports by scanning concrete files to map symbols -> file paths
		starMatches := AllReExportLineRX.FindAllStringSubmatch(text, -1)
		starAsMatches := AllAsReExportLineRX.FindAllStringSubmatch(text, -1)
		allStar := append(starMatches, starAsMatches...)
		for _, sm := range allStar {
			if len(sm) < 2 {
				continue
			}
			modulePath := strings.TrimSpace(sm[1])
			moduleRelFull := filepath.Join(barrelDir, modulePath)

			// choose a walk start path
			walkPath := ""
			if info, err := os.Stat(moduleRelFull); err == nil {
				if info.IsDir() {
					walkPath = moduleRelFull
				} else {
					walkPath = moduleRelFull
				}
			} else {
				for _, ext := range parser.extensions {
					try := moduleRelFull + ext
					if _, err := os.Stat(try); err == nil {
						walkPath = try
						break
					}
				}
			}
			if walkPath == "" {
				continue
			}

			// compute module root dir for relative file computation
			// Use the resolved walkPath (with extension fallback) to decide whether we're pointing at a file or directory.
			moduleRootFs := moduleRelFull
			moduleBasePrefix := ""
			if walkPath != "" {
				if fi, err := os.Stat(walkPath); err == nil {
					if fi.IsDir() {
						// Star re-exporting a directory: keep the module path as base prefix
						moduleRootFs = moduleRelFull
						moduleBasePrefix = filepath.ToSlash(modulePath)
					} else {
						// Star re-exporting a single file: preserve the nested folder structure by
						// using the directory portion of the module path as the base prefix and
						// walking relative to the file's directory (with extension resolved).
						moduleRootFs = filepath.Dir(walkPath)
						moduleBasePrefix = filepath.ToSlash(filepath.Dir(modulePath))
					}
				}
			}

			filepath.Walk(walkPath, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() || !parser.IsSupportedFileExtension(p) {
					return nil
				}
				bytes, err := os.ReadFile(p)
				if err != nil {
					return nil
				}
				matches := ExportLineWithModuleRX.FindAllStringSubmatch(string(bytes), -1)
				for _, match := range matches {
					if len(match) > 1 {
						moduleName := match[1]
						if moduleName == "" && len(match) > 2 {
							moduleName = match[2]
						}
						if moduleName == "" {
							continue
						}
						barrelPathExistenceMap[barrelDirAlias.FullPath] = struct{}{}
						barrelPathExistenceMap[barrelDir] = struct{}{}
						aliasKey := filepath.Join(barrelDirAlias.FullPath, moduleName)
						directKey := filepath.Join(barrelDir, moduleName)

						relWithin, err := filepath.Rel(moduleRootFs, p)
						if err != nil {
							continue
						}
						relWithin = filepath.ToSlash(relWithin)
						relWithin = strings.TrimSuffix(relWithin, filepath.Ext(relWithin))
						valuePath := relWithin
						if moduleBasePrefix != "" {
							valuePath = filepath.ToSlash(filepath.Join(moduleBasePrefix, relWithin))
						}

						barrelModuleResolverMap[aliasKey] = valuePath
						barrelModuleResolverMap[directKey] = valuePath
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
			// Skip self-referential exports
			if modulePath == "." || modulePath == "./." || modulePath == "./" {
				continue
			}
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
