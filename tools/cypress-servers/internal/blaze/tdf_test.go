package blaze

import (
	"bytes"
	"reflect"
	"testing"
)

func TestTDFTagEncoding(t *testing.T) {
	got, err := EncodeTag("VALU")
	if err != nil {
		t.Fatal(err)
	}
	if want := [3]byte{0xda, 0x1b, 0x35}; got != want {
		t.Fatalf("unexpected tag bytes: want %x, got %x", want, got)
	}
	if decoded := DecodeTag(got); decoded != "VALU" {
		t.Fatalf("unexpected decoded tag: %q", decoded)
	}
}

func TestTDFRoundTrip(t *testing.T) {
	fields := []Field{
		{Tag: "VALU", Type: TypeInteger, Value: int64(42)},
		{Tag: "NAME", Type: TypeString, Value: "Local Dynasty"},
		{Tag: "DATA", Type: TypeBlob, Value: []byte{0xde, 0xad, 0xbe, 0xef}},
		{
			Tag:  "USER",
			Type: TypeStruct,
			Value: []Field{
				{Tag: "ID", Type: TypeInteger, Value: int64(1001)},
				{Tag: "DISP", Type: TypeString, Value: "LocalPlayer"},
			},
		},
		{
			Tag:  "LIST",
			Type: TypeList,
			Value: List{
				ElementType: TypeString,
				Values:      []any{"one", "two"},
			},
		},
	}

	wire, err := Encode(fields)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, fields) {
		t.Fatalf("round trip mismatch\nwant: %#v\n got: %#v", fields, decoded)
	}
}

func TestTDFDecodeRejectsTruncatedString(t *testing.T) {
	tag, err := EncodeTag("NAME")
	if err != nil {
		t.Fatal(err)
	}
	wire := append(tag[:], byte(TypeString), 0x05, 'a')

	if _, err := Decode(wire); err == nil {
		t.Fatal("expected truncated string error")
	}
}

func TestTDFDecodeRejectsExcessiveListCount(t *testing.T) {
	tag, err := EncodeTag("LIST")
	if err != nil {
		t.Fatal(err)
	}
	wire := bytes.NewBuffer(nil)
	wire.Write(tag[:])
	wire.WriteByte(byte(TypeList))
	wire.WriteByte(byte(TypeInteger))
	wire.Write([]byte{0xff, 0xff, 0xff, 0xff, 0x7f})

	if _, err := Decode(wire.Bytes()); err == nil {
		t.Fatal("expected excessive list count error")
	}
}

// A union body is a terminated field list, not a single tagged field. Reading
// only the first member made HNET (the network address list in NotifyGameSetup)
// stop early and spill its remaining fields — MACI, PORT — to the top level,
// after which the decoder was parsing noise.
func TestUnionBodyHoldsSeveralFields(t *testing.T) {
	// list of one union; the union carries three integer fields then a terminator.
	payload := []byte{
		0xa2, 0xe9, 0x74, 0x04, // HNET, list
		0x06, // element type: union
		0x01, // one element
		0x03, // active member
		0xa7, 0x00, 0x00, 0x00, 0x9e, 0xd8, 0xda, 0xc1, 0x0c,
		0xb6, 0x18, 0xe9, 0x00, 0x00,
		0xc2, 0xfc, 0xb4, 0x00, 0xa6, 0xc8, 0x02,
		0x00, // terminator
	}
	fields, err := Decode(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("got %d top-level fields, want 1 (the rest belong inside the union)", len(fields))
	}
	list, ok := fields[0].Value.(List)
	if !ok {
		t.Fatalf("HNET value is %T, want List", fields[0].Value)
	}
	union, ok := list.Values[0].(Union)
	if !ok {
		t.Fatalf("list item is %T, want Union", list.Values[0])
	}
	if len(union.Members) != 3 {
		t.Fatalf("union has %d members, want 3", len(union.Members))
	}
}

// A struct or union that ends the payload has no terminator — there is nothing
// after it to separate from — and that must not read as truncation.
func TestTerminatorMayBeOmittedAtEndOfPayload(t *testing.T) {
	payload := []byte{
		0xca, 0x58, 0x73, 0x06, // union
		0x04,                         // active member
		0x97, 0x2c, 0x80, 0x00, 0x00, // one integer field, no terminator
	}
	if _, err := Decode(payload); err != nil {
		t.Fatalf("payload ending inside a union must decode: %v", err)
	}
}

// A union's encoding depends on where it sits, and getting this wrong corrupts
// whichever frame uses the other form:
//
//   - as a struct FIELD it carries one tagged member, and the fields after it are
//     siblings. Treating it as terminated made PNET swallow the twelve fields
//     that follow it in NotifyPlayerJoining.
//   - as a LIST ELEMENT there is no tag, so the body is terminated. Treating it
//     as a single tagged field made HNET in NotifyGameSetup stop after its first
//     member and spill the rest into the enclosing struct.
func TestUnionEncodingDependsOnPosition(t *testing.T) {
	// A union as a struct field, followed by a sibling that must survive.
	asField := []byte{
		0xa2, 0xe9, 0x74, 0x06, // HNET, union
		0x02,                   // active member
		0xb6, 0x18, 0xe9, 0x00, // tagged member: integer
		0x05,
		0xc2, 0xfc, 0xb4, 0x00, // sibling field, NOT part of the union
		0x07,
	}
	fields, err := Decode(asField)
	if err != nil {
		t.Fatalf("decode union-as-field: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("got %d fields, want 2 — the union swallowed its sibling", len(fields))
	}
	again, err := Encode(fields)
	if err != nil {
		t.Fatalf("encode union-as-field: %v", err)
	}
	if !bytes.Equal(again, asField) {
		t.Fatalf("union-as-field did not round trip:\n have %x\n want %x", again, asField)
	}

	// A union as a list element: untagged body, terminated.
	asElement := []byte{
		0xa2, 0xe9, 0x74, 0x04, // HNET, list
		0x06, 0x01, // one union element
		0x03,                         // active member
		0xb6, 0x18, 0xe9, 0x00, 0x05, // member field
		0xc2, 0xfc, 0xb4, 0x00, 0x07, // second member field
		0x00, // terminator
	}
	fields, err = Decode(asElement)
	if err != nil {
		t.Fatalf("decode union-in-list: %v", err)
	}
	list, ok := fields[0].Value.(List)
	if !ok || len(list.Values) != 1 {
		t.Fatalf("expected a one-element list, got %#v", fields[0].Value)
	}
	union, ok := list.Values[0].(Union)
	if !ok {
		t.Fatalf("list item is %T, want Union", list.Values[0])
	}
	if len(union.Members) != 2 {
		t.Fatalf("union carries %d members, want 2", len(union.Members))
	}
	again, err = Encode(fields)
	if err != nil {
		t.Fatalf("encode union-in-list: %v", err)
	}
	if !bytes.Equal(again, asElement) {
		t.Fatalf("union-in-list did not round trip:\n have %x\n want %x", again, asElement)
	}
}
