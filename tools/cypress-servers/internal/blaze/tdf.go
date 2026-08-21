package blaze

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	maxTDFDepth       = 32
	maxTDFFields      = 65536
	maxTDFCollection  = 65536
	maxTDFValueLength = 16 << 20
)

type Type uint8

const (
	TypeInteger Type = iota
	TypeString
	TypeBlob
	TypeStruct
	TypeList
	TypeMap
	TypeUnion
	TypeVariable
	TypeObjectType
	TypeObjectID
	TypeFloat
	// TypeGeneric (12) is EA's "generic" TDF: a value whose concrete type is not
	// in the wire encoding but in a registry the client and server share. It
	// appears as the value type of the TMPA template-attribute map in
	// GameManager createOrJoinGame (4/10), which is why that request would not
	// decode at all before this existed.
	//
	// Layout: [present:1][typeID varint][value][0x00]. The type id names a
	// registered TDF type; two are seen in CFB27 captures — 20 (integer) and
	// 2807479042 (list<string>).
	TypeGeneric Type = 12
)

// Known generic type ids, mapped to the wire shape their value uses.
var genericTypes = map[int64]Type{
	20:         TypeInteger,
	2807479042: TypeList,
}

// Generic is a TypeGeneric value: a registry type id plus its decoded payload.
type Generic struct {
	Present  bool
	TypeID   int64
	ValueTyp Type
	Value    any
}

type Field struct {
	Tag   string
	Type  Type
	Value any
}

type List struct {
	ElementType Type
	Values      []any
}

type MapEntry struct {
	Key   any
	Value any
}

type Map struct {
	KeyType   Type
	ValueType Type
	Entries   []MapEntry
}

type ObjectType struct {
	Component uint16
	Type      uint16
}

type ObjectID struct {
	Type ObjectType
	ID   int64
}

type Union struct {
	ActiveMember uint8
	// Value is the first member, kept so existing callers and the encoder are
	// unaffected. Members holds the full body, which can carry several fields.
	Value   *Field
	Members []Field
}

type Variable struct {
	Marker uint8
	Value  *Field
}

func EncodeTag(tag string) ([3]byte, error) {
	var out [3]byte
	if len(tag) > 4 {
		return out, fmt.Errorf("TDF tag %q is longer than four ASCII characters", tag)
	}
	tag += strings.Repeat(" ", 4-len(tag))

	var packed uint32
	for i := 0; i < 4; i++ {
		c := tag[i]
		if c < 0x20 || c > 0x5f {
			return out, fmt.Errorf("TDF tag %q contains unsupported character 0x%02x", tag, c)
		}
		packed = (packed << 6) | uint32(c-0x20)
	}
	out[0] = byte(packed >> 16)
	out[1] = byte(packed >> 8)
	out[2] = byte(packed)
	return out, nil
}

func DecodeTag(tag [3]byte) string {
	packed := uint32(tag[0])<<16 | uint32(tag[1])<<8 | uint32(tag[2])
	raw := [4]byte{
		byte((packed>>18)&0x3f) + 0x20,
		byte((packed>>12)&0x3f) + 0x20,
		byte((packed>>6)&0x3f) + 0x20,
		byte(packed&0x3f) + 0x20,
	}
	return strings.TrimRight(string(raw[:]), " ")
}

// TraceHook, when set, is called for every field header the decoder reads.
// It exists to diagnose desyncs on captured payloads: the last few entries
// before a failure show which construct the decoder mis-measured. Leave nil in
// production — it is checked, not called, on the hot path.
var TraceHook func(offset, depth int, tag string, typ Type)

func Decode(data []byte) ([]Field, error) {
	d := decoder{data: data}
	fields, err := d.fields(false, 0)
	if err != nil {
		return nil, err
	}
	if d.pos != len(d.data) {
		return nil, fmt.Errorf("TDF decoder left %d unread bytes", len(d.data)-d.pos)
	}
	return fields, nil
}

