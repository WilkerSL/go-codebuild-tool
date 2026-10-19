//go:build ignore

package specs

type InventoryService struct {
	_     struct{} `gen:"database=shared"`
	Stock struct {
		_         struct{} `gen:"roles=admin"`
		ID        int64    `gen:"pk,auto"`
		ProductID int64    `gen:"rel=belongs_to:Product,ondelete=cascade,unique"`
		Quantity  int32    `gen:"min=0"`
	}
}
