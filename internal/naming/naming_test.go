package naming

import (
	"reflect"
	"testing"
)

func TestWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"User", []string{"User"}},
		{"OrgID", []string{"Org", "ID"}},
		{"HTTPStatus", []string{"HTTP", "Status"}},
		{"UserIDs", []string{"User", "IDs"}},
		{"CreatedAt", []string{"Created", "At"}},
		{"order_items", []string{"order", "items"}},
		{"read-only", []string{"read", "only"}},
		{"Address2", []string{"Address2"}},
		{"ID", []string{"ID"}},
	}
	for _, tt := range tests {
		if got := Words(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Words(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCaseConversions(t *testing.T) {
	tests := []struct {
		in, snake, kebab, lowerCamel, pascal string
	}{
		{"OrgID", "org_id", "org-id", "orgId", "OrgID"},
		{"HTTPStatus", "http_status", "http-status", "httpStatus", "HTTPStatus"},
		{"ID", "id", "id", "id", "ID"},
		{"UserIDs", "user_ids", "user-ids", "userIds", "UserIds"},
		{"in_progress", "in_progress", "in-progress", "inProgress", "InProgress"},
		{"api", "api", "api", "api", "API"},
	}
	for _, tt := range tests {
		if got := Snake(tt.in); got != tt.snake {
			t.Errorf("Snake(%q) = %q, want %q", tt.in, got, tt.snake)
		}
		if got := Kebab(tt.in); got != tt.kebab {
			t.Errorf("Kebab(%q) = %q, want %q", tt.in, got, tt.kebab)
		}
		if got := LowerCamel(tt.in); got != tt.lowerCamel {
			t.Errorf("LowerCamel(%q) = %q, want %q", tt.in, got, tt.lowerCamel)
		}
		if got := Pascal(tt.in); got != tt.pascal {
			t.Errorf("Pascal(%q) = %q, want %q", tt.in, got, tt.pascal)
		}
	}
}

func TestPlural(t *testing.T) {
	tests := map[string]string{
		"user":       "users",
		"category":   "categories",
		"address":    "addresses",
		"box":        "boxes",
		"key":        "keys",
		"person":     "people",
		"order_item": "order_items",
		"branch":     "branches",
		"metadata":   "metadata",
		"shelf":      "shelves",
		"book_shelf": "book_shelves",
		"analysis":   "analyses",
		"quiz":       "quizzes",
		"roof":       "roofs",
		"status":     "statuses",
		"bus":        "buses",
		"photo":      "photos",
	}
	for in, want := range tests {
		if got := Plural(in); got != want {
			t.Errorf("Plural(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuggest(t *testing.T) {
	candidates := []string{"required", "notempty", "unique", "index", "default", "email"}
	tests := map[string]string{
		"requird":   "required",
		"reuqired":  "required",
		"uniqe":     "unique",
		"Index":     "index",
		"defualt":   "default",
		"something": "",
		"x":         "",
	}
	for in, want := range tests {
		if got := Suggest(in, candidates); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSQLIdentifier(t *testing.T) {
	valid := []string{"users", "user_tags", "_x", "a1"}
	invalid := []string{"", "Users", "1a", "user-tags", "a b", string(make([]byte, 64))}
	for _, s := range valid {
		if !IsSQLIdentifier(s) {
			t.Errorf("IsSQLIdentifier(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if IsSQLIdentifier(s) {
			t.Errorf("IsSQLIdentifier(%q) = true, want false", s)
		}
	}
}