func Encode(fields []Field) ([]byte, error) {
	var out bytes.Buffer
	if err := encodeFields(&out, fields, false, 0); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) fields(terminated bool, depth int) ([]Field, error) {
	if depth > maxTDFDepth {
		return nil, errors.New("TDF nesting limit exceeded")
	}
	fields := make([]Field, 0)
	for d.pos < len(d.data) {
		fieldOffset := d.pos
		if terminated && d.data[d.pos] == 0 {
			d.pos++
			return fields, nil
		}
		if len(fields) >= maxTDFFields {
			return nil, errors.New("TDF field limit exceeded")
		}
		if len(d.data)-d.pos < 4 {
			return nil, errors.New("truncated TDF field header")
		}
		tag := [3]byte{d.data[d.pos], d.data[d.pos+1], d.data[d.pos+2]}
		typ := Type(d.data[d.pos+3])
		d.pos += 4
		if TraceHook != nil {
			TraceHook(fieldOffset, depth, DecodeTag(tag), typ)
		}
		value, err := d.value(typ, depth+1)
		if err != nil {
			return nil, fmt.Errorf("decode TDF field %s at offset %d: %w", DecodeTag(tag), fieldOffset, err)
		}
		fields = append(fields, Field{Tag: DecodeTag(tag), Type: typ, Value: value})
	}
	if terminated {
		// Running out of data ends the list too. The final field of a payload can
		// be a struct or union whose terminator is simply omitted — there is
		// nothing after it to separate from. Decode still verifies that the whole
		// payload was consumed, so a genuinely truncated body is caught there.
		return fields, nil
	}
	return fields, nil
}

func (d *decoder) value(typ Type, depth int) (any, error) {
	return d.valueIn(typ, depth, false)
}

