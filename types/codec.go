package types

import (
	"reflect"
)

type CustomEncoder interface {
	Encode() []byte
}

type CustomDecoder interface {
	Decode(data []byte) (interface{}, uint32)
}

func powerOfTwo(exp uint32) uint64 {
	var result uint64 = 1
	for i := uint32(0); i < exp; i++ {
		result *= 2
	}
	return result
}

// GP v0.3.6 eq(271)  E_l - Integer Encoding
func E_l(x uint64, l uint32) []byte {
	if l == 0 {
		return []byte{}
	} else {
		encoded := []byte{byte(x % 256)}
		encoded = append(encoded, E_l(x/256, l-1)...)
		return encoded
	}
}

// GP v0.3.6 eq(271)  E_l - Integer Decoding
func DecodeE_l(encoded []byte) uint64 {
	var x uint64 = 0
	for i := len(encoded) - 1; i >= 0; i-- {
		x = x*256 + uint64(encoded[i])
	}
	return x
}

// GP v0.3.6 eq(272)  E - Integer Encoding: general natural number serialization up to 2^64
func E(x uint64) []byte {
	if x == 0 {
		return []byte{0}
	}
	for l := uint32(0); l < 8; l++ {
		if x >= powerOfTwo(7*l) && x < powerOfTwo(7*(l+1)) {
			encoded := []byte{byte(powerOfTwo(8) - powerOfTwo(8-l) + x/powerOfTwo(8*l))}
			encoded = append(encoded, E_l(x%powerOfTwo(8*l), l)...)
			return encoded
		}
	}
	encoded := []byte{byte(powerOfTwo(8) - 1)}
	encoded = append(encoded, E_l(x, 8)...)
	return encoded
}

// GP v0.3.6 eq(272) E - Integer Decoding: general natural number serialization up to 2^64
func DecodeE(encoded []byte) (uint64, uint32) {
	firstByte := encoded[0]
	if firstByte == 0 {
		return 0, 1
	}
	if firstByte == 255 {
		return DecodeE_l(encoded[1:9]), 9
	}
	var l uint32
	for l = 0; l < 8; l++ {
		if firstByte >= byte(256-powerOfTwo(8-l)) && firstByte < byte(256-powerOfTwo(8-(l+1))) {
			x1 := uint64(firstByte) - uint64(256-powerOfTwo(8-l))
			x2 := DecodeE_l(encoded[1 : 1+l])
			x := x1*powerOfTwo(8*l) + x2
			return x, l + 1
		}
	}

	return 0, 0
}

// GP v0.3.6 eq(274) ↕x≡(|x|,x) - Length Discriminator Encoding. Maybe Reuqired later [DO NOT DELETE]
func LengthE(x []uint64) []byte {
	encoded := E(uint64(len(x)))
	for i := 0; i < len(x); i++ {
		encoded = append(encoded, E(x[i])...)
	}
	return encoded
}

 // GP v0.3.6 eq(274) ↕x≡(|x|,x) - Length Discriminator Decoding. Maybe Reuqired later [DO NOT DELETE]
func DecodeLengthE(encoded []byte) ([]uint64, uint32) {
	length, l := DecodeE([]byte{encoded[0]})
	var T []uint64
	for i := 0; i < int(length); i++ {
		x, len := DecodeE(encoded[l:])
		T = append(T, x)
		l += len
	}
	return T, l
}

func CheckCustomEncode(data interface{}) (bool, []byte) {
	if data == nil {
		return false, []byte{}
	}
	if encoder, ok := data.(CustomEncoder); ok {
		return true, encoder.Encode()
	}
	return false, []byte{}
}

func CheckCustomDecode(data []byte, t reflect.Type) (bool, interface{}, uint32) {
	// Create a zero value of the type
	instance := reflect.New(t).Elem().Interface()
	// Check if the type implements the CustomDecoder interface
	if decoder, ok := instance.(CustomDecoder); ok {
		decoded, length := decoder.Decode(data)
		return true, decoded, length
	}
	// Fallback to default behavior if not implemented
	return false, nil, 0
}

