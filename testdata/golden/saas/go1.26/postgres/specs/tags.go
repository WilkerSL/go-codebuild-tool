//go:build ignore

package specs

type Tag struct {
	ID    int64   `gen:"pk,auto"`
	Name  string  `gen:"required,notempty,unique,max=50"`
	Color *string `gen:"min=4,max=7"`
	Users []User  `gen:"rel=many_to_many:User,via=user_tags"`
}