// valueIn decodes a value, tracking whether it is a list element: union encoding
// differs between the two positions.
func (d *decoder) valueIn(typ Type, depth int, inList bool) (any, error) {
	if depth > maxTDFDepth {
		return nil, errors.New("TDF nesting limit exceeded")
	}
	switch typ {
	case TypeInteger:
		return d.integer()
	case TypeString:
		length, err := d.length()
		if err != nil {
			return nil, err
		}
		if length == 0 {
			return "", nil
		}
		raw, err := d.take(length)
		if err != nil {
			return nil, err
		}
		if raw[len(raw)-1] != 0 {
			return nil, errors.New("TDF string is not null terminated")
		}
		return string(raw[:len(raw)-1]), nil
	case TypeBlob:
		length, err := d.length()
		if err != nil {
			return nil, err
		}
		raw, err := d.take(length)
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), raw...), nil
	case TypeStruct:
		return d.fields(true, depth)
	case TypeList:
		elementByte, err := d.byte()
		if err != nil {
			return nil, err
		}
		count, err := d.count()
		if err != nil {
			return nil, err
		}
		list := List{ElementType: Type(elementByte), Values: make([]any, 0, count)}
		for i := 0; i < count; i++ {
			value, err := d.valueIn(list.ElementType, depth+1, true)
			if err != nil {
				return nil, fmt.Errorf("decode list item %d: %w", i, err)
			}
			list.Values = append(list.Values, value)
		}
		return list, nil
	case TypeMap:
		keyByte, err := d.byte()
		if err != nil {
			return nil, err
		}
		valueByte, err := d.byte()
		if err != nil {
			return nil, err
		}
		count, err := d.count()
		if err != nil {
			return nil, err
		}
		m := Map{KeyType: Type(keyByte), ValueType: Type(valueByte), Entries: make([]MapEntry, 0, count)}
		for i := 0; i < count; i++ {
			key, err := d.value(m.KeyType, depth+1)
			if err != nil {
				return nil, fmt.Errorf("decode map key %d: %w", i, err)
			}
			value, err := d.value(m.ValueType, depth+1)
			if err != nil {
				return nil, fmt.Errorf("decode map value %d: %w", i, err)
			}
			m.Entries = append(m.Entries, MapEntry{Key: key, Value: value})
		}
		return m, nil
	case TypeUnion:
		activeMember, err := d.byte()
		if err != nil {
			return nil, err
		}
		union := Union{ActiveMember: activeMember}
		if activeMember == 0x7f {
			return union, nil
		}
		// How a union's body is encoded depends on where the union sits.
		//
		// As a struct FIELD it carries one tagged member, and the fields after it
		// are its siblings: reading a terminated list here made PNET swallow the
		// twelve that follow it in NotifyPlayerJoining (PSET, RCRE, ROLE, SCEN,
		// SID, SLOT, STAT, TIDX, TIME, UGID, UID, UUID).
		//
		// As a LIST ELEMENT there is no tag — list elements are untagged — so the
		// body is a terminated field list. Reading a single tagged field here made
		// HNET in NotifyGameSetup stop after its first member and spill MACI and
		// PORT to the enclosing struct, after which the stream was noise.
		if inList {
			members, err := d.fields(true, depth)
			if err != nil {
				return nil, err
			}
			union.Members = members
			if len(members) > 0 {
				first := members[0]
				union.Value = &first
			}
			return union, nil
		}
		if len(d.data)-d.pos < 4 {
			return nil, errors.New("truncated TDF union field")
		}
		tag := [3]byte{d.data[d.pos], d.data[d.pos+1], d.data[d.pos+2]}
		typ := Type(d.data[d.pos+3])
		d.pos += 4
		value, err := d.value(typ, depth+1)
		if err != nil {
			return nil, err
		}
		field := Field{Tag: DecodeTag(tag), Type: typ, Value: value}
		union.Value = &field
		union.Members = []Field{field}
		return union, nil
	case TypeVariable:
		marker, err := d.byte()
		if err != nil {
			return nil, err
		}
		if len(d.data)-d.pos < 4 {
			return nil, errors.New("truncated TDF variable field")
		}
		tag := [3]byte{d.data[d.pos], d.data[d.pos+1], d.data[d.pos+2]}
		typ := Type(d.data[d.pos+3])
		d.pos += 4
		value, err := d.value(typ, depth+1)
		if err != nil {
			return nil, err
		}
		return Variable{Marker: marker, Value: &Field{Tag: DecodeTag(tag), Type: typ, Value: value}}, nil
	case TypeObjectType:
		component, err := d.integer()
		if err != nil {
			return nil, err
		}
		objectType, err := d.integer()
		if err != nil {
			return nil, err
		}
		return ObjectType{Component: uint16(component), Type: uint16(objectType)}, nil
	case TypeObjectID:
		objectTypeValue, err := d.value(TypeObjectType, depth+1)
		if err != nil {
			return nil, err
		}
		id, err := d.integer()
		if err != nil {
			return nil, err
		}
		return ObjectID{Type: objectTypeValue.(ObjectType), ID: id}, nil
	case TypeFloat:
		raw, err := d.take(4)
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(binary.BigEndian.Uint32(raw)), nil
	case TypeGeneric:
		return d.generic(depth)
	default:
		return nil, fmt.Errorf("unsupported TDF type %d", typ)
	}
}

// generic decodes a TypeGeneric value. The concrete type is not on the wire, so
// a known type id is used when there is one and otherwise the candidate shapes
// are tried in turn: the value is followed by a 0x00 terminator, which is enough
// to tell a correct reading from a wrong one.
func (d *decoder) generic(depth int) (any, error) {
	present, err := d.byte()
	if err != nil {
		return nil, err
	}
	if present == 0 {
		return Generic{}, nil
	}
	typeID, err := d.integer()
	if err != nil {
		return nil, err
	}
	candidates := []Type{TypeInteger, TypeString, TypeList, TypeBlob, TypeStruct, TypeMap, TypeFloat}
	if known, ok := genericTypes[typeID]; ok {
		candidates = append([]Type{known}, candidates...)
	}
	start := d.pos
	for _, candidate := range candidates {
		d.pos = start
		value, err := d.valueIn(candidate, depth+1, false)
		if err != nil {
			continue
		}
		// The terminator is what confirms the reading consumed exactly the value.
		if d.pos >= len(d.data) || d.data[d.pos] != 0 {
			continue
		}
		d.pos++
		return Generic{Present: true, TypeID: typeID, ValueTyp: candidate, Value: value}, nil
	}
	return nil, fmt.Errorf("generic type id %d: no candidate TDF type decodes the value", typeID)
}

