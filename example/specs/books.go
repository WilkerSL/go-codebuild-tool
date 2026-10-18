//go:build spec

package specs

import (
	"time"
	"uuid"
)

type Book struct {
	_            struct{}  `gen:"softdelete"`
	ID           uuid.UUID `gen:"pk,auto"`
	AuthorID     uuid.UUID `gen:"rel=belongs_to:Author,ondelete=cascade"`
	LanguageCode *string   `gen:"rel=belongs_to:Language,max=2"`
	Title        string    `gen:"required,notempty,max=200"`
	ISBN         string    `gen:"required,unique,min=10,max=13"`
	Genre        string    `gen:"enum=fiction|nonfiction|science|history,default=fiction,index"`
	Pages        int32     `gen:"min=1,max=20000"`
	Price        float64   `gen:"min=0"`
	Available    bool      `gen:"default=true,index"`
	PublishedAt  *time.Time
	Tags         []Tag     `gen:"rel=many_to_many:Tag,via=book_tags"`
	Loans        []Loan    `gen:"rel=has_many:Loan"`
	CreatedAt    time.Time `gen:"auto=create"`
	UpdatedAt    time.Time `gen:"auto=update"`
}
