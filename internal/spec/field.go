package spec

import (
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

var enumItemPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

var omitTargets = []string{"create", "response", "update"}

var onDeleteValues = []string{"cascade", "restrict", "setnull"}

type fieldBuilder struct {
	p      *parser
	f      *schema.Field
	set    markerSet
	failed bool
}

func (b *fieldBuilder) fail(key, code, suggestion, format string, args ...any) {
	b.failed = true
	pos := b.f.Pos
	if m, ok := b.set.byKey[key]; ok {
		pos = m.pos
	}
	b.p.errorf(pos, code, suggestion, format, args...)
}

func (b *fieldBuilder) kindAllowed(key string, allowed func(schema.Kind) bool, description string) bool {
	if allowed(b.f.Kind) {
		return true
	}
	b.fail(key, CodeInvalidCombination, "", "%s is only valid on %s fields, not on %s %s", key, description, b.f.Name, b.f.GoType())
	return false
}

func isStringOrBytes(k schema.Kind) bool {
	return k == schema.KindString || k == schema.KindBytes
}

func isUniqueKind(k schema.Kind) bool {
	return k == schema.KindString || k == schema.KindUUID || k.IsInteger()
}

func isIndexKind(k schema.Kind) bool {
	return isUniqueKind(k) || k == schema.KindBool || k == schema.KindTime
}

func isBoundKind(k schema.Kind) bool {
	return isStringOrBytes(k) || k.IsNumeric()
}

func isDefaultKind(k schema.Kind) bool {
	return k == schema.KindString || k == schema.KindBool || k.IsNumeric()
}

func isPKKind(k schema.Kind) bool {
	return k == schema.KindUUID || k == schema.KindString || k == schema.KindInt || k == schema.KindInt32 || k == schema.KindInt64
}

func (b *fieldBuilder) apply() {
	f, set := b.f, b.set
	for _, key := range []string{"fk", "via"} {
		if set.has(key) {
			kind := "has_many"
			if key == "via" {
				kind = "many_to_many"
			}
			b.fail(key, CodeInvalidCombination, "", "%s is only valid with rel=%s on a []Entity field", key, kind)
		}
	}
	if set.has("rel") {
		b.applyRel()
	}
	if set.has("pk") {
		if b.kindAllowed("pk", isPKKind, "uuid.UUID, string, int, int32 or int64") {
			if f.Nullable {
				b.fail("pk", CodeInvalidCombination, "", "primary key %s cannot be nullable; remove the *", f.Name)
			}
			f.PK = true
		}
	}
	if set.has("auto") {
		b.applyAuto()
	}
	if set.has("required") {
		if f.Nullable {
			b.fail("required", CodeInvalidCombination, "", "required cannot be used on nullable field %s; remove the * or the marker", f.Name)
		} else {
			f.Required = true
		}
	}
	if set.has("notempty") && b.kindAllowed("notempty", isStringOrBytes, "string or []byte") {
		f.NotEmpty = true
	}
	if set.has("unique") && b.kindAllowed("unique", isUniqueKind, "string, integer or uuid.UUID") {
		f.Unique = true
	}
	if set.has("index") && b.kindAllowed("index", isIndexKind, "string, integer, bool, uuid.UUID or time.Time") {
		f.Index = true
	}
	b.applyBounds()
	if set.has("email") && b.kindAllowed("email", func(k schema.Kind) bool { return k == schema.KindString }, "string") {
		f.Email = true
	}
	if set.has("enum") {
		b.applyEnum()
	}
	if set.has("omit") {
		b.applyOmit()
	}
	if set.has("column") {
		m := set.get("column")
		if !naming.IsSQLIdentifier(m.value) {
			b.fail("column", CodeInvalidValue, sqlIdentifierSuggestion(m.value), "column name %q must match ^[a-z_][a-z0-9_]*$ and have at most 63 characters", m.value)
		} else {
			f.Column = m.value
		}
	}
	if set.has("ondelete") {
		b.applyOnDelete()
	}
	if set.has("default") && b.kindAllowed("default", isDefaultKind, "string, bool, integer or float") {
		f.HasDefault = true
		f.Default = set.get("default").value
	}
	b.checkCombinations()
	if f.HasDefault && !b.failed {
		b.applyDefault()
	}
	if f.Ref != nil && !f.Nullable && !f.OmitCreate {
		f.Required = true
	}
	if f.PK && !f.AutoPK {
		f.Required = true
	}
}

func (b *fieldBuilder) applyRel() {
	m := b.set.get("rel")
	kind, target, ok := b.p.parseRel(m)
	if !ok {
		b.failed = true
		return
	}
	if kind != "belongs_to" {
		b.fail("rel", CodeRelationTypeMismatch, "", "%s requires a field of type []%s", kind, target)
		return
	}
	b.f.Ref = &schema.Reference{Entity: target, OnDelete: schema.OnDeleteRestrict}
}

func (b *fieldBuilder) applyAuto() {
	f, m := b.f, b.set.get("auto")
	if !m.hasValue {
		switch {
		case !b.set.has("pk"):
			b.fail("auto", CodeInvalidCombination, "auto=create", "auto without a value is only valid on the primary key")
		case f.Kind == schema.KindUUID || f.Kind.IsInteger():
			f.AutoPK = true
		default:
			b.fail("auto", CodeInvalidCombination, "", "auto is only supported on uuid.UUID and integer primary keys; string keys are sent by the client")
		}
		return
	}
	switch m.value {
	case "create", "update":
	default:
		b.fail("auto", CodeInvalidValue, naming.Suggest(m.value, []string{"create", "update"}), "auto accepts no value, create or update, not %q", m.value)
		return
	}
	if f.Kind != schema.KindTime || f.Nullable {
		b.fail("auto", CodeInvalidCombination, "", "auto=%s is only valid on time.Time fields, not on %s %s", m.value, f.Name, f.GoType())
		return
	}
	if b.set.has("pk") {
		b.fail("auto", CodeInvalidCombination, "auto", "auto=%s cannot be used on the primary key", m.value)
		return
	}
	f.AutoCreate = m.value == "create"
	f.AutoUpdate = m.value == "update"
}

func (b *fieldBuilder) applyBounds() {
	f := b.f
	var minV, maxV float64
	var hasMin, hasMax bool
	for _, key := range []string{"min", "max"} {
		if !b.set.has(key) || !b.kindAllowed(key, isBoundKind, "string, []byte, integer or float") {
			continue
		}
		m := b.set.get(key)
		normalized, value, ok := b.parseBound(m.value)
		if !ok {
			b.fail(key, CodeInvalidValue, "", "%s=%s is not valid for %s; %s", key, m.value, f.GoType(), b.boundHint())
			continue
		}
		if key == "min" {
			f.Min, minV, hasMin = normalized, value, true
		} else {
			f.Max, maxV, hasMax = normalized, value, true
		}
	}
	if hasMin && hasMax && minV > maxV {
		b.fail("min", CodeInvalidValue, "", "min=%s cannot be greater than max=%s", f.Min, f.Max)
	}
	if hasMax && isStringOrBytes(f.Kind) && maxV < 1 {
		b.fail("max", CodeInvalidValue, "", "max must be at least 1 for %s", f.GoType())
	}
}

func (b *fieldBuilder) boundHint() string {
	switch {
	case isStringOrBytes(b.f.Kind):
		return "use a non-negative integer length"
	case b.f.Kind.IsInteger():
		return fmt.Sprintf("use an integer that fits in %s", b.f.Kind.GoType())
	default:
		return "use a number"
	}
}

func (b *fieldBuilder) parseBound(s string) (string, float64, bool) {
	switch {
	case isStringOrBytes(b.f.Kind):
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil || n < 0 {
			return "", 0, false
		}
		return strconv.FormatInt(n, 10), float64(n), true
	case b.f.Kind.IsInteger():
		n, err := strconv.ParseInt(s, 10, b.f.Kind.BitSize())
		if err != nil {
			return "", 0, false
		}
		return strconv.FormatInt(n, 10), float64(n), true
	default:
		v, err := strconv.ParseFloat(s, b.f.Kind.BitSize())
		if err != nil {
			return "", 0, false
		}
		return strconv.FormatFloat(v, 'g', -1, b.f.Kind.BitSize()), v, true
	}
}

func (b *fieldBuilder) applyEnum() {
	f := b.f
	if !b.kindAllowed("enum", func(k schema.Kind) bool { return k == schema.KindString }, "string") {
		return
	}
	m := b.set.get("enum")
	items := strings.Split(m.value, "|")
	seen := map[string]bool{}
	for _, item := range items {
		switch {
		case item == "":
			b.fail("enum", CodeInvalidValue, "", "enum items cannot be empty")
			return
		case !enumItemPattern.MatchString(item):
			b.fail("enum", CodeInvalidValue, "", "enum item %q may only contain letters, digits, '_', '-' and '.'", item)
			return
		case seen[item]:
			b.fail("enum", CodeInvalidValue, "", "enum item %q is listed more than once", item)
			return
		}
		seen[item] = true
	}
	if f.Max != "" {
		limit, _ := strconv.Atoi(f.Max)
		for _, item := range items {
			if utf8.RuneCountInString(item) > limit {
				b.fail("enum", CodeInvalidValue, "", "enum item %q is longer than max=%s", item, f.Max)
				return
			}
		}
	}
	if f.Min != "" {
		limit, _ := strconv.Atoi(f.Min)
		for _, item := range items {
			if utf8.RuneCountInString(item) < limit {
				b.fail("enum", CodeInvalidValue, "", "enum item %q is shorter than min=%s", item, f.Min)
				return
			}
		}
	}
	f.Enum = items
}

func (b *fieldBuilder) applyOmit() {
	m := b.set.get("omit")
	seen := map[string]bool{}
	for _, target := range strings.Split(m.value, "|") {
		if !slices.Contains(omitTargets, target) {
			b.fail("omit", CodeInvalidValue, naming.Suggest(target, omitTargets), "unknown omit target %q; use create, update or response", target)
			return
		}
		if seen[target] {
			b.fail("omit", CodeInvalidValue, "", "omit target %q is listed more than once", target)
			return
		}
		seen[target] = true
	}
	b.f.OmitCreate = seen["create"]
	b.f.OmitUpdate = seen["update"]
	b.f.OmitResponse = seen["response"]
}

func (b *fieldBuilder) applyOnDelete() {
	m := b.set.get("ondelete")
	if b.f.Ref == nil {
		b.fail("ondelete", CodeInvalidCombination, "", "ondelete is only valid together with rel=belongs_to")
		return
	}
	if !slices.Contains(onDeleteValues, m.value) {
		b.fail("ondelete", CodeInvalidValue, naming.Suggest(m.value, onDeleteValues), "unknown ondelete action %q; use restrict, cascade or setnull", m.value)
		return
	}
	if m.value == "setnull" && !b.f.Nullable {
		b.fail("ondelete", CodeInvalidCombination, "", "ondelete=setnull requires a nullable field; declare %s as *%s", b.f.Name, b.f.Kind.GoType())
		return
	}
	b.f.Ref.OnDelete = schema.OnDelete(m.value)
}

func (b *fieldBuilder) conflict(a, c string, reason string) {
	if b.set.has(a) && b.set.has(c) {
		b.fail(c, CodeInvalidCombination, "", "%s cannot be combined with %s: %s", c, a, reason)
	}
}

func (b *fieldBuilder) checkCombinations() {
	f := b.f
	b.conflict("pk", "rel", "a primary key cannot be a foreign key in this version")
	b.conflict("pk", "default", "primary keys never use defaults")
	b.conflict("pk", "unique", "the primary key is already unique")
	b.conflict("pk", "index", "the primary key is already indexed")
	b.conflict("unique", "index", "a unique index already indexes the column")
	b.conflict("required", "default", "a required member is never absent, so the default would never apply")
	b.conflict("email", "enum", "a value cannot be both an email address and an enum item")
	b.conflict("rel", "default", "foreign keys do not support defaults")
	b.conflict("rel", "enum", "foreign keys cannot be enums")
	if f.PK && f.OmitResponse {
		b.fail("omit", CodeInvalidCombination, "", "the primary key cannot be omitted from responses")
	}
	if f.PK && !f.AutoPK && f.OmitCreate {
		b.fail("omit", CodeInvalidCombination, "", "a client-provided primary key cannot be omitted from create; add auto or remove omit=create")
	}
	if f.AutoPK && f.Required {
		b.fail("required", CodeInvalidCombination, "", "auto primary keys are generated and cannot be required")
	}
	if f.AutoCreate || f.AutoUpdate {
		for _, key := range []string{"required", "default", "omit", "unique", "rel", "pk"} {
			if b.set.has(key) {
				b.fail(key, CodeInvalidCombination, "", "%s cannot be combined with auto=%s; the database sets the value", key, b.set.get("auto").value)
			}
		}
	}
	if f.Required && f.OmitCreate {
		b.fail("omit", CodeInvalidCombination, "", "required field %s cannot be omitted from create", f.Name)
	}
	if f.Ref != nil && !f.Nullable && f.OmitCreate {
		b.fail("omit", CodeInvalidCombination, "", "non-nullable reference %s must be sent on create; make it a pointer or remove omit=create", f.Name)
	}
	if f.Email && f.Max != "" {
		if limit, _ := strconv.Atoi(f.Max); limit < 6 {
			b.fail("max", CodeInvalidValue, "", "max must be at least 6 for email fields")
		}
	}
}

func (b *fieldBuilder) applyDefault() {
	f := b.f
	raw := f.Default
	switch {
	case f.Kind == schema.KindString:
		if len(f.Enum) > 0 && !slices.Contains(f.Enum, raw) {
			b.fail("default", CodeInvalidValue, naming.Suggest(raw, f.Enum), "default %q is not one of the enum items %s", raw, strings.Join(f.Enum, ", "))
			return
		}
		n := utf8.RuneCountInString(raw)
		if f.Min != "" {
			if limit, _ := strconv.Atoi(f.Min); n < limit {
				b.fail("default", CodeInvalidValue, "", "default %q is shorter than min=%s", raw, f.Min)
				return
			}
		}
		if f.Max != "" {
			if limit, _ := strconv.Atoi(f.Max); n > limit {
				b.fail("default", CodeInvalidValue, "", "default %q is longer than max=%s", raw, f.Max)
				return
			}
		}
		if f.NotEmpty && strings.TrimSpace(raw) == "" {
			b.fail("default", CodeInvalidValue, "", "default cannot be empty because the field is notempty")
			return
		}
		if f.Email {
			if addr, err := mail.ParseAddress(raw); err != nil || addr.Address != raw || addr.Name != "" {
				b.fail("default", CodeInvalidValue, "", "default %q is not a valid email address", raw)
				return
			}
		}
	case f.Kind == schema.KindBool:
		if raw != "true" && raw != "false" {
			b.fail("default", CodeInvalidValue, naming.Suggest(raw, []string{"false", "true"}), "default for bool must be true or false, not %q", raw)
			return
		}
	case f.Kind.IsInteger():
		n, err := strconv.ParseInt(raw, 10, f.Kind.BitSize())
		if err != nil {
			b.fail("default", CodeInvalidValue, "", "default %q is not a valid %s", raw, f.Kind.GoType())
			return
		}
		if !b.withinBounds(float64(n)) {
			return
		}
		f.Default = strconv.FormatInt(n, 10)
	case f.Kind.IsFloat():
		v, err := strconv.ParseFloat(raw, f.Kind.BitSize())
		if err != nil {
			b.fail("default", CodeInvalidValue, "", "default %q is not a valid %s", raw, f.Kind.GoType())
			return
		}
		if !b.withinBounds(v) {
			return
		}
		f.Default = strconv.FormatFloat(v, 'g', -1, f.Kind.BitSize())
	}
}

func (b *fieldBuilder) withinBounds(v float64) bool {
	f := b.f
	if f.Min != "" {
		if limit, _ := strconv.ParseFloat(f.Min, 64); v < limit {
			b.fail("default", CodeInvalidValue, "", "default %s is less than min=%s", f.Default, f.Min)
			return false
		}
	}
	if f.Max != "" {
		if limit, _ := strconv.ParseFloat(f.Max, 64); v > limit {
			b.fail("default", CodeInvalidValue, "", "default %s is greater than max=%s", f.Default, f.Max)
			return false
		}
	}
	return true
}