func (d *decoder) integer() (int64, error) {
	first, err := d.byte()
	if err != nil {
		return 0, err
	}
	negative := first&0x40 != 0
	value := uint64(first & 0x3f)
	shift := uint(6)
	for first&0x80 != 0 {
		if shift >= 63 {
			return 0, errors.New("TDF integer overflow")
		}
		first, err = d.byte()
		if err != nil {
			return 0, err
		}
		value |= uint64(first&0x7f) << shift
		shift += 7
	}
	if value > math.MaxInt64 {
		return 0, errors.New("TDF integer overflow")
	}
	if negative {
		return -int64(value), nil
	}
	return int64(value), nil
}

func (d *decoder) length() (int, error) {
	value, err := d.integer()
	if err != nil {
		return 0, err
	}
	if value < 0 || value > maxTDFValueLength {
		return 0, fmt.Errorf("invalid TDF value length %d", value)
	}
	return int(value), nil
}

func (d *decoder) count() (int, error) {
	value, err := d.integer()
	if err != nil {
		return 0, err
	}
	if value < 0 || value > maxTDFCollection {
		return 0, fmt.Errorf("invalid TDF collection count %d", value)
	}
	return int(value), nil
}

func (d *decoder) byte() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, errors.New("unexpected end of TDF data")
	}
	value := d.data[d.pos]
	d.pos++
	return value, nil
}

func (d *decoder) take(length int) ([]byte, error) {
	if length < 0 || length > len(d.data)-d.pos {
		return nil, errors.New("truncated TDF value")
	}
	value := d.data[d.pos : d.pos+length]
	d.pos += length
	return value, nil
}

func encodeFields(out *bytes.Buffer, fields []Field, terminated bool, depth int) error {
	if depth > maxTDFDepth {
		return errors.New("TDF nesting limit exceeded")
	}
	if len(fields) > maxTDFFields {
		return errors.New("TDF field limit exceeded")
	}
	for _, field := range fields {
		tag, err := EncodeTag(field.Tag)
		if err != nil {
			return err
		}
		out.Write(tag[:])
		out.WriteByte(byte(field.Type))
		if err := encodeValue(out, field.Type, field.Value, depth+1); err != nil {
			return fmt.Errorf("encode TDF field %s: %w", field.Tag, err)
		}
	}
	if terminated {
		out.WriteByte(0)
	}
	return nil
}

func encodeValue(out *bytes.Buffer, typ Type, value any, depth int) error {
	return encodeValueIn(out, typ, value, depth, false)
}

