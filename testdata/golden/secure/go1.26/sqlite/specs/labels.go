//go:build ignore

package specs

type Label struct {
	_     struct{} `gen:"public=read,writeroles=admin"`
	ID    int64    `gen:"pk,auto"`
	Name  string   `gen:"required,notempty,unique,max=40"`
	Notes []Note   `gen:"rel=many_to_many:Note,via=note_labels"`
}
