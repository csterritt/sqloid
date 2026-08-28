// Catalog-to-tracer composition tests, per Issue #10 Tasks 1 and 4 and the
// early-integration decisions in Notes/PRD-sqloid.md: one catalog-selected
// eligible object becomes a safely identifier-quoted hardcoded SELECT * that
// executes through Connection's request boundary, returning typed headers,
// rows, or errors suitable for UI composition. Only the composition seam is
// proven here — no builder, validation, paging, count, history, recovery, or
// write behavior exists at this milestone, and none may be implied.

package schema

import (
	"strings"
	"testing"
)

func TestSelectAllSQLQuotesIdentifierSafely(t *testing.T) {
	tests := []struct {
		name string
		obj  Object
		want string
	}{
		{name: "ordinary name", obj: Object{Name: "albums"}, want: `SELECT * FROM "albums"`},
		{name: "embedded double quote", obj: Object{Name: `we"ird`}, want: `SELECT * FROM "we""ird"`},
		{name: "spaces and mixed case", obj: Object{Name: `My odd Table!`}, want: `SELECT * FROM "My odd Table!"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SelectAllSQL(&tt.obj); got != tt.want {
				t.Errorf("SelectAllSQL(%q) = %q, want %q", tt.obj.Name, got, tt.want)
			}
		})
	}
}

func TestChooseTracerTargetSelectsCatalogObject(t *testing.T) {
	cat := &Catalog{Objects: []*Object{{Name: "albums", Kind: KindOrdinaryTable}, {Name: "recent", Kind: KindView}}}
	for _, name := range []string{"albums", "recent"} {
		obj, err := ChooseTracerTarget(cat, name)
		if err != nil {
			t.Fatalf("ChooseTracerTarget(%q) error = %v, want success", name, err)
		}
		if obj.Name != name {
			t.Errorf("ChooseTracerTarget(%q).Name = %q, want %q", name, obj.Name, name)
		}
	}
}

func TestChooseTracerTargetRejectsUncatalogedObject(t *testing.T) {
	cat := &Catalog{Objects: []*Object{{Name: "albums"}}}
	for _, name := range []string{"missing", "sqlite_master"} {
		_, err := ChooseTracerTarget(cat, name)
		if err == nil {
			t.Fatalf("ChooseTracerTarget(%q) = nil error, want typed failure", name)
		}
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not identify the rejected object %q", err.Error(), name)
		}
		var te *TracerError
		if !asTracerError(err, &te) {
			t.Errorf("error %v is not a *TracerError", err)
		}
	}
}

func asTracerError(err error, target **TracerError) bool {
	for err != nil {
		if e, ok := err.(*TracerError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
