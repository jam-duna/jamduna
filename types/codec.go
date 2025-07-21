package types

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
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
	if len(encoded) == 0 {
		return 0, 0
	}

	firstByte := encoded[0]
	if firstByte == 0 {
		return 0, 1
	}
	if firstByte == 255 {
		if len(encoded) < 9 {
			// Not enough bytes for DecodeE_l(encoded[1:9])
			return 0, 0
		}
		return DecodeE_l(encoded[1:9]), 9
	}

	var l uint32
	for l = 0; l < 8; l++ {
		if firstByte >= byte(256-powerOfTwo(8-l)) && firstByte < byte(256-powerOfTwo(8-(l+1))) {
			if len(encoded) < int(1+l) {
				// Not enough bytes for DecodeE_l(encoded[1:1+l])
				return 0, 0
			}
			x1 := uint64(firstByte) - uint64(256-powerOfTwo(8-l))
			x2 := DecodeE_l(encoded[1 : 1+l])
			x := x1*powerOfTwo(8*l) + x2
			return x, l + 1
		}
	}

	// If no match found, return zero
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

	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
		}
	}()

	if encoder, ok := data.(CustomEncoder); ok {
		return true, encoder.Encode()
	}

	return false, []byte{}
}

func CheckCustomDecode(data []byte, t reflect.Type) (bool, interface{}, uint32) {
	instance := reflect.New(t).Elem().Interface()

	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
		}
	}()

	if decoder, ok := instance.(CustomDecoder); ok {
		decoded, length := decoder.Decode(data)
		return true, decoded, length
	}

	return false, nil, 0
}

func EncodeAsHex(data interface{}) string {
	// Ignore codec err
	ecodedBytes, err := Encode(data)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("0x%x", ecodedBytes)
}

func Encode(data interface{}) ([]byte, error) {
	v := reflect.ValueOf(data)
	customEncodeRequired, customEncoded := CheckCustomEncode(data)
	if customEncodeRequired {
		if len(customEncoded) == 0 {
			return []byte{}, nil
		}
		return customEncoded, nil
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case reflect.Uint:
		return E(v.Uint()), nil
	case reflect.Uint8:
		return E_l(uint64(v.Uint()), 1), nil
	case reflect.Uint16:
		return E_l(uint64(v.Uint()), 2), nil
	case reflect.Uint32:
		return E_l(uint64(v.Uint()), 4), nil
	case reflect.Uint64:
		return E_l(uint64(v.Uint()), 8), nil
	case reflect.String:
		uint64Slice := make([]uint64, 0)
		for _, c := range v.String() {
			uint64Slice = append(uint64Slice, uint64(c))
		}
		encoded := E(uint64(len(uint64Slice)))
		for i := 0; i < len(uint64Slice); i++ {
			encoded = append(encoded, E(uint64Slice[i])...)
		}
		return encoded, nil

	// GP v0.3.6 eq(273) Sequence Encoding
	case reflect.Array:
		var encoded []byte
		for i := 0; i < v.Len(); i++ {
			// GP v0.3.6 eq(268)  "The serialization of an octet-sequence as itself"
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, []byte{byte(v.Index(i).Uint())}...)
			} else {
				encodedVi, err := Encode(v.Index(i).Interface())
				if err != nil {
					return nil, err
				}
				encoded = append(encoded, encodedVi...)
			}
		}
		return encoded, nil

	case reflect.Slice:
		// GP v0.3.6 eq(274) Length Discriminator Encoding
		encoded := E(uint64(v.Len()))
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, []byte{byte(v.Index(i).Uint())}...)
			} else {
				encodedVi, err := Encode(v.Index(i).Interface())
				if err != nil {
					return nil, err
				}
				encoded = append(encoded, encodedVi...)
			}
		}
		return encoded, nil

	// GP v0.3.6 eq(269) Concatenation Rule
	case reflect.Struct:
		var encoded []byte
		for i := 0; i < v.NumField(); i++ {
			encodedVi, err := Encode(v.Field(i).Interface())
			if err != nil {
				field := v.Type().Field(i) //good to debug
				tag := field.Tag.Get("json")
				name := field.Name
				typeName := field.Type.Name()
				return nil, fmt.Errorf("[Error From Codec Encode]%s , type: %s, name: %s, tag: %s", err, typeName, name, tag)
			}
			encoded = append(encoded, encodedVi...)
		}
		return encoded, nil

	// GP v0.3.6 eq(270) Tuples Rule
	case reflect.Ptr:
		if v.IsNil() {
			return []byte{0}, nil
		}
		return Encode(v.Elem().Interface())
	case reflect.Map:
		keys := v.MapKeys()
		if len(keys) == 0 {
			return []byte{0}, nil
		}

		sort.Slice(keys, func(i, j int) bool {
			switch keys[i].Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return keys[i].Int() < keys[j].Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return keys[i].Uint() < keys[j].Uint()
			case reflect.String:
				return keys[i].String() < keys[j].String()
			default:
				return fmt.Sprintf("%v", keys[i]) < fmt.Sprintf("%v", keys[j])
			}
		})

		type kvPair struct {
			Key   interface{}
			Value interface{}
		}
		var sortedKVPairs []kvPair
		for _, key := range keys {
			sortedKVPairs = append(sortedKVPairs, kvPair{
				Key:   key.Interface(),
				Value: v.MapIndex(key).Interface(),
			})
		}
		return Encode(sortedKVPairs)

	default:
		return []byte{}, fmt.Errorf("unsupported type: %s", v.Kind().String())
	}
}

