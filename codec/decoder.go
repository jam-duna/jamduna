package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
)

// Uint128 represents a 128-bit unsigned integer.
type Uint128 struct {
	Low  uint64
	High uint64
}

// Implement the missing methods for Uint128
func NewUint128(buf []byte) (Uint128, error) {
	if len(buf) != 16 {
		return Uint128{}, fmt.Errorf("invalid length for Uint128: %d", len(buf))
	}
	return Uint128{
		Low:  binary.LittleEndian.Uint64(buf[:8]),
		High: binary.LittleEndian.Uint64(buf[8:]),
	}, nil
}

// Result represents a generic result type with an OK or Error value.
type Result struct {
	mode int
	ok   interface{}
	err  interface{}
}

const (
	OK  = 0
	Err = 1
)

// Result methods
func (r *Result) Set(mode int, value interface{}) error {
	if mode != OK && mode != Err {
		return fmt.Errorf("invalid result mode: %d", mode)
	}
	r.mode = mode
	if mode == OK {
		r.ok = value
	} else {
		r.err = value
	}
	return nil
}

func (r *Result) IsSet() bool {
	return r.mode == OK || r.mode == Err
}

// indirect walks down v allocating pointers as needed, until it gets to a non-pointer.
func indirect(dstv reflect.Value) (elem reflect.Value) {
	dstv0 := dstv
	haveAddr := false
	for {
		if dstv.Kind() == reflect.Interface && !dstv.IsNil() {
			e := dstv.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() && e.Elem().Kind() == reflect.Ptr {
				haveAddr = false
				dstv = e
				continue
			}
		}
		if dstv.Kind() != reflect.Ptr {
			break
		}
		if dstv.CanSet() {
			break
		}
		if dstv.Elem().Kind() == reflect.Interface && dstv.Elem().Elem() == dstv {
			dstv = dstv.Elem()
			break
		}
		if dstv.IsNil() {
			dstv.Set(reflect.New(dstv.Type().Elem()))
		}
		if haveAddr {
			dstv = dstv0
			haveAddr = false
		} else {
			dstv = dstv.Elem()
		}
	}
	elem = dstv
	return
}

// Unmarshal takes data and a destination pointer to unmarshal the data to.
func Unmarshal(data []byte, dst interface{}) (err error) {
	dstv := reflect.ValueOf(dst)
	if dstv.Kind() != reflect.Ptr || dstv.IsNil() {
		err = fmt.Errorf("%w: %T", ErrUnsupportedDestination, dst)
		return
	}

	ds := decodeState{}

	ds.Reader = bytes.NewBuffer(data)

	err = ds.unmarshal(indirect(dstv))
	if err != nil {
		return
	}
	return
}

// Unmarshaler is the interface for custom JAM codec for a given type
type Unmarshaler interface {
	UnmarshalJAM(io.Reader) error
}

// Decoder is used to decode from an io.Reader
type Decoder struct {
	decodeState
}

// Decode accepts a pointer to a destination and decodes into the supplied destination
func (d *Decoder) Decode(dst interface{}) (err error) {
	dstv := reflect.ValueOf(dst)
	if dstv.Kind() != reflect.Ptr || dstv.IsNil() {
		err = fmt.Errorf("%w: %T", ErrUnsupportedDestination, dst)
		return
	}

	err = d.unmarshal(indirect(dstv))
	if err != nil {
		return
	}
	return nil
}

// NewDecoder is constructor for Decoder
func NewDecoder(r io.Reader) (d *Decoder) {
	d = &Decoder{
		decodeState{r},
	}
	return
}

type decodeState struct {
	io.Reader
}

