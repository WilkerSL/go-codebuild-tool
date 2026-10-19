//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID        uuid.UUID  `gen:"pk,auto"`
	OrgID     uuid.UUID  `gen:"rel=belongs_to:Org,ondelete=cascade"`
	OwnerID   *uuid.UUID `gen:"rel=belongs_to:User,ondelete=setnull"`
	Name      string     `gen:"required,notempty,max=200"`
	Budget    float64    `gen:"min=0"`
	Priority  int32      `gen:"min=1,max=5,default=3"`
	Archived  bool       `gen:"index"`
	Logo      []byte     `gen:"max=65536"`
	DueAt     *time.Time
	CreatedAt time.Time `gen:"auto=create"`
}
