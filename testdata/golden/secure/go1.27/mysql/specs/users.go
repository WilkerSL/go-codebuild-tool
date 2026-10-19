//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	_            struct{}  `gen:"auth=session|apikey,register,roles=admin,softdelete"`
	ID           uuid.UUID `gen:"pk,auto"`
	Email        string    `gen:"required,unique,email,max=254,identity"`
	Name         string    `gen:"required,notempty,max=120"`
	PasswordHash string    `gen:"password,min=10"`
	Role         string    `gen:"enum=admin|member,default=member,role"`
	CreatedAt    time.Time `gen:"auto=create"`
	UpdatedAt    time.Time `gen:"auto=update"`
}
