package types

import (
	"reflect"
)

// Data Encoding
func Encode(data interface{}) []byte {
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Struct:
		var encoded []byte
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				// Unexported field, skip it
				continue
			}
			encoded = append(encoded, Encode(v.Field(i).Interface())...)
		}
		return encoded
	case reflect.Bool:
		if v.Bool() {
			return []byte{1}
		}
		return []byte{0}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return E(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return E(v.Uint())
	case reflect.Float32, reflect.Float64:
		return E(uint64(v.Float()))
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
	case reflect.Ptr:
		if v.IsNil() {
			return []byte{0}
		}
		return append([]byte{1}, Encode(v.Elem().Interface())...)
	}
	return []byte{}
}

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
