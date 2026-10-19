//go:build ignore

package specs

import "time"

type Audit struct {
	_         struct{}  `gen:"table=audit_log,noroutes"`
	ID        int64     `gen:"pk,auto"`
	Action    string    `gen:"required,notempty,max=64,index"`
	Payload   *string   `gen:"omit=response"`
	CreatedAt time.Time `gen:"auto=create,index"`
}
