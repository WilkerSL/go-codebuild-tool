package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

var mergePatch = http.Header{"Content-Type": {"application/merge-patch+json"}}

// patchCheck changes one member of the item at path with a JSON merge patch
// and checks that the others keep their values, then exercises the errors: a
// null on a required member, an unknown member, a body that is not an object
// and another caller, who must not see the item.
func patchCheck(t *testing.T, owner, other client, path, member string, value any, required string) map[string]any {
	t.Helper()
	before := owner.expect(http.MethodGet, path, nil, http.StatusOK)
	resp, after := owner.send(http.MethodPatch, path, map[string]any{member: value}, mergePatch)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH %s %s = %d %v", path, member, resp.StatusCode, after)
	}
	for name, was := range before {
		if name == member || name == "updatedAt" {
			continue
		}
		if now := after[name]; !sameJSON(was, now) {
			t.Fatalf("PATCH %s %s changed %s from %v to %v", path, member, name, was, now)
		}
	}
	if resp, out := owner.send(http.MethodPatch, path, map[string]any{required: nil}, mergePatch); resp.StatusCode != http.StatusUnprocessableEntity || len(fieldErrors(out)) != 1 || fieldErrors(out)[0] != required {
		t.Fatalf("PATCH %s with %s null = %d %v", path, required, resp.StatusCode, out)
	}
	if resp, out := owner.send(http.MethodPatch, path, map[string]any{"noSuchMember": 1}, mergePatch); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH %s with an unknown member = %d %v", path, resp.StatusCode, out)
	}
	if resp, out := owner.send(http.MethodPatch, path, []any{1}, mergePatch); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH %s with an array = %d %v", path, resp.StatusCode, out)
	}
	if resp, out := other.send(http.MethodPatch, path, map[string]any{member: value}, mergePatch); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PATCH %s by another caller = %d %v", path, resp.StatusCode, out)
	}
	return after
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
