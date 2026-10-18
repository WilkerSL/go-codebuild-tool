//go:build spec

package specs

type Tag struct {
	ID    int64  `gen:"pk,auto"`
	Name  string `gen:"required,notempty,unique,max=40"`
	Books []Book `gen:"rel=many_to_many:Book,via=book_tags"`
}