func (ds *decodeState) unmarshal(dstv reflect.Value) (err error) {
	unmarshalerType := reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	if dstv.CanAddr() && dstv.Addr().Type().Implements(unmarshalerType) {
		methodVal := dstv.Addr().MethodByName("UnmarshalJAM")
		values := methodVal.Call([]reflect.Value{reflect.ValueOf(ds.Reader)})
		if !values[0].IsNil() {
			errIn := values[0].Interface()
			err := errIn.(error)
			return err
		}
		return
	}

	if dstv.CanAddr() {
		addr := dstv.Addr()
		vdt, ok := addr.Interface().(VaryingDataType)
		if ok {
			err = ds.decodeVaryingDataType(vdt)
			return
		}
	}

	in := dstv.Interface()
	switch in.(type) {
	case *big.Int:
		err = ds.decodeBigInt(dstv)
	case *Uint128:
		err = ds.decodeUint128(dstv)
	case int, uint:
		err = ds.decodeUint(dstv)
	case int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		err = ds.decodeFixedWidthInt(dstv)
	case []byte:
		err = ds.decodeBytes(dstv)
	case string:
		err = ds.decodeBytes(dstv)
	case bool:
		err = ds.decodeBool(dstv)
	case []bool:
		err = ds.decodeBitSequence(dstv)
	case Result:
		err = ds.decodeResult(dstv)
	default:
		t := reflect.TypeOf(in)
		switch t.Kind() {
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16,
			reflect.Int32, reflect.Int64, reflect.String, reflect.Uint,
			reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			err = ds.decodeCustomPrimitive(dstv)
		case reflect.Ptr:
			err = ds.decodePointer(dstv)
		case reflect.Struct:
			err = ds.decodeStruct(dstv)
		case reflect.Array:
			err = ds.decodeArray(dstv)
		case reflect.Slice:
			err = ds.decodeSlice(dstv)
		case reflect.Map:
			err = ds.decodeMap(dstv)
		default:
			err = fmt.Errorf("%w: %T", ErrUnsupportedType, in)
		}
	}
	return
}

