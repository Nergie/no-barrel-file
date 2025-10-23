package resolver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

type Resolver struct {
	aliasPaths map[string]string
	rootPath   string
}

func New(rootPath string, tsConfigPath *string) Resolver {
	return Resolver{
		aliasPaths: getAliasPaths(rootPath, tsConfigPath),
		rootPath:   rootPath,
	}
}

type TSConfig struct {
	CompilerOptions struct {
		Paths   map[string][]string `json:"paths"`
		BaseUrl string              `json:"baseUrl"`
	} `json:"compilerOptions"`
}

func getAliasPaths(rootPath string, tsConfigPath *string) map[string]string {
	aliasPaths := make(map[string]string)
	if tsConfigPath == nil || *tsConfigPath == "" {
		return aliasPaths
	}
	tsConfigFullPath := filepath.Join(rootPath, *tsConfigPath)
	file, err := os.ReadFile(tsConfigFullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening tsconfig file: %v\n", err)
		return nil
	}

	b, err := hujson.Standardize(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing tsconfig: %v\n", err)
		fmt.Fprintf(os.Stderr, "Ignoring tsconfig file\n")
	}

	var tsConfig TSConfig
	if err := json.Unmarshal(b, &tsConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing tsconfig: %v\n", err)
		fmt.Fprintf(os.Stderr, "Ignoring tsconfig file\n")
		return nil
	}

	// Determine baseUrlPath relative to the tsconfig directory
	cfgDir := filepath.Dir(tsConfigFullPath)
	var baseUrlPath string
	if tsConfig.CompilerOptions.BaseUrl == "" {
		baseUrlPath = cfgDir
	} else {
		baseUrlPath = filepath.Join(cfgDir, tsConfig.CompilerOptions.BaseUrl)
	}

	for alias, paths := range tsConfig.CompilerOptions.Paths {
		if len(paths) > 0 {
			for _, path := range paths {
				realPath := filepath.ToSlash(filepath.Join(baseUrlPath, filepath.Dir(path)))
				aliasPaths[realPath] = strings.TrimSuffix(alias, "/*")
			}
		}
	}

	return aliasPaths
}

type Alias struct {
	ShortPath string
	FullPath  string
}

func (resolver *Resolver) AliasPath(path string) Alias {
	p := filepath.ToSlash(path)
	for realPath, alias := range resolver.aliasPaths {
		rp := filepath.ToSlash(realPath)
		if p == rp || strings.HasPrefix(p, rp+"/") {
			remainder := strings.TrimPrefix(p, rp)
			remainder = strings.TrimPrefix(remainder, "/")
			fullPath := filepath.ToSlash(filepath.Join(alias, remainder))
			return Alias{
				ShortPath: alias,
				FullPath:  fullPath,
			}
		}
	}

	return Alias{
		ShortPath: p,
		FullPath:  p,
	}
}

func (resolver *Resolver) IsAliasPath(path string) bool {
	if strings.HasPrefix(path, "@") || strings.HasPrefix(path, "~") {
		return true
	}
	for _, alias := range resolver.aliasPaths {
		if alias == "" {
			continue
		}
		if path == alias || strings.HasPrefix(path, alias+"/") {
			return true
		}
	}
	return false
}

// ResolveAliasImport resolves an alias import path (like "ui/components/Button")
// to its real filesystem path based on tsconfig paths. Returns the real path and true on success.
func (resolver *Resolver) ResolveAliasImport(importPath string) (string, bool) {
	for realPath, alias := range resolver.aliasPaths {
		if alias == "" {
			continue
		}
		if importPath == alias || strings.HasPrefix(importPath, alias+"/") {
			remainder := strings.TrimPrefix(importPath, alias)
			remainder = strings.TrimPrefix(remainder, "/")
			full := filepath.ToSlash(filepath.Join(filepath.ToSlash(realPath), remainder))
			return filepath.ToSlash(full), true
		}
	}
	return "", false
}
