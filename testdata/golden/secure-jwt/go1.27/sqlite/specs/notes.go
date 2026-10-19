//go:build spec

package specs

import (
	"time"

	"github.com/google/uuid"
)

type Note struct {
	_         struct{}  `gen:"softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	OwnerID   uuid.UUID `gen:"rel=belongs_to:User,ondelete=cascade,owner"`
	FolderID  *int64    `gen:"rel=belongs_to:Folder"`
	Title     string    `gen:"required,notempty,max=200"`
	Body      *string   `gen:"max=5000"`
	Pinned    bool      `gen:"default=false,index"`
	CreatedAt time.Time `gen:"auto=create"`
	UpdatedAt time.Time `gen:"auto=update"`
	Labels    []Label   `gen:"rel=many_to_many:Label,via=note_labels"`
}
