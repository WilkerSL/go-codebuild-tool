//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type Org struct {
	_         struct{}  `gen:"softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Slug      string    `gen:"required,unique,max=64"`
	Plan      string    `gen:"enum=free|pro|enterprise,default=free,index"`
	Active    bool      `gen:"default=true"`
	Locale    string    `gen:"default=en-US"`
	CreatedAt time.Time `gen:"auto=create"`
	UpdatedAt time.Time `gen:"auto=update"`
	Users     []User    `gen:"rel=has_many:User"`
	Projects  []Project `gen:"rel=has_many:Project"`
}
