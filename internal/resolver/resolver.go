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

	if tsConfig.CompilerOptions.BaseUrl == "" {
		tsConfig.CompilerOptions.BaseUrl = filepath.Dir(tsConfigFullPath)
	}
	baseUrlPath := filepath.Dir(filepath.Join(tsConfigFullPath, tsConfig.CompilerOptions.BaseUrl))

	for alias, paths := range tsConfig.CompilerOptions.Paths {
		if len(paths) > 0 {
			for _, path := range paths {
				realPath := filepath.Join(baseUrlPath, filepath.Dir(path))
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
	for realPath, alias := range resolver.aliasPaths {
		if strings.HasPrefix(path, realPath) {
			fullPath := filepath.Join(alias, strings.TrimPrefix(path, realPath))
			return Alias{
				ShortPath: alias,
				FullPath:  fullPath,
			}
		}
	}

	return Alias{
		ShortPath: path,
		FullPath:  path,
	}
}

func (resolver *Resolver) IsAliasPath(path string) bool {
	return strings.HasPrefix(path, "@") || strings.HasPrefix(path, "~")
}
