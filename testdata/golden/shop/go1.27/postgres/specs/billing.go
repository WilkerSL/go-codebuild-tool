//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type BillingService struct {
	_       struct{} `gen:"database=own"`
	Invoice struct {
		_         struct{}      `gen:"softdelete"`
		ID        uuid.UUID     `gen:"pk,auto"`
		OwnerID   uuid.UUID     `gen:"rel=belongs_to:User,ondelete=cascade,owner"`
		Number    string        `gen:"required,unique,max=32"`
		Status    string        `gen:"enum=draft|issued|paid,default=draft,index"`
		Notes     *string       `gen:"max=500"`
		CreatedAt time.Time     `gen:"auto=create"`
		Items     []InvoiceItem `gen:"rel=has_many:InvoiceItem"`
	}
	InvoiceItem struct {
		ID        int64     `gen:"pk,auto"`
		InvoiceID uuid.UUID `gen:"rel=belongs_to:Invoice,ondelete=cascade"`
		ProductID int64     `gen:"rel=belongs_to:Product"`
		Quantity  int32     `gen:"min=1,default=1"`
		Price     float64   `gen:"min=0"`
	}
}
