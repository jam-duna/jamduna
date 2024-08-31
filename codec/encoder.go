package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"reflect"
)

// Encoder scale encodes to a given io.Writer.
type Encoder struct {
	encodeState
}

// NewEncoder creates a new encoder with the given writer.
func NewEncoder(writer io.Writer) (encoder *Encoder) {
	return &Encoder{
		encodeState: encodeState{
			Writer:                 writer,
			fieldScaleIndicesCache: cache,
		},
	}
}

// Encode scale encodes value to the encoder writer.
func (e *Encoder) Encode(value interface{}) (err error) {
	return e.marshal(value)
}

// Marshal takes in an interface{} and attempts to marshal into []byte
func Marshal(v interface{}) (b []byte, err error) {
	buffer := bytes.NewBuffer(nil)
	es := encodeState{
		Writer:                 buffer,
		fieldScaleIndicesCache: cache,
	}
	err = es.marshal(v)
	if err != nil {
		return
	}
	b = buffer.Bytes()
	return
}

// Marshaler is the interface for custom JAM marshalling for a given type
type Marshaler interface {
	MarshalJAM() ([]byte, error)
}

// MustMarshal runs Marshal and panics on error.
func MustMarshal(v interface{}) (b []byte) {
	b, err := Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

type encodeState struct {
	io.Writer
	*fieldScaleIndicesCache
}

func (es *encodeState) marshal(in interface{}) (err error) {
	marshaler, ok := in.(Marshaler)
	if ok {
		var bytes []byte
		bytes, err = marshaler.MarshalJAM()
		if err != nil {
			return
		}
		_, err = es.Write(bytes)
		return
	}

	vdt, ok := in.(EncodeVaryingDataType)
	if ok {
		err = es.encodeVaryingDataType(vdt)
		return
	}

	switch in := in.(type) {
	case int:
		err = es.encodeUint(uint(in))
	case uint:
		err = es.encodeUint(in)
	case int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		err = es.encodeFixedWidthInt(in)
	case *big.Int:
		err = es.encodeBigInt(in)
	case *Uint128:
		err = es.encodeUint128(in)
	case []byte:
		err = es.encodeBytes(in)
	case string:
		err = es.encodeBytes([]byte(in))
	case bool:
		err = es.encodeBool(in)
	case []bool:
		err = es.encodeBitSequence(in)
	case Result:
		err = es.encodeResult(in)
	default:
		switch reflect.TypeOf(in).Kind() {
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16,
			reflect.Int32, reflect.Int64, reflect.String, reflect.Uint,
			reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			err = es.encodeCustomPrimitive(in)
		case reflect.Ptr:
			// Assuming that anything that is a pointer is an Option to capture {nil, T}
			elem := reflect.ValueOf(in).Elem()
			switch elem.IsValid() {
			case false:
				_, err = es.Write([]byte{0})
			default:
				_, err = es.Write([]byte{1})
				if err != nil {
					return
				}
				err = es.marshal(elem.Interface())
			}
		case reflect.Struct:
			err = es.encodeStruct(in)
		case reflect.Array:
			err = es.encodeArray(in)
		case reflect.Slice:
			err = es.encodeSlice(in)
		case reflect.Map:
			err = es.encodeMap(in)
		default:
			err = fmt.Errorf("%w: %T", ErrUnsupportedType, in)
		}
	}
	return
}

// encodeCustomPrimitive encodes basic Go primitives
func (es *encodeState) encodeCustomPrimitive(in interface{}) (err error) {
	switch reflect.TypeOf(in).Kind() {
	case reflect.Bool:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(false)).Interface()
	case reflect.Int:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(int(0))).Interface()
	case reflect.Int8:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(int8(0))).Interface()
	case reflect.Int16:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(int16(0))).Interface()
	case reflect.Int32:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(int32(0))).Interface()
	case reflect.Int64:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(int64(0))).Interface()
	case reflect.String:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf("")).Interface()
	case reflect.Uint:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(uint(0))).Interface()
	case reflect.Uint8:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(uint8(0))).Interface()
	case reflect.Uint16:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(uint16(0))).Interface()
	case reflect.Uint32:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(uint32(0))).Interface()
	case reflect.Uint64:
		in = reflect.ValueOf(in).Convert(reflect.TypeOf(uint64(0))).Interface()
	default:
		err = fmt.Errorf("%w: %T", ErrUnsupportedCustomPrimitive, in)
		return
	}
	err = es.marshal(in)
	return
}

