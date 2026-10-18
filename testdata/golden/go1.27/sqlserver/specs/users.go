//go:build spec

package specs

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	_         struct{}  `gen:"table=users,softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Email     string    `gen:"required,unique,email,max=254"`
	Role      string    `gen:"enum=admin|operator|viewer,default=viewer"`
	Bio       *string   `gen:"max=500"`
	OrgID     uuid.UUID `gen:"rel=belongs_to:Org"`
	CreatedAt time.Time `gen:"auto=create"`
	UpdatedAt time.Time `gen:"auto=update"`
	Tags      []Tag     `gen:"rel=many_to_many:Tag,via=user_tags"`
}
