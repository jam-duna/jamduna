package types

import (
	// "fmt"
	"reflect"
)

func powerOfTwo(exp uint32) uint64 {
	var result uint64 = 1
	for i := uint32(0); i < exp; i++ {
		result *= 2
	}
	return result
}

func E_l(x uint64, l uint32) []byte {
	if l == 0 {
		return []byte{}
	} else {
		encoded := []byte{byte(x % 256)}
		encoded = append(encoded, E_l(x/256, l-1)...)
		return encoded
	}
}

// func E4(x uint64) []byte {
// 	if x == 0 {
// 		return []byte{0}
// 	}
// 	for l := uint32(0); l < 3; l++ {
// 		if x >= powerOfTwo(7*l) && x < powerOfTwo(7*(l+1)) {
// 			encoded := []byte{byte(powerOfTwo(8) - powerOfTwo(8-l) + (x / powerOfTwo(8*l)))}
// 			encoded = append(encoded, E_l(x%powerOfTwo(8*l), l)...)
// 			return encoded
// 		}
// 	}
// 	if x >= powerOfTwo(21) && x < powerOfTwo(29) {
// 		encoded := []byte{byte(powerOfTwo(8) - powerOfTwo(5) + x/powerOfTwo(24))}
// 		encoded = append(encoded, E_l(x%powerOfTwo(24), 3)...)
// 		return encoded
// 	}
// 	return nil
// }

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

func Seq_E(T []uint64) []byte {
	encoded := []byte{}
	for _, x := range T {
		encoded = append(encoded, E(x)...)
	}
	return encoded
}

func LengthE(x []uint64) []byte {
	encoded := E(uint64(len(x)))
	for i := 0; i < len(x); i++ {
		encoded = append(encoded, E(x[i])...)
	}
	return encoded
}

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

func DecodeE_l(encoded []byte) uint64 {
	var x uint64 = 0
	for i := len(encoded) - 1; i >= 0; i-- {
		x = x*256 + uint64(encoded[i])
	}
	return x
}

// func DecodeE4(encoded []byte) (uint64, uint32) {
// 	firstByte := encoded[0]
// 	if firstByte == 0 {
// 		return 0, 1
// 	}

// 	var l uint32
// 	for l = 0; l < 3; l++ {
// 		if firstByte >= byte(256-powerOfTwo(8-l)) && firstByte < byte(256-powerOfTwo(8-(l+1))) {
// 			x1 := uint64(firstByte) - uint64(256-powerOfTwo(8-l))
// 			x2 := DecodeE_l(encoded[1 : 1+l])
// 			x := x1*powerOfTwo(8*l) + x2
// 			return x, l + 1
// 		}
// 	}

// 	if firstByte >= byte(256-32) {
// 		x1 := uint64(firstByte) - (256 - 32)
// 		x2 := DecodeE_l(encoded[1 : 1+3])
// 		x := x1*powerOfTwo(24) + x2
// 		return x, 3 + 1
// 	}

// 	return 0, 0
// }

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

func DecodeSeq_E(encoded []byte) []uint64 {
	if len(encoded) == 0 {
		return nil
	}

	var T []uint64
	length := uint32(0)
	for int(length) < len(encoded) {
		x, l := DecodeE(encoded[length:])
		T = append(T, x)
		length += l
	}
	return T
}

func CheckCustomEncode(data interface{}) (bool, []byte) {
	switch v := data.(type) {
	case TicketsMark:
		return true, v.Encode()
	case EpochMark:
		return true, v.Encode()
	case Prerequisite:
		return true, v.Encode()
	case Result:
		return true, v.Encode()
	default:
		return false, []byte{}
	}
}

func CheckCustomDecode(data []byte, t reflect.Type) (bool, interface{}, uint32) {
	switch t {
	case reflect.TypeOf(TicketsMark{}):
		decoded, length := TicketsMarkDecode(data, t)
		return true, decoded, length
	case reflect.TypeOf(EpochMark{}):
		decoded, length := EpochMarkDecode(data, t)
		return true, decoded, length
	case reflect.TypeOf(Prerequisite{}):
		decoded, length := PrerequisiteDecode(data, t)
		return true, decoded, length
	case reflect.TypeOf(Result{}):
		decoded, length := ResultDecode(data, t)
		return true, decoded, length
	default:
		return false, nil, 0
	}
}

