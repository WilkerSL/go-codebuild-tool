//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type Reminder struct {
	ID     int64     `gen:"pk,auto"`
	NoteID uuid.UUID `gen:"rel=belongs_to:Note,ondelete=cascade"`
	Text   string    `gen:"required,notempty,max=200"`
	DueAt  time.Time `gen:"required"`
	Done   bool      `gen:"default=false"`
}
