//go:build spec

package specs

import (
	"time"
	"uuid"
)

type Author struct {
	_         struct{}  `gen:"softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Email     *string   `gen:"email,max=254"`
	Bio       *string   `gen:"max=2000"`
	Books     []Book    `gen:"rel=has_many:Book"`
	CreatedAt time.Time `gen:"auto=create"`
	UpdatedAt time.Time `gen:"auto=update"`
}
