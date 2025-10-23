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
	// import type { A } from 'x' | import { A, B } from 'x' with optional newlines
	NamedImportLineRX = regexp.MustCompile(`(?s)import\s+(type\s+)?\{([^}]*)\}\s+from\s+(['"])([^'"]+)['"]\s*(;?)`)
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
		if err != nil {
			return err
		}

		if ignorer.IgnorePath(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// Reuse config extensions directly (parser.IsSupportedFileExtension expects parser receiver)
		isSupported := false
		for _, ext := range config.extensions {
			if strings.HasSuffix(path, ext) {
				isSupported = true
				break
			}
		}
		if !isSupported {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		updatedContents := NamedImportLineRX.ReplaceAllStringFunc(string(contents), func(importStatement string) string {
			matches := NamedImportLineRX.FindStringSubmatch(importStatement)
			if len(matches) < 5 {
				return importStatement
			}

			// matches[1]: optional "type "
			// matches[2]: spec list inside {}
			// matches[3]: quote ' or "
			// matches[4]: import path
			// matches[5]: optional ;
			importNames := strings.Split(matches[2], ",")
			quoteSymbol := matches[3]
			importPath := normalizeImportPath(matches[4], config.extensions)
			endSymbol := matches[5]
			origWasRelative := importPath == "." || importPath == "./" || importPath == "./." || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../")
			isAliasPath := resolver.IsAliasPath(importPath)
			var resolvedPathKey string
			if isAliasPath && !origWasRelative {
				resolvedPathKey = importPath
			} else {
				// Normalize '.' style imports to the barrel directory key
				ip := strings.TrimSpace(importPath)
				if ip == "." || ip == "./" || ip == "./." {
					resolvedPathKey = filepath.ToSlash(filepath.Dir(path))
				} else {
					resolvedPathKey = joinCrossPlatformPaths(filepath.Dir(path), importPath)
				}
			}

			if !barrelResolvedPaths.IsResolved(resolvedPathKey) {
				if config.verbose {
					cmd.Printf("Skipping non-barrel import %q in %s\n", resolvedPathKey, path)
				}
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
				// Fallback: search nested barrel maps if not found directly under this barrel path
				if !exists {
					candPrefixes := []string{filepath.ToSlash(resolvedPathKey)}
					aliasForKey := resolver.AliasPath(resolvedPathKey).FullPath
					if aliasForKey != "" {
						candPrefixes = append(candPrefixes, filepath.ToSlash(aliasForKey))
					}
					for k, v := range barrelResolvedPaths.ModuleResolverMap {
						kk := filepath.ToSlash(k)
						if strings.HasSuffix(kk, "/"+moduleName) {
							for _, pref := range candPrefixes {
								pp := filepath.ToSlash(pref)
								if strings.HasPrefix(kk, pp+"/") {
									resolvedModulePath = v
									exists = true
									break
								}
							}
							if exists {
								break
							}
						}
					}
				}
				var newImportPath string
				isDefaultReExport := false
				isRenameReExport := false
				renameSource := ""
				resolvedPathValue := resolvedModulePath
				if exists {
					// Try to resolve across nested barrels and chained re-exports (rename-as/default-as)
					if deepBase, deepType, deepSource, ok := resolveModuleChain(barrelResolvedPaths, resolver, resolvedPathKey, moduleName); ok {
						resolvedPathValue = deepBase
						if deepType == "default" {
							isDefaultReExport = true
							isRenameReExport = false
							renameSource = ""
						} else if deepType == "rename" {
							isDefaultReExport = false
							isRenameReExport = true
							if deepSource != "" {
								renameSource = deepSource
							}
						}
					} else {
						// Fallback: detect sentinels encoded by parser for a single hop
						if strings.HasPrefix(resolvedPathValue, "default|") {
							isDefaultReExport = true
							resolvedPathValue = strings.TrimPrefix(resolvedPathValue, "default|")
						} else if strings.HasPrefix(resolvedPathValue, "rename|") {
							// format: rename|Source|path
							parts := strings.SplitN(strings.TrimPrefix(resolvedPathValue, "rename|"), "|", 2)
							if len(parts) == 2 {
								renameSource = parts[0]
								resolvedPathValue = parts[1]
								isRenameReExport = true
							}
						}
					}
					// Build new path by appending the resolved subpath to the original specifier (alias or relative)
					base := resolvedPathValue
					// Ensure base is a pure subpath (no leading './', '../' or '/'), otherwise filepath.Join might drop the parent part or traverse up
					for {
						if strings.HasPrefix(base, "./") {
							base = strings.TrimPrefix(base, "./")
							continue
						}
						if strings.HasPrefix(base, "../") {
							base = strings.TrimPrefix(base, "../")
							continue
						}
						if strings.HasPrefix(base, "/") {
							base = strings.TrimPrefix(base, "/")
							continue
						}
						break
					}
					joined := joinCrossPlatformPaths(importPath, base)
					if origWasRelative {
						if !strings.HasPrefix(joined, "./") && !strings.HasPrefix(joined, "../") {
							joined = "./" + joined
						}
					}
					newImportPath = normalizeImportPath(joined, config.extensions)
					// If the original was alias and the joined path lost the alias prefix (unlikely), restore it
					if isAliasPath && !strings.HasPrefix(newImportPath, "@") && !strings.HasPrefix(newImportPath, "~") {
						// Fall back to using the resolved barrel directory as base
						alt := joinCrossPlatformPaths(resolvedPathKey, base)
						newImportPath = normalizeImportPath(alt, config.extensions)
					}
				} else {
					newImportPath = importPath
				}
				if _, exists := importsByModule[newImportPath]; !exists {
					orderedImportPaths = append(orderedImportPaths, newImportPath)
				}

				if exists && isDefaultReExport {
					// derive the local binding name (handle optional "as")
					localName := strings.TrimSpace(importName)
					if strings.HasPrefix(strings.ToLower(localName), "type ") {
						localName = strings.TrimSpace(localName[5:])
					}
					if idx := strings.Index(localName, " as "); idx != -1 {
						localName = strings.TrimSpace(localName[idx+4:])
					}
					// For default re-exports, emit as a named specifier using `default as <LocalName>`
					importsByModule[newImportPath] = append(importsByModule[newImportPath], fmt.Sprintf("default as %s", localName))
				} else if exists && isRenameReExport {
					// For rename-as, emit `Source as Alias`
					aliasName := strings.TrimSpace(importName)
					if strings.HasPrefix(strings.ToLower(aliasName), "type ") {
						aliasName = strings.TrimSpace(aliasName[5:])
					}
					if idx := strings.Index(aliasName, " as "); idx != -1 {
						aliasName = strings.TrimSpace(aliasName[idx+4:])
					}
					if renameSource == "" {
						renameSource = moduleName
					}
					importsByModule[newImportPath] = append(importsByModule[newImportPath], fmt.Sprintf("%s as %s", renameSource, aliasName))
				} else {
					importsByModule[newImportPath] = append(importsByModule[newImportPath], importName)
				}
			}

			for _, resolvedPath := range orderedImportPaths {
				importNames := importsByModule[resolvedPath]
				if len(importNames) == 0 {
					continue
				}
				newImportStatement := "import "
				isTypeImport := (matches[1] != "") || (len(importNames) == 1 && strings.Contains(importNames[0], "type "))
				if isTypeImport {
					newImportStatement += "type { "
				} else {
					newImportStatement += "{ "
				}

				for _, importName := range importNames {
					if isTypeImport {
						trimmed := strings.TrimSpace(importName)
						lower := strings.ToLower(trimmed)
						if strings.Contains(lower, "default as ") {
							newImportStatement += trimmed + ", "
						} else {
							class := getModuleName(importName)
							newImportStatement += class + ", "
						}
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
				// Ensure we don't accidentally glue the next line after our replacement
				// If the original import statement ended with a newline, keep one at the end
				if strings.HasSuffix(importStatement, "\n") && !strings.HasSuffix(replacedImportStatement, "\n") {
					replacedImportStatement += "\n"
				}
				if config.verbose {
					cmd.Printf("Updating imports in %s:\nBefore:\n%s\nAfter:\n%s\n\n", path, importStatement, replacedImportStatement)
				}
				return replacedImportStatement
			}

			return importStatement
		})

		if updatedContents != string(contents) {
			// Write with LF endings to match expected fixtures
			normUpd := strings.ReplaceAll(updatedContents, "\r\n", "\n")
			normUpd = strings.ReplaceAll(normUpd, "\r", "\n")
			os.WriteFile(path, []byte(normUpd), info.Mode())
			updatedFilesTotal += 1
		}

		return nil
	})

	// Second pass: normalize line endings to LF for all supported files to ensure
	// deterministic comparisons across platforms without affecting update count.
	isSupported := func(p string) bool {
		for _, ext := range config.extensions {
			if strings.HasSuffix(strings.ToLower(p), strings.ToLower(ext)) {
				return true
			}
		}
		return false
	}
	filepath.Walk(targetFullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !isSupported(path) {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		norm := strings.ReplaceAll(string(contents), "\r\n", "\n")
		norm = strings.ReplaceAll(norm, "\r", "\n")
		if norm != string(contents) {
			os.WriteFile(path, []byte(norm), info.Mode())
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

// resolveModuleChain follows chained re-exports across nested barrels.
// It returns the deepest accumulated subpath (relative to the starting barrel key),
// the kind of re-export ("default", "rename", or ""), and the source name for rename-as chains.
// ok == true indicates that at least one mapping hop was resolved.
func resolveModuleChain(b data.BarrelResolvedPath, r resolver.Resolver, startKey string, moduleName string) (finalSubpath string, kind string, source string, ok bool) {
	currentKey := startKey
	name := moduleName
	accum := ""
	visited := make(map[string]struct{})
	maxDepth := 20

	sanitize := func(p string) string {
		base := filepath.ToSlash(p)
		for {
			if strings.HasPrefix(base, "./") {
				base = strings.TrimPrefix(base, "./")
				continue
			}
			if strings.HasPrefix(base, "../") {
				base = strings.TrimPrefix(base, "../")
				continue
			}
			if strings.HasPrefix(base, "/") {
				base = strings.TrimPrefix(base, "/")
				continue
			}
			break
		}
		return base
	}

	for i := 0; i < maxDepth; i++ {
		key := filepath.Join(currentKey, name)
		if _, seen := visited[key]; seen {
			break
		}
		visited[key] = struct{}{}

		val, exists := b.ModuleResolverMap[key]
		if !exists {
			break
		}

		pathVal := val
		if strings.HasPrefix(val, "default|") {
			kind = "default"
			source = ""
			pathVal = strings.TrimPrefix(val, "default|")
		} else if strings.HasPrefix(val, "rename|") {
			parts := strings.SplitN(strings.TrimPrefix(val, "rename|"), "|", 2)
			if len(parts) == 2 {
				source = parts[0]
				kind = "rename"
				name = parts[0] // follow the source symbol name into the next barrel
				pathVal = parts[1]
			}
		} else {
			// plain named re-export; keep current name
		}

		pv := sanitize(pathVal)
		if pv == "" {
			break
		}
		if accum == "" {
			accum = pv
		} else {
			accum = filepath.ToSlash(filepath.Join(accum, pv))
		}

		nextKey := filepath.Join(currentKey, filepath.FromSlash(pv))
		// Always descend into the referenced path; on next iteration, if there is no mapping, we'll stop.
		currentKey = nextKey
		continue
	}

	if accum != "" {
		return accum, kind, source, true
	}
	return "", "", "", false
}
