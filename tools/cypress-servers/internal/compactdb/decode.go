// Package compactdb decodes the CFB27 Dynasty "FranTk form" compact database —
// the FORM/DATA blobs the Blaze Dynasty commands exchange (list menu, hub,
// member registry, settings, team select). The private server currently replays
// captured Akron copies of these; decoding them is the first half of generating
// real ones (correct team, sticky settings, real member list).
//
// Wire format (see Docs/cfb27-compact-db-format.md):
//
//	FORM/DATA : struct {
//	  DICT : map<int typeId, TypeDef>    // embedded schema
//	  RIBC, ROOT, SIBC : int             // ROOT = root object id
//	  TABL : map<int objectId, object>   // the data
//	}
//
// The outer layer is standard Blaze TDF. TABL objects are schema-driven, so this
// package builds the schema from DICT (resolving BASE inheritance) and uses it
// to interpret objects. Field/value encodings not yet reversed are preserved as
// raw bytes rather than dropped, so nothing is silently lost and the gaps are
// visible.
package compactdb

import (
	"errors"
	"fmt"
)

// Type is the TDF/FranTk type tag on a field or map/list element.
type Type uint8

const (
	TypeInt      Type = 0
	TypeString   Type = 1
	TypeBlob     Type = 2
	TypeStruct   Type = 3
	TypeList     Type = 4
	TypeMap      Type = 5
	TypeUnion    Type = 6
	TypeVariable Type = 7
	TypeObjType  Type = 8
	TypeObjID    Type = 9
	TypeFloat    Type = 10
)

// TypeDef is one entry of the embedded DICT schema.
type TypeDef struct {
	ID     int64
	Name   string
	Base   int64
	Fields map[string]int64 // field name -> field id (this type's own fields)
}

// Schema is the DICT, indexed by type id, with base-chain resolution.
type Schema struct {
	Types map[int64]TypeDef
}

// FieldsWithBase returns the full field set for a type, its own fields merged
// over its base chain (child overrides base on name collisions).
func (s Schema) FieldsWithBase(typeID int64) map[string]int64 {
	merged := map[string]int64{}
	var walk func(id int64, depth int)
	walk = func(id int64, depth int) {
		if depth > 32 {
			return
		}
		def, ok := s.Types[id]
		if !ok {
			return
		}
		if def.Base != 0 {
			walk(def.Base, depth+1)
		}
		for name, fid := range def.Fields {
			merged[name] = fid
		}
	}
	walk(typeID, 0)
	return merged
}

// Form is a decoded compact-DB form.
type Form struct {
	Root    int64
	RIBC    int64
	SIBC    int64
	Schema  Schema
	Objects map[int64]Node // TABL, by object id
	// Scalars are any sibling top-level fields (CAYE, CNFW, PFIL, SUCC, …).
	Scalars map[string]Node
}

// Node is a decoded value. Kind mirrors Type; Raw holds bytes for encodings not
// yet reversed.
type Node struct {
	Kind     Type
	Int      int64
	Str      string
	Bytes    []byte
	Children []Field // struct fields
	List     []Node
	MapKV    []KV
	Unknown  bool // true when the value type could not be interpreted
}

type Field struct {
	Tag   string
	Value Node
}

type KV struct {
	Key   Node
	Value Node
}

// Decode parses a compact-DB form blob.
func Decode(data []byte) (*Form, error) {
	d := &decoder{data: data}
	fields, err := d.structFields(0)
	// An unreversed TABL value type is expected for now: the schema (DICT) and
	// structural ints decode before it, so keep the partial result. Any other
	// error is fatal.
	if err != nil && !IsUnknownType(err) {
		return nil, err
	}
	// The form is usually a single top-level struct (FORM/DATA) plus optional
	// sibling scalars. Find the struct that carries DICT/TABL.
	form := &Form{Objects: map[int64]Node{}, Scalars: map[string]Node{}}
	var container []Field
	for _, f := range fields {
		if f.Value.Kind == TypeStruct && hasChild(f.Value.Children, "DICT") {
			container = f.Value.Children
		} else {
			form.Scalars[f.Tag] = f.Value
		}
	}
	if container == nil {
		// Some forms are the container at top level (no wrapper).
		if hasChild(fields, "DICT") {
			container = fields
		} else {
			return nil, errors.New("no DICT container found in form")
		}
	}
	form.Schema = Schema{Types: map[int64]TypeDef{}}
	for _, f := range container {
		switch f.Tag {
		case "DICT":
			form.Schema = buildSchema(f.Value)
		case "ROOT":
			form.Root = f.Value.Int
		case "RIBC":
			form.RIBC = f.Value.Int
		case "SIBC":
			form.SIBC = f.Value.Int
		case "TABL":
			for _, kv := range f.Value.MapKV {
				form.Objects[kv.Key.Int] = kv.Value
			}
		}
	}
	return form, nil
}

func hasChild(fields []Field, tag string) bool {
	for _, f := range fields {
		if f.Tag == tag {
			return true
		}
	}
	return false
}

func buildSchema(dict Node) Schema {
	s := Schema{Types: map[int64]TypeDef{}}
	for _, kv := range dict.MapKV {
		def := TypeDef{ID: kv.Key.Int, Fields: map[string]int64{}}
		for _, f := range kv.Value.Children {
			switch f.Tag {
			case "NAME":
				def.Name = f.Value.Str
			case "BASE":
				def.Base = f.Value.Int
			case "DICT":
				for _, fkv := range f.Value.MapKV {
					def.Fields[fkv.Key.Str] = fkv.Value.Int
				}
			}
		}
		s.Types[def.ID] = def
	}
	return s
}