// encodeResult encodes a Result type
func (es *encodeState) encodeResult(res Result) (err error) {
	if !res.IsSet() {
		err = fmt.Errorf("%w: %+v", ErrResultNotSet, res)
		return
	}

	var in interface{}
	switch res.mode {
	case OK:
		_, err = es.Write([]byte{0})
		if err != nil {
			return
		}
		in = res.ok
	case Err:
		_, err = es.Write([]byte{1})
		if err != nil {
			return
		}
		in = res.err
	}
	switch in := in.(type) {
	//case empty:
	default:
		err = es.marshal(in)
	}
	return
}

// encodeVaryingDataType encodes varying data types with discriminator
func (es *encodeState) encodeVaryingDataType(vdt EncodeVaryingDataType) (err error) {
	index, value, err := vdt.IndexValue()
	if err != nil {
		return
	}
	_, err = es.Write([]byte{byte(index)})
	if err != nil {
		return
	}
	err = es.marshal(value)
	return
}

// encodeSlice encodes a slice with length prefix
func (es *encodeState) encodeSlice(in interface{}) (err error) {
	v := reflect.ValueOf(in)
	err = es.encodeLength(v.Len())
	if err != nil {
		return
	}
	for i := 0; i < v.Len(); i++ {
		err = es.marshal(v.Index(i).Interface())
		if err != nil {
			return
		}
	}
	return
}

// encodeArray encodes an array without length prefix
func (es *encodeState) encodeArray(in interface{}) (err error) {
	v := reflect.ValueOf(in)
	for i := 0; i < v.Len(); i++ {
		err = es.marshal(v.Index(i).Interface())
		if err != nil {
			return
		}
	}
	return
}

// encodeMap encodes a map with key-value pairs and length prefix
func (es *encodeState) encodeMap(in interface{}) (err error) {
	v := reflect.ValueOf(in)
	err = es.encodeLength(v.Len())
	if err != nil {
		return fmt.Errorf("encoding length: %w", err)
	}

	iterator := v.MapRange()
	for iterator.Next() {
		key := iterator.Key()
		err = es.marshal(key.Interface())
		if err != nil {
			return fmt.Errorf("encoding map key: %w", err)
		}

		mapValue := iterator.Value()
		if !mapValue.CanInterface() {
			continue
		}

		err = es.marshal(mapValue.Interface())
		if err != nil {
			return fmt.Errorf("encoding map value: %w", err)
		}
	}
	return nil
}

// encodeBigInt encodes a big.Int with JAM encoding
func (es *encodeState) encodeBigInt(i *big.Int) (err error) {
	switch {
	case i == nil:
		err = fmt.Errorf("%w", errBigIntIsNil)
	case i.Cmp(new(big.Int).Lsh(big.NewInt(1), 6)) < 0:
		err = binary.Write(es, binary.LittleEndian, uint8(i.Int64()<<2))
	case i.Cmp(new(big.Int).Lsh(big.NewInt(1), 14)) < 0:
		err = binary.Write(es, binary.LittleEndian, uint16(i.Int64()<<2)+1)
	case i.Cmp(new(big.Int).Lsh(big.NewInt(1), 30)) < 0:
		err = binary.Write(es, binary.LittleEndian, uint32(i.Int64()<<2)+2)
	default:
		numBytes := len(i.Bytes())
		topSixBits := uint8(numBytes - 4)
		lengthByte := topSixBits<<2 + 3

		// write byte which encodes mode and length
		err = binary.Write(es, binary.LittleEndian, lengthByte)
		if err == nil {
			// write integer itself
			err = binary.Write(es, binary.LittleEndian, reverseBytes(i.Bytes()))
			if err != nil {
				err = fmt.Errorf("writing bytes %s: %w", i, err)
			}
		}
	}
	return
}