func Encode(data interface{}) []byte {
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Ptr {
		CheckCustomEncode(data)
		customEncodeRequired, customEncoded := CheckCustomEncode(data)
		if customEncodeRequired {
			return customEncoded
		}
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return []byte{1}
		}
		return []byte{0}
	case reflect.Uint:
		return E(v.Uint())
	case reflect.Uint8:
		return E_l(uint64(v.Uint()), 1)
	case reflect.Uint16:
		return E_l(uint64(v.Uint()), 2)
	case reflect.Uint32:
		return E_l(uint64(v.Uint()), 4)
	case reflect.Uint64:
		return E_l(uint64(v.Uint()), 8)
	case reflect.String:
		uint64Slice := make([]uint64, 0)
		for _, c := range v.String() {
			uint64Slice = append(uint64Slice, uint64(c))
		}
		encoded := E(uint64(len(uint64Slice)))
		for i := 0; i < len(uint64Slice); i++ {
			encoded = append(encoded, E(uint64Slice[i])...)
		}
		return encoded

	// GP v0.3.6 eq(273) Sequence Encoding
	case reflect.Array:
		var encoded []byte
		for i := 0; i < v.Len(); i++ {
			// GP v0.3.6 eq(268)  "The serialization of an octet-sequence as itself"
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, []byte{byte(v.Index(i).Uint())}...)
			} else {
				encoded = append(encoded, Encode(v.Index(i).Interface())...)
			}
		}
		return encoded

	case reflect.Slice:
		// GP v0.3.6 eq(274) Length Discriminator Encoding
		encoded := E(uint64(v.Len()))
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, []byte{byte(v.Index(i).Uint())}...)
			} else {
				encoded = append(encoded, Encode(v.Index(i).Interface())...)
			}
		}
		return encoded

		// GP v0.3.6 eq(269) Concatenation Rule
	case reflect.Struct:
		var encoded []byte
		for i := 0; i < v.NumField(); i++ {
			encoded = append(encoded, Encode(v.Field(i).Interface())...)
		}
		return encoded

		// GP v0.3.6 eq(270) Tuples Rule
	case reflect.Ptr:
		if v.IsNil() {
			return []byte{0}
		}
		return Encode(v.Elem().Interface())
	}
	return []byte{}
}

func Decode(data []byte, t reflect.Type) (interface{}, uint32) {
	length := uint32(0)
	v := reflect.New(t).Elem()
	if t.Kind() != reflect.Ptr {
		customDecodeRequired, decoded, customLength := CheckCustomDecode(data, t)
		if customDecodeRequired {
			return decoded, customLength
		}
	}

	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(data[0] == 1)
		length = 1
	case reflect.Uint:
		x, l := DecodeE(data)
		v.SetUint(x)
		length = l
	case reflect.Uint8:
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 1
	case reflect.Uint16:
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 2
	case reflect.Uint32:
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 4
	case reflect.Uint64:
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 8
	case reflect.String:
		str_len, length := DecodeE([]byte{data[0]})
		var T []uint64
		for i := 0; i < int(str_len); i++ {
			x, l := DecodeE(data[length:])
			T = append(T, x)
			length += l
		}
		var str string
		for _, c := range T {
			str += string(rune(c))
		}
		v.SetString(str)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				v.Index(i).Set(reflect.ValueOf(data[length]))
				length++
			} else {
				elem, l := Decode(data[length:], v.Index(i).Type())
				v.Index(i).Set(reflect.ValueOf(elem))
				length += l
			}
		}

	// GP v0.3.6 eq(274) Length Discriminator Decoding
	case reflect.Slice:
		item_len, l := DecodeE(data)

		v.Set(reflect.MakeSlice(t, int(item_len), int(item_len)))
		length += l
		for i := 0; i < int(item_len); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				v.Index(i).Set(reflect.ValueOf(data[length]))
				length++
			} else {
				elem, l := Decode(data[length:], v.Index(i).Type())
				v.Index(i).Set(reflect.ValueOf(elem))
				length += l
			}
		}

	// GP v0.3.6 eq(269) Concatenation Rule
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			elem, l := Decode(data[length:], v.Field(i).Type())
			v.Field(i).Set(reflect.ValueOf(elem))
			length += l
		}

	// GP v0.3.6 eq(270) Tuples Rule
	case reflect.Ptr:
		if data[length] == 0 {
			length++
			v.Set(reflect.Zero(t))
		} else {
			ptrType := t.Elem()
			ptr := reflect.New(ptrType)
			elem, l := Decode(data[length:], ptrType)
			ptr.Elem().Set(reflect.ValueOf(elem))
			v.Set(ptr)
			length += l
		}
	}
	return v.Interface(), length
}
