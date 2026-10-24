package dialect

import "github.com/WilkerSL/go-codebuild-tool/internal/schema"

type IdempotencyQueries struct {
	Claim    string
	Lookup   string
	Retake   string
	Complete string
	Release  string
	Purge    string
}

// BuildIdempotencyQueries writes the statements of the idempotency key store.
// Every placeholder has its own key, so the arguments come in the same order
// in every dialect, positional or not.
func BuildIdempotencyQueries(d Dialect) IdempotencyQueries {
	var q IdempotencyQueries
	table := d.Quote(schema.IdempotencyTable)
	col := d.Quote
	p := newParams(d)
	q.Claim = "INSERT INTO " + table + " (" + col("key_hash") + ", " + col("request_hash") + ", " + col("status") + ", " + col("created_at") + ", " + col("expires_at") + ")\nVALUES (" +
		p.next("key") + ", " + p.next("request") + ", 0, " + p.next("now") + ", " + p.next("expires") + ")"
	p = newParams(d)
	q.Lookup = "SELECT " + col("request_hash") + ", " + col("status") + ", " + col("location") + ", " + col("body") + ", " + col("created_at") + ", " + col("expires_at") +
		"\nFROM " + table + "\nWHERE " + col("key_hash") + " = " + p.next("key")
	p = newParams(d)
	q.Retake = "UPDATE " + table + "\nSET " + col("request_hash") + " = " + p.next("request") + ", " + col("status") + " = 0, " + col("location") + " = NULL, " + col("body") + " = NULL, " +
		col("created_at") + " = " + p.next("now") + ", " + col("expires_at") + " = " + p.next("expires") +
		"\nWHERE " + col("key_hash") + " = " + p.next("key") + " AND ((" + col("status") + " = 0 AND " + col("created_at") + " <= " + p.next("stale") + ") OR " + col("expires_at") + " <= " + p.next("expired") + ")"
	p = newParams(d)
	q.Complete = "UPDATE " + table + "\nSET " + col("status") + " = " + p.next("status") + ", " + col("location") + " = " + p.next("location") + ", " + col("body") + " = " + p.next("body") +
		"\nWHERE " + col("key_hash") + " = " + p.next("key") + " AND " + col("status") + " = 0"
	p = newParams(d)
	q.Release = "DELETE FROM " + table + "\nWHERE " + col("key_hash") + " = " + p.next("key") + " AND " + col("status") + " = 0"
	p = newParams(d)
	q.Purge = batchDelete(d, schema.IdempotencyTable, "key_hash", col("expires_at")+" <= "+p.next("before"))
	return q
}

// batchDelete deletes at most 1,000 matching rows, so a cleanup never holds a
// long lock on the table.
func batchDelete(d Dialect, table, key, where string) string {
	const size = "1000"
	t := d.Quote(table)
	switch d.Name() {
	case "sqlserver":
		return "DELETE TOP (" + size + ") FROM " + t + "\nWHERE " + where
	case "mysql":
		return "DELETE FROM " + t + "\nWHERE " + where + "\nLIMIT " + size
	}
	return "DELETE FROM " + t + "\nWHERE " + d.Quote(key) + " IN (\n    SELECT " + d.Quote(key) + "\n    FROM " + t + "\n    WHERE " + where + "\n    LIMIT " + size + "\n)"
}