// encodeBool encodes a boolean value
func (es *encodeState) encodeBool(l bool) (err error) {
	switch l {
	case true:
		_, err = es.Write([]byte{0x01})
	case false:
		_, err = es.Write([]byte{0x00})
	}
	return
}

// encodeBytes encodes a byte slice with length prefix
func (es *encodeState) encodeBytes(b []byte) (err error) {
	err = es.encodeLength(len(b))
	if err != nil {
		return
	}

	_, err = es.Write(b)
	return
}

// encodeFixedWidthInt encodes fixed width integers
func (es *encodeState) encodeFixedWidthInt(i interface{}) (err error) {
	switch i := i.(type) {
	case int8:
		err = binary.Write(es, binary.LittleEndian, byte(i))
	case uint8:
		err = binary.Write(es, binary.LittleEndian, i)
	case int16:
		err = binary.Write(es, binary.LittleEndian, uint16(i))
	case uint16:
		err = binary.Write(es, binary.LittleEndian, i)
	case int32:
		err = binary.Write(es, binary.LittleEndian, uint32(i))
	case uint32:
		err = binary.Write(es, binary.LittleEndian, i)
	case int64:
		err = binary.Write(es, binary.LittleEndian, uint64(i))
	case uint64:
		err = binary.Write(es, binary.LittleEndian, i)
	default:
		err = fmt.Errorf("invalid type: %T", i)
	}
	return
}

// encodeStruct encodes struct fields recursively
func (es *encodeState) encodeStruct(in interface{}) (err error) {
	v, indices, err := es.fieldScaleIndices(in)
	if err != nil {
		return
	}
	for _, i := range indices {
		field := v.Field(i.fieldIndex)
		if !field.CanInterface() {
			continue
		}
		err = es.marshal(field.Interface())
		if err != nil {
			return
		}
	}
	return
}

// encodeLength encodes the length of a collection
func (es *encodeState) encodeLength(l int) (err error) {
	return es.encodeUint(uint(l))
}

// encodeUint encodes unsigned integers with JAM encoding
func (es *encodeState) encodeUint(i uint) (err error) {
	switch {
	case i < 1<<6:
		err = binary.Write(es, binary.LittleEndian, byte(i)<<2)
	case i < 1<<14:
		err = binary.Write(es, binary.LittleEndian, uint16(i<<2)+1)
	case i < 1<<30:
		err = binary.Write(es, binary.LittleEndian, uint32(i<<2)+2)
	default:
		o := make([]byte, 8)
		m := i
		var numBytes int
		for numBytes = 0; numBytes < 256 && m != 0; numBytes++ {
			m = m >> 8
		}

		topSixBits := uint8(numBytes - 4)
		lengthByte := topSixBits<<2 + 3

		err = binary.Write(es, binary.LittleEndian, lengthByte)
		if err == nil {
			binary.LittleEndian.PutUint64(o, uint64(i))
			err = binary.Write(es, binary.LittleEndian, o[0:numBytes])
		}
	}
	return
}

// Bytes converts the Uint128 value into a byte slice in little-endian order.
func (i *Uint128) Bytes() []byte {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[:8], i.Low)
	binary.LittleEndian.PutUint64(buf[8:], i.High)
	return buf
}

// encodeUint128 encodes a Uint128 value
func (es *encodeState) encodeUint128(i *Uint128) (err error) {
	if i == nil {
		err = fmt.Errorf("%w", errUint128IsNil)
		return
	}
	err = binary.Write(es, binary.LittleEndian, padBytes(i.Bytes(), binary.LittleEndian))
	return
}

// encodeBitSequence encodes a sequence of bits as a packed octet stream
func (es *encodeState) encodeBitSequence(bits []bool) (err error) {
	if len(bits) == 0 {
		_, err = es.Write([]byte{})
		return
	}

	var packedBytes []byte
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j := 0; j < 8 && i+j < len(bits); j++ {
			if bits[i+j] {
				b |= 1 << j
			}
		}
		packedBytes = append(packedBytes, b)
	}

	_, err = es.Write(packedBytes)
	return
}
