package schema

// IdempotencyTable keeps the answers to requests sent with an Idempotency-Key
// header, so a client that retries a create after a lost answer gets the
// first answer back instead of a second record. Every database with entities
// has one.
const IdempotencyTable = "idempotency_keys"

const IdempotencyLocationSize = 2048

func idempotencyTable() *Table {
	t := &Table{Name: IdempotencyTable}
	t.Columns = []*Column{
		{Name: "key_hash", Kind: ColumnString, Size: TokenHashSize},
		{Name: "request_hash", Kind: ColumnString, Size: TokenHashSize},
		{Name: "status", Kind: ColumnInt32},
		{Name: "location", Kind: ColumnString, Size: IdempotencyLocationSize, Nullable: true},
		{Name: "body", Kind: ColumnBytes, Nullable: true},
		{Name: "created_at", Kind: ColumnTime},
		{Name: "expires_at", Kind: ColumnTime},
	}
	t.PrimaryKey = &PrimaryKey{Name: ConstraintName("pk", t.Name), Columns: []string{"key_hash"}}
	t.Indexes = []*Index{{Name: ConstraintName("ix", t.Name, "expires_at"), Columns: []string{"expires_at"}}}
	sortTable(t)
	return t
}
