//go:build spec

package specs

type Country struct {
	_    struct{} `gen:"readonly"`
	Code string   `gen:"pk,min=2,max=2"`
	Name string   `gen:"required,notempty,max=100"`
}