func (ds *decodeState) decodeCustomPrimitive(dstv reflect.Value) (err error) {
	in := dstv.Interface()
	inType := reflect.TypeOf(in)
	var temp reflect.Value
	switch inType.Kind() {
	case reflect.Bool:
		temp = reflect.New(reflect.TypeOf(false))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Int:
		temp = reflect.New(reflect.TypeOf(int(1)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Int8:
		temp = reflect.New(reflect.TypeOf(int8(1)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Int16:
		temp = reflect.New(reflect.TypeOf(int16(1)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Int32:
		temp = reflect.New(reflect.TypeOf(int32(1)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Int64:
		temp = reflect.New(reflect.TypeOf(int64(1)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.String:
		temp = reflect.New(reflect.TypeOf(""))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Uint:
		temp = reflect.New(reflect.TypeOf(uint(0)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Uint8:
		temp = reflect.New(reflect.TypeOf(uint8(0)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Uint16:
		temp = reflect.New(reflect.TypeOf(uint16(0)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Uint32:
		temp = reflect.New(reflect.TypeOf(uint32(0)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	case reflect.Uint64:
		temp = reflect.New(reflect.TypeOf(uint64(0)))
		err = ds.unmarshal(temp.Elem())
		if err != nil {
			break
		}
	default:
		err = fmt.Errorf("%w: %T", ErrUnsupportedType, in)
		return
	}
	dstv.Set(temp.Elem().Convert(inType))
	return
}

func (ds *decodeState) ReadByte() (byte, error) {
	b := make([]byte, 1)
	_, err := ds.Reader.Read(b)
	return b[0], err
}

func (ds *decodeState) decodeResult(dstv reflect.Value) (err error) {
	res := dstv.Interface().(Result)
	var rb byte
	rb, err = ds.ReadByte()
	if err != nil {
		return
	}
	switch rb {
	case 0x00:
		tempElem := reflect.New(reflect.TypeOf(res.ok))
		tempElem.Elem().Set(reflect.ValueOf(res.ok))
		err = ds.unmarshal(tempElem.Elem())
		if err != nil {
			return
		}
		err = res.Set(OK, tempElem.Elem().Interface())
		if err != nil {
			return
		}
		dstv.Set(reflect.ValueOf(res))
	case 0x01:
		tempElem := reflect.New(reflect.TypeOf(res.err))
		tempElem.Elem().Set(reflect.ValueOf(res.err))
		err = ds.unmarshal(tempElem.Elem())
		if err != nil {
			return
		}
		err = res.Set(Err, tempElem.Elem().Interface())
		if err != nil {
			return
		}
		dstv.Set(reflect.ValueOf(res))
	default:
		bytes, _ := io.ReadAll(ds.Reader)
		err = fmt.Errorf("%w: value: %v, bytes: %v", ErrUnsupportedResult, rb, bytes)
	}
	return
}

func (ds *decodeState) decodePointer(dstv reflect.Value) (err error) {
	var rb byte
	rb, err = ds.ReadByte()
	if err != nil {
		return
	}
	switch rb {
	case 0x00:
		// nil case
	case 0x01:
		switch dstv.IsZero() {
		case false:
			if dstv.Elem().Kind() == reflect.Ptr {
				err = ds.unmarshal(dstv.Elem().Elem())
			} else {
				err = ds.unmarshal(dstv.Elem())
			}
		case true:
			elemType := reflect.TypeOf(dstv.Interface()).Elem()
			tempElem := reflect.New(elemType)
			err = ds.unmarshal(tempElem.Elem())
			if err != nil {
				return
			}
			dstv.Set(tempElem)
		}
	default:
		bytes, _ := io.ReadAll(ds.Reader)
		err = fmt.Errorf("%w: value: %v, bytes: %v", ErrUnsupportedOption, rb, bytes)
	}
	return
}

func (ds *decodeState) decodeVaryingDataType(vdt VaryingDataType) (err error) {
	var b byte
	b, err = ds.ReadByte()
	if err != nil {
		return
	}

	val, err := vdt.ValueAt(uint(b))
	if err != nil {
		err = fmt.Errorf("%w: for key %d %v", ErrUnknownVaryingDataTypeValue, uint(b), err)
		return
	}

	tempVal := reflect.New(reflect.TypeOf(val))
	tempVal.Elem().Set(reflect.ValueOf(val))
	err = ds.unmarshal(tempVal.Elem())
	if err != nil {
		return
	}
	err = vdt.SetValue(tempVal.Elem().Interface())
	return
}

func (ds *decodeState) decodeSlice(dstv reflect.Value) (err error) {
	l, err := ds.decodeLength()
	if err != nil {
		return
	}
	in := dstv.Interface()
	temp := reflect.New(reflect.ValueOf(in).Type())
	for i := uint(0); i < l; i++ {
		tempElemType := reflect.TypeOf(in).Elem()
		tempElem := reflect.New(tempElemType).Elem()

		err = ds.unmarshal(tempElem)
		if err != nil {
			return
		}
		temp.Elem().Set(reflect.Append(temp.Elem(), tempElem))
	}
	dstv.Set(temp.Elem())

	return
}

func (ds *decodeState) decodeArray(dstv reflect.Value) (err error) {
	in := dstv.Interface()
	temp := reflect.New(reflect.ValueOf(in).Type())
	for i := 0; i < temp.Elem().Len(); i++ {
		elem := temp.Elem().Index(i)
		err = ds.unmarshal(elem)
		if err != nil {
			return
		}
	}
	dstv.Set(temp.Elem())
	return
}

// FieldIndex represents an index of a field within a struct.
type FieldIndex struct {
	fieldIndex int
}

// fieldScaleIndicesCache is a placeholder for caching information about field indices.
type fieldScaleIndicesCache struct{}

// fieldScaleIndices retrieves the indices of all exported fields in the struct.
func (c *fieldScaleIndicesCache) fieldScaleIndices(v interface{}) (reflect.Value, []FieldIndex, error) {
	value := reflect.ValueOf(v)
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("expected a struct, got %T", v)
	}

	typ := value.Type()
	var indices []FieldIndex
	for i := 0; i < value.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath == "" { // PkgPath is empty for exported fields
			indices = append(indices, FieldIndex{fieldIndex: i})
		}
	}
	return value, indices, nil
}

// A global cache variable
var cache = &fieldScaleIndicesCache{}

func (ds *decodeState) decodeMap(dstv reflect.Value) (err error) {
	numberOfTuples, err := ds.decodeLength()
	if err != nil {
		return fmt.Errorf("decoding length: %w", err)
	}
	in := dstv.Interface()

	for i := uint(0); i < numberOfTuples; i++ {
		tempKeyType := reflect.TypeOf(in).Key()
		tempKey := reflect.New(tempKeyType).Elem()
		err = ds.unmarshal(tempKey)
		if err != nil {
			return fmt.Errorf("decoding key %d of %d: %w", i+1, numberOfTuples, err)
		}

		tempElemType := reflect.TypeOf(in).Elem()
		tempElem := reflect.New(tempElemType).Elem()
		err = ds.unmarshal(tempElem)
		if err != nil {
			return fmt.Errorf("decoding value %d of %d: %w", i+1, numberOfTuples, err)
		}

		dstv.SetMapIndex(tempKey, tempElem)
	}

	return nil
}

// decodeStruct decodes a byte array representing a JAM tuple. The order of data is
// determined by the source tuple in rust, or the struct field order in a go struct
func (ds *decodeState) decodeStruct(dstv reflect.Value) (err error) {
	in := dstv.Interface()
	v, indices, err := cache.fieldScaleIndices(in)
	if err != nil {
		return fmt.Errorf("failed to get field indices: %w", err)
	}

	temp := reflect.New(v.Type()).Elem()
	for _, index := range indices {
		field := temp.Field(index.fieldIndex)
		if !field.CanInterface() {
			continue
		}

		// If the field is already set, retain the value.
		if !v.Field(index.fieldIndex).IsZero() {
			field.Set(v.Field(index.fieldIndex))
		}

		err = ds.unmarshal(field)
		if err != nil {
			return fmt.Errorf("failed to unmarshal field at index %d: %w", index.fieldIndex, err)
		}
	}
	dstv.Set(temp)
	return nil
}

// decodeBool accepts a byte array representing a JAM encoded bool and performs JAM decoding
// of the bool then returns it. If invalid returns an error
func (ds *decodeState) decodeBool(dstv reflect.Value) (err error) {
	rb, err := ds.ReadByte()
	if err != nil {
		return
	}

	var b bool
	switch rb {
	case 0x00:
	case 0x01:
		b = true
	default:
		err = fmt.Errorf("%w", errDecodeBool)
	}
	dstv.Set(reflect.ValueOf(b))
	return
}

// decodeUint will decode unsigned integers according to the JAM encoding specification
func (ds *decodeState) decodeUint(dstv reflect.Value) (err error) {
	const maxUint32 = ^uint32(0)
	const maxUint64 = ^uint64(0)
	prefix, err := ds.ReadByte()
	if err != nil {
		return fmt.Errorf("reading byte: %w", err)
	}

	in := dstv.Interface()
	temp := reflect.New(reflect.TypeOf(in))
	mode := prefix % 4
	var value uint64
	switch mode {
	case 0:
		value = uint64(prefix >> 2)
	case 1:
		buf, err := ds.ReadByte()
		if err != nil {
			return fmt.Errorf("reading byte: %w", err)
		}
		value = uint64(binary.LittleEndian.Uint16([]byte{prefix, buf}) >> 2)
		if value <= 0b0011_1111 || value > 0b0111_1111_1111_1111 {
			return fmt.Errorf("%w: %d (%b)", ErrU16OutOfRange, value, value)
		}
	case 2:
		buf := make([]byte, 3)
		_, err = ds.Read(buf)
		if err != nil {
			return fmt.Errorf("reading bytes: %w", err)
		}
		value = uint64(binary.LittleEndian.Uint32(append([]byte{prefix}, buf...)) >> 2)
		if value <= 0b0011_1111_1111_1111 || value > uint64(maxUint32>>2) {
			return fmt.Errorf("%w: %d (%b)", ErrU32OutOfRange, value, value)
		}
	case 3:
		byteLen := (prefix >> 2) + 4
		buf := make([]byte, byteLen)
		_, err = ds.Read(buf)
		if err != nil {
			return fmt.Errorf("reading bytes: %w", err)
		}
		switch byteLen {
		case 4:
			value = uint64(binary.LittleEndian.Uint32(buf))
			if value <= uint64(maxUint32>>2) {
				return fmt.Errorf("%w: %d (%b)", ErrU32OutOfRange, value, value)
			}
		case 8:
			const uintSize = 32 << (^uint(0) >> 32 & 1)
			if uintSize == 32 {
				return ErrU64NotSupported
			}
			tmp := make([]byte, 8)
			copy(tmp, buf)
			value = binary.LittleEndian.Uint64(tmp)
			if value <= maxUint64>>8 {
				return fmt.Errorf("%w: %d (%b)", ErrU64OutOfRange, value, value)
			}
		default:
			return fmt.Errorf("%w: %d", ErrCompactUintPrefixUnknown, prefix)
		}
	}
	temp.Elem().Set(reflect.ValueOf(value).Convert(reflect.TypeOf(in)))
	dstv.Set(temp.Elem())
	return
}

var (
	ErrU16OutOfRange               = errors.New("uint16 out of range")
	ErrU32OutOfRange               = errors.New("uint32 out of range")
	ErrU64OutOfRange               = errors.New("uint64 out of range")
	ErrU64NotSupported             = errors.New("uint64 is not supported")
	ErrCompactUintPrefixUnknown    = errors.New("unknown prefix for compact uint")
	ErrUnsupportedCustomPrimitive  = errors.New("UnsupportedCustomPrimitive")
	ErrResultNotSet                = errors.New("ErrResultNotSet")
	ErrUnsupportedDestination      = errors.New("unsupported destination type")
	ErrUnsupportedType             = errors.New("unsupported type")
	ErrUnsupportedResult           = errors.New("unsupported result value")
	ErrUnknownVaryingDataTypeValue = errors.New("unknown varying data type value")
	ErrUnsupportedOption           = errors.New("unsupported option")
	errDecodeBool                  = errors.New("failed to decode bool")
	errBigIntIsNil                 = errors.New("big.Int is nil")
	errUint128IsNil                = errors.New("Uint128 is nil")
)

// EncodeVaryingDataType is an interface for encoding varying data types with discriminators.
type EncodeVaryingDataType interface {
	IndexValue() (int, interface{}, error)
}

// VaryingDataType represents a generic interface for types that can vary and need a discriminator.
type VaryingDataType interface {
	ValueAt(index uint) (interface{}, error)
	SetValue(interface{}) error
}

// decodeLength is helper method which calls decodeUint and casts to int
func (ds *decodeState) decodeLength() (l uint, err error) {
	dstv := reflect.New(reflect.TypeOf(l))
	err = ds.decodeUint(dstv.Elem())
	if err != nil {
		return 0, fmt.Errorf("decoding uint: %w", err)
	}
	l = dstv.Elem().Interface().(uint)
	return
}

// decodeBytes is used to decode with a destination of []byte or string type
func (ds *decodeState) decodeBytes(dstv reflect.Value) (err error) {
	length, err := ds.decodeLength()
	if err != nil {
		return
	}

	// bytes length in encoded as Compact<u32>, so it can't be more than math.MaxUint32
	if length > math.MaxUint32 {
		return fmt.Errorf("byte array length %d exceeds max value of uint32", length)
	}

	b := make([]byte, length)

	if length > 0 {
		_, err = ds.Read(b)
		if err != nil {
			return
		}
	}

	in := dstv.Interface()
	inType := reflect.TypeOf(in)
	dstv.Set(reflect.ValueOf(b).Convert(inType))
	return
}

// decodeSmallInt is used in the decodeUint and decodeBigInt functions when the mode is <= 2
// need to pass in the first byte, since we assume it's already been read
func (ds *decodeState) decodeSmallInt(firstByte, mode byte) (out int64, err error) {
	switch mode {
	case 0:
		out = int64(firstByte >> 2)
	case 1:
		var buf byte
		buf, err = ds.ReadByte()
		if err != nil {
			break
		}
		out = int64(binary.LittleEndian.Uint16([]byte{firstByte, buf}) >> 2)
	case 2:
		buf := make([]byte, 3)
		_, err = ds.Read(buf)
		if err != nil {
			break
		}
		out = int64(binary.LittleEndian.Uint32(append([]byte{firstByte}, buf...)) >> 2)
	}
	return
}

// decodeBigInt decodes a JAM encoded byte array into a *big.Int
// Works for all integers, including ints > 2**64
func (ds *decodeState) decodeBigInt(dstv reflect.Value) (err error) {
	b, err := ds.ReadByte()
	if err != nil {
		return
	}

	var output *big.Int
	// check mode of encoding, stored at 2 least significant bits
	mode := b & 0x03
	switch {
	case mode <= 2:
		var tmp int64
		tmp, err = ds.decodeSmallInt(b, mode)
		if err != nil {
			break
		}
		output = big.NewInt(tmp)

	default:
		// >4 byte mode
		topSixBits := b >> 2
		byteLen := uint(topSixBits) + 4

		buf := make([]byte, byteLen)
		_, err = ds.Read(buf)
		if err != nil {
			err = fmt.Errorf("reading bytes: %w", err)
			break
		}
		o := reverseBytes(buf)
		output = big.NewInt(0).SetBytes(o)
	}
	dstv.Set(reflect.ValueOf(output))
	return
}

// decodeFixedWidthInt decodes integers < 2**32 by reading the bytes in little endian
func (ds *decodeState) decodeFixedWidthInt(dstv reflect.Value) (err error) {
	in := dstv.Interface()
	var out interface{}
	switch in.(type) {
	case int8:
		var b byte
		b, err = ds.ReadByte()
		if err != nil {
			return
		}
		out = int8(b)
	case uint8:
		var b byte
		b, err = ds.ReadByte()
		if err != nil {
			return
		}
		out = b
	case int16:
		buf := make([]byte, 2)
		_, err = ds.Read(buf)
		if err != nil {
			return
		}
		out = int16(binary.LittleEndian.Uint16(buf))
	case uint16:
		buf := make([]byte, 2)
		_, err = ds.Read(buf)
		if err != nil {
			return
		}
		out = binary.LittleEndian.Uint16(buf)
	case int32:
		buf := make([]byte, 4)
		_, err = ds.Read(buf)
		if err != nil {
			return
		}
		out = int32(binary.LittleEndian.Uint32(buf))
	case uint32:
		buf := make([]byte, 4)
		_, err = ds.Read(buf)
		if err != nil {
			return
		}
		out = binary.LittleEndian.Uint32(buf)
	case int64:
		buf := make([]byte, 8)
		_, err = ds.Read(buf)
		if err != nil {
			return
		}
		out = int64(binary.LittleEndian.Uint64(buf))
	case uint64:
		buf := make([]byte, 8)
		_, err = ds.Read(buf)
		if err != nil {
			return
		}
		out = binary.LittleEndian.Uint64(buf)
	default:
		err = fmt.Errorf("invalid type: %T", in)
		return
	}
	dstv.Set(reflect.ValueOf(out))
	return
}

// decodeUint128 accepts a byte array representing a JAM encoded
// common.Uint128 and performs JAM decoding of the Uint128
func (ds *decodeState) decodeUint128(dstv reflect.Value) (err error) {
	buf := make([]byte, 16)
	err = binary.Read(ds, binary.LittleEndian, buf)
	if err != nil {
		return
	}
	ui128, err := NewUint128(buf)
	if err != nil {
		return
	}
	dstv.Set(reflect.ValueOf(ui128))
	return
}

// decodeBitSequence decodes a sequence of bits packed into octets
func (ds *decodeState) decodeBitSequence(dstv reflect.Value) (err error) {
	in := dstv.Interface()
	temp := reflect.New(reflect.TypeOf(in)).Elem()

	length := dstv.Len()
	packedBytes := make([]byte, (length+7)/8)
	_, err = ds.Read(packedBytes)
	if err != nil {
		return fmt.Errorf("reading bit sequence: %w", err)
	}

	for i := 0; i < length; i++ {
		byteIndex := i / 8
		bitIndex := i % 8
		bitValue := (packedBytes[byteIndex] >> bitIndex) & 1
		temp.Index(i).SetBool(bitValue == 1)
	}

	dstv.Set(temp)
	return
}

// reverseBytes reverses the order of bytes in a slice
func reverseBytes(input []byte) []byte {
	if len(input) == 0 {
		return input
	}
	output := make([]byte, len(input))
	for i := range input {
		output[i] = input[len(input)-1-i]
	}
	return output
}

// padBytes pads the byte slice to a specific size
func padBytes(input []byte, order binary.ByteOrder) []byte {
	padded := make([]byte, 16)
	copy(padded, input)
	if order == binary.BigEndian {
		reverseBytes(padded)
	}
	return padded
}
