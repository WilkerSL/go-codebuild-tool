//go:build ignore

package specs

type CatalogService struct {
	_        struct{} `gen:"database=shared"`
	Category struct {
		_        struct{}  `gen:"public=read,writeroles=admin"`
		ID       int64     `gen:"pk,auto"`
		Name     string    `gen:"required,notempty,unique,max=80"`
		Products []Product `gen:"rel=has_many:Product"`
	}
	Product struct {
		_          struct{} `gen:"public=read,writeroles=admin,softdelete"`
		ID         int64    `gen:"pk,auto"`
		CategoryID *int64   `gen:"rel=belongs_to:Category,ondelete=setnull"`
		Name       string   `gen:"required,notempty,max=120"`
		Price      float64  `gen:"min=0"`
	}
}
