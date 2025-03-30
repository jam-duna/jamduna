package types

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func EncodeDebug(data interface{}, data_byte []byte) ([]byte, error) {
	return encodeDebugWithPath(data, data_byte, []string{})
}

func encodeDebugWithPath(data interface{}, data_byte []byte, fieldPath []string) ([]byte, error) {
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
		fmt.Printf("encoding with E: %d\n", v.Uint())
		return E(v.Uint()), nil
	case reflect.Uint8:
		fmt.Printf("encoding with E_l(1): %d\n", v.Uint())
		return E_l(uint64(v.Uint()), 1), nil
	case reflect.Uint16:
		fmt.Printf("encoding with E_l(2): %d\n", v.Uint())
		return E_l(uint64(v.Uint()), 2), nil
	case reflect.Uint32:
		fmt.Printf("encoding with E_l(4): %d\n", v.Uint())
		return E_l(uint64(v.Uint()), 4), nil
	case reflect.Uint64:
		fmt.Printf("encoding with E_l(8): %d\n", v.Uint())
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
	case reflect.Array:
		var encoded []byte
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, byte(v.Index(i).Uint()))
			} else {
				newPath := append(fieldPath, fmt.Sprintf("[%d]", i))
				encodedVi, err := encodeDebugWithPath(v.Index(i).Interface(), data_byte[len(encoded):], newPath)
				if err != nil {
					return nil, err
				}
				origin_data := data_byte[len(encoded) : len(encoded)+len(encodedVi)]
				if !reflect.DeepEqual(origin_data, encodedVi) {
					fmt.Printf("❌ Mismatch at Field Path: %s\n", strings.Join(newPath, "."))
					fmt.Printf("  → Origin Data:  %x\n", origin_data)
					fmt.Printf("  → Encoded Data: %x\n", encodedVi)
					return nil, fmt.Errorf("data mismatch at %s", strings.Join(newPath, "."))
				} else {
					fmt.Printf("✅ Match Field Path: %s\n", strings.Join(newPath, "."))
					fmt.Printf("  → Origin Data:  %x\n", origin_data)
					fmt.Printf("  → Encoded Data: %x\n", encodedVi)
				}
				encoded = append(encoded, encodedVi...)
			}
		}
		return encoded, nil
	case reflect.Slice:
		encoded := E(uint64(v.Len()))
		for i := 0; i < v.Len(); i++ {
			if v.Index(i).Kind() == reflect.Uint8 {
				encoded = append(encoded, byte(v.Index(i).Uint()))
			} else {
				newPath := append(fieldPath, fmt.Sprintf("[%d]", i))
				encodedVi, err := encodeDebugWithPath(v.Index(i).Interface(), data_byte[len(encoded):], newPath)
				if err != nil {
					return nil, err
				}
				origin_data := data_byte[len(encoded) : len(encoded)+len(encodedVi)]
				if !reflect.DeepEqual(origin_data, encodedVi) {
					fmt.Printf("❌ Mismatch at Field Path: %s\n", strings.Join(newPath, "."))
					fmt.Printf("  → Origin Data:  %x\n", origin_data)
					fmt.Printf("  → Encoded Data: %x\n", encodedVi)
					return nil, fmt.Errorf("data mismatch at %s", strings.Join(newPath, "."))
				} else {
					fmt.Printf("✅ Match Field Path: %s\n", strings.Join(newPath, "."))
					fmt.Printf("  → Origin Data:  %x\n", origin_data)
					fmt.Printf("  → Encoded Data: %x\n", encodedVi)
				}
				encoded = append(encoded, encodedVi...)
			}
		}
		return encoded, nil
	case reflect.Struct:
		var encoded []byte
		offset := 0
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			// use json tag if exists
			name := field.Name
			if tag, ok := field.Tag.Lookup("json"); ok {
				name = tag
			}
			newPath := append(fieldPath, name)

			encodedVi, err := encodeDebugWithPath(v.Field(i).Interface(), data_byte[offset:], newPath)
			if err != nil {
				return nil, err
			}

			origin_data := data_byte[offset : offset+len(encodedVi)]
			if !reflect.DeepEqual(origin_data, encodedVi) {
				fmt.Printf("❌ Mismatch at Field Path: %s\n", strings.Join(newPath, "."))
				fmt.Printf("  → Origin Data:  %x\n", origin_data)
				fmt.Printf("  → Encoded Data: %x\n", encodedVi)
				return nil, fmt.Errorf("data mismatch at %s", strings.Join(newPath, "."))
			} else {
				fmt.Printf("✅ Match Field Path: %s\n", strings.Join(newPath, "."))
				fmt.Printf("  → Origin Data:  %x\n", origin_data)
				fmt.Printf("  → Encoded Data: %x\n", encodedVi)
			}

			offset += len(encodedVi)
			encoded = append(encoded, encodedVi...)
		}
		return encoded, nil
	case reflect.Ptr:
		if v.IsNil() {
			return []byte{0}, nil
		}
		return encodeDebugWithPath(v.Elem().Interface(), data_byte, fieldPath)
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
		return encodeDebugWithPath(sortedKVPairs, data_byte, fieldPath)

	default:
		return []byte{}, fmt.Errorf("unsupported type: %s at %s", v.Kind().String(), strings.Join(fieldPath, "."))
	}
}