func Decode(data []byte, t reflect.Type) (interface{}, uint32, error) {
	length := uint32(0)
	v := reflect.New(t).Elem()
	customDecodeRequired, decoded, customLength := CheckCustomDecode(data, t)
	if customDecodeRequired {
		if len(data) < int(customLength) {
			return nil, 0, fmt.Errorf("data length insufficient for custom decode")
		}
		return decoded, customLength, nil
	}

	switch v.Kind() {
	case reflect.Bool:
		if len(data) < 1 {
			return nil, 0, fmt.Errorf("data length insufficient for bool")
		}
		v.SetBool(data[0] == 1)
		length = 1
	case reflect.Uint:
		x, l := DecodeE(data)
		if len(data) < int(l) {
			return nil, 0, fmt.Errorf("data length insufficient for uint")
		}
		v.SetUint(x)
		length = l
	case reflect.Uint8:
		if len(data) < 1 {
			return nil, 0, fmt.Errorf("data length insufficient for uint8")
		}
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 1
	case reflect.Uint16:
		if len(data) < 2 {
			return nil, 0, fmt.Errorf("data length insufficient for uint16")
		}
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 2
	case reflect.Uint32:
		if len(data) < 4 {
			return nil, 0, fmt.Errorf("data length insufficient for uint32")
		}
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 4
	case reflect.Uint64:
		if len(data) < 8 {
			return nil, 0, fmt.Errorf("data length insufficient for uint64")
		}
		x := DecodeE_l(data)
		v.SetUint(x)
		length = 8
	case reflect.String:
		str_len, l := DecodeE(data)
		if len(data) < int(length+l) {
			return nil, 0, fmt.Errorf("data length insufficient for decoding string length")
		}
		length += l
		var T []uint64
		for i := 0; i < int(str_len); i++ {
			if len(data) < int(length+1) {
				return nil, 0, fmt.Errorf("data length insufficient for string character")
			}
			x, l := DecodeE(data[length:])
			if len(data) < int(length+l) {
				return nil, 0, fmt.Errorf("data length insufficient for string character decoding")
			}
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
			if len(data) < int(length+1) {
				return nil, 0, fmt.Errorf("data length insufficient for array element")
			}
			if v.Index(i).Kind() == reflect.Uint8 {
				v.Index(i).Set(reflect.ValueOf(data[length]))
				length++
			} else {
				if len(data[length:]) < 1 {
					return nil, 0, fmt.Errorf("data length insufficient for array element decoding")
				}
				elem, l, err := Decode(data[length:], v.Index(i).Type())
				if err != nil {
					return nil, 0, err
				}
				if len(data) < int(length+l) {
					return nil, 0, fmt.Errorf("data length insufficient for array element decoding")
				}
				if elem != nil {
					v.Index(i).Set(reflect.ValueOf(elem))
				}
				length += l
			}
		}
	case reflect.Slice:
		item_len, l := DecodeE(data)
		if len(data) < int(length+l) {
			return nil, 0, fmt.Errorf("data length insufficient for slice length")
		}
		if item_len > 10000000 {
			return nil, 0, fmt.Errorf("item_len too large %d", item_len)
		}
		v.Set(reflect.MakeSlice(t, int(item_len), int(item_len)))
		length += l
		for i := 0; i < int(item_len); i++ {
			if len(data) < int(length+1) {
				return nil, 0, fmt.Errorf("data length insufficient for slice element")
			}
			if v.Index(i).Kind() == reflect.Uint8 {
				v.Index(i).Set(reflect.ValueOf(data[length]))
				length++
			} else {
				if len(data[length:]) < 1 {
					return nil, 0, fmt.Errorf("data length insufficient for slice element decoding")
				}
				elem, l, err := Decode(data[length:], v.Index(i).Type())
				if err != nil {
					return nil, 0, err
				}
				if len(data) < int(length+l) {
					return nil, 0, fmt.Errorf("data length insufficient for slice element decoding")
				}
				if elem != nil {
					v.Index(i).Set(reflect.ValueOf(elem))
				}
				length += l
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if len(data[length:]) < 1 {
				return nil, 0, fmt.Errorf("data length insufficient for struct field: %s", v.Type().Field(i).Name)
			}
			elem, l, err := Decode(data[length:], v.Field(i).Type())
			if err != nil {
				field := t.Field(i)
				tag := field.Tag.Get("json")
				name := field.Name
				typeName := field.Type.Name()
				return nil, 0, fmt.Errorf("[Error From Codec Decode]%s , type: %s, name: %s, tag: %s", err, typeName, name, tag)

			}
			if len(data) < int(length+l) {
				return nil, 0, fmt.Errorf("data length insufficient for struct field decoding")
			}
			if elem != nil {
				v.Field(i).Set(reflect.ValueOf(elem))
			}
			length += l
		}
	case reflect.Ptr:
		if len(data) < int(length+1) {
			return nil, 0, fmt.Errorf("data length insufficient for pointer indicator")
		}
		if data[length] == 0 {
			length++
			v.Set(reflect.Zero(t))
		} else {
			ptrType := t.Elem()
			if len(data[length:]) < 1 {
				return nil, 0, fmt.Errorf("data length insufficient for pointer content decoding")
			}
			ptr := reflect.New(ptrType)
			elem, l, err := Decode(data[length:], ptrType)
			if err != nil {
				return nil, 0, err
			}
			if len(data) < int(length+l) {
				return nil, 0, fmt.Errorf("data length insufficient for pointer content")
			}
			if elem != nil {
				ptr.Elem().Set(reflect.ValueOf(elem))
			}
			v.Set(ptr)
			length += l
		}
	case reflect.Map:
		keyType := t.Key()
		valueType := t.Elem()

		kvPairType := reflect.StructOf([]reflect.StructField{
			{
				Name: "Key",
				Type: keyType,
				Tag:  reflect.StructTag(`json:"key"`),
			},
			{
				Name: "Value",
				Type: valueType,
				Tag:  reflect.StructTag(`json:"value"`),
			},
		})

		kvPairSliceType := reflect.SliceOf(kvPairType)

		decoded, l, err := Decode(data, kvPairSliceType)
		if err != nil {
			return nil, 0, err
		}

		length += l

		v.Set(reflect.MakeMap(t))

		kvPairs := reflect.ValueOf(decoded)
		for i := 0; i < kvPairs.Len(); i++ {
			kv := kvPairs.Index(i)
			key := kv.FieldByName("Key")
			value := kv.FieldByName("Value")
			v.SetMapIndex(key, value)
		}

	}
	return v.Interface(), length, nil
}

func DecodeWithRemainder(data []byte, t reflect.Type) (value interface{}, remainder []byte, err error) {
	val, used, err := Decode(data, t)
	if err != nil {
		return nil, nil, err
	}
	if len(data) < int(used) {
		return nil, nil, fmt.Errorf("data length smaller than used bytes")
	}
	return val, data[used:], nil
}

func SaveObject(path string, obj interface{}) error {
	codecPath := fmt.Sprintf("%s.bin", path)

	switch v := obj.(type) {
	default:
		codecEncode, err := Encode(v)
		if err != nil {
			return fmt.Errorf("Error encoding object: %v\n", err)
		}

		err = os.WriteFile(codecPath, codecEncode, 0644)
		if err != nil {
			return fmt.Errorf("Error writing codec file: %v\n", err)
		}
	}

	return nil
}

func PrintObject(obj interface{}) string {
	switch v := obj.(type) {
	default:
		jsonEncode, _ := json.MarshalIndent(v, "", "    ")
		return string(jsonEncode)
	}
}

func extractBytes(input []byte) ([]byte, []byte) {
	/*
		In GP_0.36 (272):
		If the input value of (272) is large, "l" will also increase and vice versa.
		"l" is than be used to encode first byte and the reaming "l" bytes.
		If the first byte is large, that means the number of the entire encoded bytes is large and vice versa.
		So the first byte can be used to determine the number of bytes to extract and the rule is as follows:
	*/

	if len(input) == 0 {
		return nil, input
	}

	firstByte := input[0]
	var numBytes int

	// Determine the number of bytes to extract based on the value of the 0th byte.
	switch {
	case firstByte >= 0 && firstByte < 128:
		numBytes = 1
	case firstByte >= 128 && firstByte < 192:
		numBytes = 2
	case firstByte >= 192 && firstByte < 224:
		numBytes = 3
	case firstByte >= 224 && firstByte < 240:
		numBytes = 4
	case firstByte >= 240 && firstByte < 248:
		numBytes = 5
	case firstByte >= 248 && firstByte < 252:
		numBytes = 6
	case firstByte >= 252 && firstByte < 254:
		numBytes = 7
	case firstByte >= 254:
		numBytes = 8
	default:
		numBytes = 1
	}

	// If the input length is insufficient to extract the specified number of bytes, return the original input.
	if len(input) < numBytes {
		return input, nil
	}

	// Extract the specified number of bytes and return the remaining bytes.
	extracted := input[:numBytes]
	remaining := input[numBytes:]

	return extracted, remaining
}
