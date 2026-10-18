//go:build spec

package specs

import (
	"time"
	"uuid"
)

type Loan struct {
	ID         int64      `gen:"pk,auto"`
	BookID     uuid.UUID  `gen:"rel=belongs_to:Book,ondelete=cascade"`
	Borrower   string     `gen:"required,email,max=254,index"`
	DueAt      time.Time  `gen:"required"`
	ReturnedAt *time.Time `gen:"omit=create"`
	CreatedAt  time.Time  `gen:"auto=create"`
}