func Encode(data interface{}) []byte {
	CheckCustomEncode(data)
	customEncodeRequired, customEncoded := CheckCustomEncode(data)
	if customEncodeRequired {
		return customEncoded
	}
	v := reflect.ValueOf(data)

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
		// use LengthE
		uint64Slice := make([]uint64, 0)
		for _, c := range v.String() {
			uint64Slice = append(uint64Slice, uint64(c))
		}
		encoded := E(uint64(len(uint64Slice)))
		for i := 0; i < len(uint64Slice); i++ {
			encoded = append(encoded, E(uint64Slice[i])...)
		}
		return encoded
	case reflect.Array:
		var encoded []byte
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, []byte{byte(v.Index(i).Uint())}...)
			} else {
				encoded = append(encoded, Encode(v.Index(i).Interface())...)
			}
		}
		return encoded
	case reflect.Slice:
		encoded := E(uint64(v.Len()))
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, []byte{byte(v.Index(i).Uint())}...)
			} else {
				encoded = append(encoded, Encode(v.Index(i).Interface())...)
			}
		}
		return encoded
	case reflect.Struct:
		var encoded []byte
		for i := 0; i < v.NumField(); i++ {
			encoded = append(encoded, Encode(v.Field(i).Interface())...)
		}
		return encoded
	case reflect.Ptr:
		if v.IsNil() {
			return []byte{0}
		}
		return Encode(v.Elem().Interface())
		// case reflect.Map:
		// 	if _, ok := data.(map[string]interface{})["ok"]; ok {
		// 		ok_byte := common.FromHex(data.(map[string]interface{})["ok"].(string))
		// 		return append([]byte{0}, Encode(ok_byte)...)
		// 	} else if _, ok := data.(map[string]interface{})["out-of-gas"]; ok {
		// 		return []byte{1}
		// 	} else if _, ok := data.(map[string]interface{})["panic"]; ok {
		// 		return []byte{2}
		// 	} else if _, ok := data.(map[string]interface{})["bad-code"]; ok {
		// 		return []byte{3}
		// 	} else if _, ok := data.(map[string]interface{})["code-oversize"]; ok {
		// 		return []byte{4}
		// 	}
	}
	return []byte{}
}

func Decode(data []byte, t reflect.Type) (interface{}, uint32) {
	length := uint32(0)
	customDecodeRequired, decoded, customLength := CheckCustomDecode(data, t)
	if customDecodeRequired {
		return decoded, customLength
	}
	v := reflect.New(t).Elem()
	/*
		if t == reflect.TypeOf(&EpochMark{}) {
			if data[0] == 0 {
				v.Set(reflect.Zero(t))
				return v.Interface(), 1
			} else {
				length++
				ptrType := t.Elem()
				ptr := reflect.New(ptrType)
				elem, l := Decode(data[length:], ptrType)
				ptr.Elem().Set(reflect.ValueOf(elem))
				v.Set(ptr)
				length += l
				return v.Interface(), length
			}
		}

		if t.Kind() == reflect.Array && t.Elem() == reflect.TypeOf(&TicketBody{}) {
			if data[0] == 0 {
				return reflect.Zero(t).Interface(), 1
			} else {
				arr := reflect.New(t).Elem()
				length++
				for i := 0; i < t.Len(); i++ {
					elem, l := Decode(data[length:], t.Elem())
					arr.Index(i).Set(reflect.ValueOf(elem))
					length += l
				}
				return arr.Interface(), length
			}
		}
	*/

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
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			elem, l := Decode(data[length:], v.Field(i).Type())
			v.Field(i).Set(reflect.ValueOf(elem))
			length += l
		}
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
		// case reflect.Map:
		// 	v.Set(reflect.MakeMap(reflect.TypeOf(map[string]interface{}{})))
		// 	if data[0] == 0 {
		// 		length++
		// 		ok_byte, l := Decode(data[length:], reflect.TypeOf([]byte{}))
		// 		v.SetMapIndex(reflect.ValueOf("ok"), reflect.ValueOf(ok_byte))
		// 		length += l
		// 	} else if data[0] == 1 {
		// 		length++
		// 		v.SetMapIndex(reflect.ValueOf("out-of-gas"), reflect.ValueOf(nil))
		// 	} else if data[0] == 2 {
		// 		length++
		// 		v.SetMapIndex(reflect.ValueOf("panic"), reflect.ValueOf(nil))
		// 	} else if data[0] == 3 {
		// 		length++
		// 		v.SetMapIndex(reflect.ValueOf("bad-code"), reflect.ValueOf(nil))
		// 	} else if data[0] == 4 {
		// 		length++
		// 		v.SetMapIndex(reflect.ValueOf("code-oversize"), reflect.ValueOf(nil))
		// 	}
	}
	return v.Interface(), length
}