// encodeValueIn mirrors valueIn: a union written as a list element has no tag and
// is terminated, while one written as a struct field carries a tagged member.
func encodeValueIn(out *bytes.Buffer, typ Type, value any, depth int, inList bool) error {
	if depth > maxTDFDepth {
		return errors.New("TDF nesting limit exceeded")
	}
	switch typ {
	case TypeInteger:
		v, ok := value.(int64)
		if !ok {
			return fmt.Errorf("integer value has type %T", value)
		}
		writeInteger(out, v)
	case TypeString:
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("string value has type %T", value)
		}
		writeInteger(out, int64(len(v)+1))
		out.WriteString(v)
		out.WriteByte(0)
	case TypeBlob:
		v, ok := value.([]byte)
		if !ok {
			return fmt.Errorf("blob value has type %T", value)
		}
		writeInteger(out, int64(len(v)))
		out.Write(v)
	case TypeStruct:
		v, ok := value.([]Field)
		if !ok {
			return fmt.Errorf("struct value has type %T", value)
		}
		return encodeFields(out, v, true, depth)
	case TypeList:
		v, ok := value.(List)
		if !ok {
			return fmt.Errorf("list value has type %T", value)
		}
		if len(v.Values) > maxTDFCollection {
			return errors.New("TDF list count exceeds limit")
		}
		out.WriteByte(byte(v.ElementType))
		writeInteger(out, int64(len(v.Values)))
		for _, item := range v.Values {
			if err := encodeValueIn(out, v.ElementType, item, depth+1, true); err != nil {
				return err
			}
		}
	case TypeMap:
		v, ok := value.(Map)
		if !ok {
			return fmt.Errorf("map value has type %T", value)
		}
		if len(v.Entries) > maxTDFCollection {
			return errors.New("TDF map count exceeds limit")
		}
		out.WriteByte(byte(v.KeyType))
		out.WriteByte(byte(v.ValueType))
		writeInteger(out, int64(len(v.Entries)))
		for _, entry := range v.Entries {
			if err := encodeValue(out, v.KeyType, entry.Key, depth+1); err != nil {
				return err
			}
			if err := encodeValue(out, v.ValueType, entry.Value, depth+1); err != nil {
				return err
			}
		}
	case TypeUnion:
		v, ok := value.(Union)
		if !ok {
			return fmt.Errorf("union value has type %T", value)
		}
		out.WriteByte(v.ActiveMember)
		if v.ActiveMember == 0x7f {
			return nil
		}
		// Mirror the decoder: in a list the body is a terminated field list; as a
		// struct field it is a single tagged member.
		if inList {
			members := v.Members
			if len(members) == 0 && v.Value != nil {
				members = []Field{*v.Value}
			}
			return encodeFields(out, members, true, depth+1)
		}
		if v.Value == nil {
			return errors.New("active union has no value")
		}
		tag, err := EncodeTag(v.Value.Tag)
		if err != nil {
			return err
		}
		out.Write(tag[:])
		out.WriteByte(byte(v.Value.Type))
		return encodeValue(out, v.Value.Type, v.Value.Value, depth+1)
	case TypeVariable:
		v, ok := value.(Variable)
		if !ok {
			return fmt.Errorf("variable value has type %T", value)
		}
		out.WriteByte(v.Marker)
		if v.Value == nil {
			return errors.New("variable has no value")
		}
		tag, err := EncodeTag(v.Value.Tag)
		if err != nil {
			return err
		}
		out.Write(tag[:])
		out.WriteByte(byte(v.Value.Type))
		return encodeValue(out, v.Value.Type, v.Value.Value, depth+1)
	case TypeObjectType:
		v, ok := value.(ObjectType)
		if !ok {
			return fmt.Errorf("object type value has type %T", value)
		}
		writeInteger(out, int64(v.Component))
		writeInteger(out, int64(v.Type))
	case TypeObjectID:
		v, ok := value.(ObjectID)
		if !ok {
			return fmt.Errorf("object ID value has type %T", value)
		}
		if err := encodeValue(out, TypeObjectType, v.Type, depth+1); err != nil {
			return err
		}
		writeInteger(out, v.ID)
	case TypeFloat:
		v, ok := value.(float32)
		if !ok {
			return fmt.Errorf("float value has type %T", value)
		}
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], math.Float32bits(v))
		out.Write(raw[:])
	case TypeGeneric:
		v, ok := value.(Generic)
		if !ok {
			return fmt.Errorf("generic value has type %T", value)
		}
		if !v.Present {
			out.WriteByte(0)
			return nil
		}
		out.WriteByte(1)
		writeInteger(out, v.TypeID)
		if err := encodeValue(out, v.ValueTyp, v.Value, depth+1); err != nil {
			return err
		}
		out.WriteByte(0)
	default:
		return fmt.Errorf("unsupported TDF type %d", typ)
	}
	return nil
}

func writeInteger(out *bytes.Buffer, value int64) {
	negative := value < 0
	var magnitude uint64
	if negative {
		magnitude = uint64(-(value + 1))
		magnitude++
	} else {
		magnitude = uint64(value)
	}

	first := byte(magnitude & 0x3f)
	magnitude >>= 6
	if negative {
		first |= 0x40
	}
	if magnitude != 0 {
		first |= 0x80
	}
	out.WriteByte(first)

	for magnitude != 0 {
		next := byte(magnitude & 0x7f)
		magnitude >>= 7
		if magnitude != 0 {
			next |= 0x80
		}
		out.WriteByte(next)
	}
}