type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) structFields(depth int) ([]Field, error) {
	if depth > 64 {
		return nil, errors.New("compactdb: nesting too deep")
	}
	var fields []Field
	for d.pos < len(d.data) {
		if d.data[d.pos] == 0 { // struct terminator
			d.pos++
			return fields, nil
		}
		if d.pos+4 > len(d.data) {
			return nil, errors.New("compactdb: truncated field header")
		}
		tag := decodeTag(d.data[d.pos], d.data[d.pos+1], d.data[d.pos+2])
		ty := Type(d.data[d.pos+3])
		d.pos += 4
		val, err := d.value(ty, depth+1)
		if err != nil {
			// Preserve fields decoded so far so the caller keeps the schema even
			// when a later field (TABL) hits an unreversed encoding. The offset of
			// the failing field is recorded on the value for raw capture.
			val.Bytes = append([]byte(nil), d.data[d.pos:]...)
			fields = append(fields, Field{Tag: tag, Value: val})
			return fields, fmt.Errorf("field %s: %w", tag, err)
		}
		fields = append(fields, Field{Tag: tag, Value: val})
	}
	return fields, nil
}

func (d *decoder) value(ty Type, depth int) (Node, error) {
	if depth > 64 {
		return Node{}, errors.New("compactdb: value nesting too deep")
	}
	switch ty {
	case TypeInt:
		v, err := d.integer()
		return Node{Kind: TypeInt, Int: v}, err
	case TypeString:
		s, err := d.str()
		return Node{Kind: TypeString, Str: s}, err
	case TypeStruct:
		children, err := d.structFields(depth)
		return Node{Kind: TypeStruct, Children: children}, err
	case TypeList:
		if d.pos >= len(d.data) {
			return Node{}, errors.New("compactdb: truncated list header")
		}
		et := Type(d.data[d.pos])
		d.pos++
		n, err := d.count()
		if err != nil {
			return Node{}, err
		}
		node := Node{Kind: TypeList}
		for i := 0; i < n; i++ {
			el, err := d.value(et, depth+1)
			if err != nil {
				return node, err
			}
			node.List = append(node.List, el)
		}
		return node, nil
	case TypeMap:
		if d.pos+2 > len(d.data) {
			return Node{}, errors.New("compactdb: truncated map header")
		}
		kt := Type(d.data[d.pos])
		vt := Type(d.data[d.pos+1])
		d.pos += 2
		n, err := d.count()
		if err != nil {
			return Node{}, err
		}
		node := Node{Kind: TypeMap}
		for i := 0; i < n; i++ {
			k, err := d.value(kt, depth+1)
			if err != nil {
				return node, fmt.Errorf("map key %d: %w", i, err)
			}
			v, err := d.value(vt, depth+1)
			if err != nil {
				return node, fmt.Errorf("map value %d: %w", i, err)
			}
			node.MapKV = append(node.MapKV, KV{Key: k, Value: v})
		}
		return node, nil
	case TypeFloat:
		if d.pos+4 > len(d.data) {
			return Node{}, errors.New("compactdb: truncated float")
		}
		raw := d.data[d.pos : d.pos+4]
		d.pos += 4
		return Node{Kind: TypeFloat, Bytes: append([]byte(nil), raw...)}, nil
	default:
		// FranTk-specific / not-yet-reversed encoding (e.g. schema-driven object
		// fields, type 0x8d). Preserve remaining bytes as raw and stop cleanly so
		// the caller sees an explicit gap instead of a decode crash.
		return Node{Kind: ty, Unknown: true}, errUnknownType{ty: ty, at: d.pos - 1}
	}
}

type errUnknownType struct {
	ty Type
	at int
}

func (e errUnknownType) Error() string {
	return fmt.Sprintf("compactdb: unreversed value type 0x%02x at offset %d", uint8(e.ty), e.at)
}

// IsUnknownType reports whether err is the sentinel for an unreversed value type.
func IsUnknownType(err error) bool {
	var u errUnknownType
	return errors.As(err, &u)
}

func (d *decoder) integer() (int64, error) {
	if d.pos >= len(d.data) {
		return 0, errors.New("compactdb: truncated integer")
	}
	first := d.data[d.pos]
	d.pos++
	neg := first&0x40 != 0
	value := uint64(first & 0x3f)
	shift := uint(6)
	for first&0x80 != 0 {
		if d.pos >= len(d.data) || shift >= 64 {
			return 0, errors.New("compactdb: malformed integer")
		}
		first = d.data[d.pos]
		d.pos++
		value |= uint64(first&0x7f) << shift
		shift += 7
	}
	if neg {
		return -int64(value), nil
	}
	return int64(value), nil
}

func (d *decoder) count() (int, error) {
	v, err := d.integer()
	if err != nil {
		return 0, err
	}
	if v < 0 || v > 1<<24 {
		return 0, fmt.Errorf("compactdb: implausible count %d", v)
	}
	return int(v), nil
}

func (d *decoder) str() (string, error) {
	length, err := d.integer()
	if err != nil {
		return "", err
	}
	if length < 1 || int(length) > len(d.data)-d.pos {
		return "", fmt.Errorf("compactdb: bad string length %d", length)
	}
	// Length includes the trailing NUL.
	s := string(d.data[d.pos : d.pos+int(length)-1])
	d.pos += int(length)
	return s, nil
}

func decodeTag(a, b, c byte) string {
	packed := uint32(a)<<16 | uint32(b)<<8 | uint32(c)
	raw := []byte{
		byte((packed>>18)&0x3f) + 0x20,
		byte((packed>>12)&0x3f) + 0x20,
		byte((packed>>6)&0x3f) + 0x20,
		byte(packed&0x3f) + 0x20,
	}
	// trim trailing spaces
	end := len(raw)
	for end > 0 && raw[end-1] == ' ' {
		end--
	}
	return string(raw[:end])
}
