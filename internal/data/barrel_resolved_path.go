package data

import (
	"path/filepath"

	"github.com/nergie/no-barrel-file/internal/parser"
	"github.com/nergie/no-barrel-file/internal/resolver"
)

type BarrelResolvedPath struct {
	ExistenceMap      map[string]struct{}
	ModuleResolverMap map[string]parser.ModuleResolverMapValue
}

func NewBarrelResolvedPath(parser parser.Parser, resolver resolver.Resolver) BarrelResolvedPath {
	barrelPathExistenceMap, barrelModuleResolverMap := parser.BarrelMaps(resolver)
	return BarrelResolvedPath{
		ExistenceMap:      barrelPathExistenceMap,
		ModuleResolverMap: barrelModuleResolverMap,
	}
}

func (b *BarrelResolvedPath) IsResolved(path string) bool {
	_, exists := b.ExistenceMap[path]
	return exists
}

func (b *BarrelResolvedPath) ResolveModuleName(path string, moduleName string) (parser.ModuleResolverMapValue, bool) {
	resolvedPath, exists := b.ModuleResolverMap[filepath.Join(path, moduleName)]
	return resolvedPath, exists
}
