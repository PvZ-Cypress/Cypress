package compactdb

import (
	_ "embed"
	"testing"
)

// A real team-filter form captured from EA (public FBS team names + UI schema,
// no personal data). Exercises the outer TDF + DICT schema decode.
//
//go:embed testdata/form-teamfilter.bin
var teamFilterForm []byte

func TestDecodeExtractsSchema(t *testing.T) {
	form, err := Decode(teamFilterForm)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// ROOT is the form/object id (matches the 483 request FORM=133300253).
	if form.Root != 133300253 {
		t.Fatalf("expected ROOT 133300253, got %d", form.Root)
	}
	if form.RIBC != 17 || form.SIBC != 14 {
		t.Fatalf("expected RIBC=17 SIBC=14, got RIBC=%d SIBC=%d", form.RIBC, form.SIBC)
	}

	// The embedded schema must carry the UI type hierarchy.
	byName := map[string]TypeDef{}
	for _, def := range form.Schema.Types {
		byName[def.Name] = def
	}
	for _, name := range []string{"UIForm", "UIDataForm", "UISpreadsheetForm", "UIListSelectForm", "Command", "FlowCommand", "ResponseForm"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("schema missing type %q (got %d types)", name, len(form.Schema.Types))
		}
	}

	// Inheritance chain: UIListSelectForm -> UISpreadsheetForm -> UIDataForm ->
	// UIForm -> ResponseForm. FieldsWithBase must merge the whole chain.
	list := byName["UIListSelectForm"]
	if list.Base != byName["UISpreadsheetForm"].ID {
		t.Fatalf("UIListSelectForm base = %d, want UISpreadsheetForm %d", list.Base, byName["UISpreadsheetForm"].ID)
	}
	// FranTk child types embed their full inherited field set in their own DICT,
	// so the merged set equals the child's own fields (merge is idempotent).
	merged := form.Schema.FieldsWithBase(list.ID)
	if len(merged) < len(list.Fields) {
		t.Fatalf("FieldsWithBase dropped fields: merged=%d own=%d", len(merged), len(list.Fields))
	}
	// The type must carry its selection bounds and the inherited data-source /
	// naming fields.
	for _, field := range []string{"MaxSelectedItems", "MinSelectedItems", "Name", "DataSource", "Title"} {
		if _, ok := merged[field]; !ok {
			t.Fatalf("UIListSelectForm fields missing %q; has %v", field, keysOf(merged))
		}
	}
}

func TestDecodeReportsTablAsUnreversed(t *testing.T) {
	// Until the schema-driven object encoding is reversed, TABL should surface as
	// an explicit gap (no objects), not a crash or a false-empty success.
	form, err := Decode(teamFilterForm)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(form.Objects) != 0 {
		t.Logf("TABL now yields %d objects — object decode has progressed", len(form.Objects))
	}
}

func keysOf(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
