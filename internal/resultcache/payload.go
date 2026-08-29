// Retained-payload accounting for Issue #31 and the Cache and snapshot
// invariant of Notes/PRD-sqloid.md. Accounting is exact and pure: raw TEXT
// and BLOB encoded byte length, exactly 8 bytes for each INTEGER or REAL, and
// 0 for NULL. It excludes every Go/model container cost — string headers,
// slice headers, model fields — and all cache metadata, and it never derives
// cost from display width or formatted token length. BLOB payloads keep their
// distinct type and byte-for-byte identity; they are never converted to text.

package resultcache

import "github.com/chris/sqloid/internal/result"

// ValuePayload returns the exact retained-payload cost in bytes of one typed
// result value: len(Str) for TEXT, len(Bytes) for BLOB, exactly 8 for INTEGER
// and REAL, and 0 for NULL. No implementation overhead is included.
func ValuePayload(v result.Value) int64 {
	switch v.Kind {
	case result.KindText:
		return int64(len(v.Str))
	case result.KindBlob:
		return int64(len(v.Bytes))
	case result.KindInteger, result.KindReal:
		return 8
	default:
		return 0 // NULL and unknown kinds cost nothing
	}
}

// RowPayload returns the exact sum of ValuePayload across one row's values.
// Repeated values count once per position; duplicate-valued rows remain
// separate positions.
func RowPayload(values []result.Value) int64 {
	var total int64
	for _, v := range values {
		total += ValuePayload(v)
	}
	return total
}
