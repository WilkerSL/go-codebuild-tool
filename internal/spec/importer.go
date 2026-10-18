package spec

import (
	"go/token"
	"go/types"
	"path"
)

const (
	importTime       = "time"
	importStdUUID    = "uuid"
	importGoogleUUID = "github.com/google/uuid"
)

var allowedImports = []string{importGoogleUUID, importTime, importStdUUID}

type syntheticImporter struct {
	packages map[string]*types.Package
}

func newSyntheticImporter() *syntheticImporter {
	return &syntheticImporter{
		packages: map[string]*types.Package{
			importTime:       newSyntheticPackage(importTime, "time", "Time", types.NewStruct(nil, nil)),
			importStdUUID:    newSyntheticPackage(importStdUUID, "uuid", "UUID", types.NewArray(types.Typ[types.Byte], 16)),
			importGoogleUUID: newSyntheticPackage(importGoogleUUID, "uuid", "UUID", types.NewArray(types.Typ[types.Byte], 16)),
		},
	}
}

func newSyntheticPackage(importPath, name, typeName string, underlying types.Type) *types.Package {
	pkg := types.NewPackage(importPath, name)
	obj := types.NewTypeName(token.NoPos, pkg, typeName, nil)
	types.NewNamed(obj, underlying, nil)
	pkg.Scope().Insert(obj)
	pkg.MarkComplete()
	return pkg
}

func (im *syntheticImporter) Import(importPath string) (*types.Package, error) {
	if pkg, ok := im.packages[importPath]; ok {
		return pkg, nil
	}
	pkg := types.NewPackage(importPath, path.Base(importPath))
	pkg.MarkComplete()
	im.packages[importPath] = pkg
	return pkg, nil
}
