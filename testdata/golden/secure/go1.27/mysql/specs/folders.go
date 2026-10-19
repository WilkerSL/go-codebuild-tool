//go:build ignore

package specs

import "github.com/google/uuid"

type Folder struct {
	ID      int64     `gen:"pk,auto"`
	OwnerID uuid.UUID `gen:"rel=belongs_to:User,ondelete=cascade,owner"`
	Name    string    `gen:"required,notempty,max=80"`
	Notes   []Note    `gen:"rel=has_many:Note"`
}
