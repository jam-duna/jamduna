package types

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/jam-duna/jamduna/log"
)

type CustomEncoder interface {
	Encode() []byte
}

type CustomDecoder interface {
	Decode(data []byte) (interface{}, uint32)
}

// Cache for map kv pair types to avoid repeated reflect.StructOf calls
var (
	kvPairTypeCache   = make(map[reflect.Type]reflect.Type)
	kvPairTypeCacheMu sync.RWMutex
)

func getKVPairSliceType(keyType, valueType reflect.Type) reflect.Type {
	// Create a unique key for this map type
	mapType := reflect.MapOf(keyType, valueType)

	kvPairTypeCacheMu.RLock()
	if sliceType, ok := kvPairTypeCache[mapType]; ok {
		kvPairTypeCacheMu.RUnlock()
		return sliceType
	}
	kvPairTypeCacheMu.RUnlock()

	// Create the kv pair type
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
	sliceType := reflect.SliceOf(kvPairType)

	kvPairTypeCacheMu.Lock()
	kvPairTypeCache[mapType] = sliceType
	kvPairTypeCacheMu.Unlock()

	return sliceType
}

func powerOfTwo(exp uint32) uint64 {
	return 1 << exp
}

func E_l(x uint64, l uint32) []byte {
	n := int(l)
	if n <= 0 {
		return nil
	}
	b := make([]byte, n)

	if n >= 8 {
		binary.LittleEndian.PutUint64(b[:8], x)
		// b[8:] already zeroed
		return b
	}

	// n < 8
	for i := 0; i < n; i++ {
		b[i] = byte(x)
		x >>= 8
	}
	return b
}

func DecodeE_l(b []byte) uint64 {
	if len(b) >= 8 {
		return binary.LittleEndian.Uint64(b[:8])
	}
	var buf [8]byte // stack-allocated, no heap escape
	copy(buf[:], b)
	return binary.LittleEndian.Uint64(buf[:])
}

// GP v0.3.6 eq(272)  E - Integer Encoding: general natural number serialization up to 2^64
func E(x uint64) []byte {
	if x == 0 {
		return []byte{0}
	}
	for l := uint32(0); l < 8; l++ {
		lower := uint64(1) << (7 * l)       // powerOfTwo(7*l)
		upper := uint64(1) << (7 * (l + 1)) // powerOfTwo(7*(l+1))
		if x >= lower && x < upper {
			divisor := uint64(1) << (8 * l) // powerOfTwo(8*l)
			headerBase := uint64(256) - (uint64(1) << (8 - l))
			// Pre-allocate exact size
			encoded := make([]byte, 1+l)
			encoded[0] = byte(headerBase + x/divisor)
			// Inline E_l for trailing bytes
			rem := x % divisor
			for i := uint32(0); i < l; i++ {
				encoded[1+i] = byte(rem)
				rem >>= 8
			}
			return encoded
		}
	}
	// x >= 2^56, need 9 bytes
	encoded := make([]byte, 9)
	encoded[0] = 255
	binary.LittleEndian.PutUint64(encoded[1:], x)
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
	// Pre-allocate: length prefix (max 9 bytes) + each uint64 (max 9 bytes each)
	encoded := make([]byte, 0, 9+len(x)*9)
	encoded = append(encoded, E(uint64(len(x)))...)
	for i := 0; i < len(x); i++ {
		encoded = append(encoded, E(x[i])...)
	}
	return encoded
}

// GP v0.3.6 eq(274) ↕x≡(|x|,x) - Length Discriminator Decoding. Maybe Reuqired later [DO NOT DELETE]
func DecodeLengthE(encoded []byte) ([]uint64, uint32) {
	length, l := DecodeE([]byte{encoded[0]})
	T := make([]uint64, 0, int(length)) // Pre-allocate capacity
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

// Cache for custom decoder type checks
var (
	customDecoderCache   = make(map[reflect.Type]bool)
	customDecoderCacheMu sync.RWMutex
)

func isCustomDecoder(t reflect.Type) bool {
	customDecoderCacheMu.RLock()
	if result, ok := customDecoderCache[t]; ok {
		customDecoderCacheMu.RUnlock()
		return result
	}
	customDecoderCacheMu.RUnlock()

	// Check if type implements CustomDecoder
	instance := reflect.New(t).Elem().Interface()
	_, isDecoder := instance.(CustomDecoder)

	customDecoderCacheMu.Lock()
	customDecoderCache[t] = isDecoder
	customDecoderCacheMu.Unlock()

	return isDecoder
}

func CheckCustomDecode(data []byte, t reflect.Type) (bool, interface{}, uint32) {
	// Fast path: check cache first
	if !isCustomDecoder(t) {
		return false, nil, 0
	}

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
		s := v.String()
		runes := []rune(s)
		// Pre-allocate: length prefix (max 9 bytes) + each rune encoded (max 9 bytes each)
		encoded := make([]byte, 0, 9+len(runes)*9)
		encoded = append(encoded, E(uint64(len(runes)))...)
		for _, c := range runes {
			encoded = append(encoded, E(uint64(c))...)
		}
		return encoded, nil

	// GP v0.3.6 eq(273) Sequence Encoding
	case reflect.Array:
		arrayLen := v.Len()
		// Fast path for [N]byte arrays - direct byte copy
		if arrayLen > 0 && v.Index(0).Kind() == reflect.Uint8 {
			encoded := make([]byte, arrayLen)
			for i := 0; i < arrayLen; i++ {
				encoded[i] = byte(v.Index(i).Uint())
			}
			return encoded, nil
		}
		// General case for other array types
		var encoded []byte
		for i := 0; i < arrayLen; i++ {
			encodedVi, err := Encode(v.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, encodedVi...)
		}
		return encoded, nil

	case reflect.Slice:
		// GP v0.3.6 eq(274) Length Discriminator Encoding
		sliceLen := v.Len()
		// Fast path for []byte slices - use v.Bytes() for direct access
		if v.Type().Elem().Kind() == reflect.Uint8 {
			encoded := E(uint64(sliceLen))
			if sliceLen > 0 {
				encoded = append(encoded, v.Bytes()...)
			}
			return encoded, nil
		}
		encoded := E(uint64(sliceLen))
		for i := 0; i < sliceLen; i++ {
			encodedVi, err := Encode(v.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, encodedVi...)
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
			case reflect.Float32, reflect.Float64:
				return keys[i].Float() < keys[j].Float()
			case reflect.Bool:
				// false < true
				return !keys[i].Bool() && keys[j].Bool()
			default:
				// For complex types like arrays/structs, compare their encoded bytes
				ei, _ := Encode(keys[i].Interface())
				ej, _ := Encode(keys[j].Interface())
				return string(ei) < string(ej)
			}
		})

		type kvPair struct {
			Key   interface{}
			Value interface{}
		}
		sortedKVPairs := make([]kvPair, 0, len(keys)) // Pre-allocate capacity
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
		var sb strings.Builder
		sb.Grow(int(str_len)) // Pre-allocate capacity
		for i := 0; i < int(str_len); i++ {
			if len(data) < int(length+1) {
				return nil, 0, fmt.Errorf("data length insufficient for string character")
			}
			x, l := DecodeE(data[length:])
			if len(data) < int(length+l) {
				return nil, 0, fmt.Errorf("data length insufficient for string character decoding")
			}
			sb.WriteRune(rune(x))
			length += l
		}
		v.SetString(sb.String())
	case reflect.Array:
		arrayLen := v.Len()
		// Fast path for [N]byte arrays - use copy instead of element-by-element
		if arrayLen > 0 && v.Index(0).Kind() == reflect.Uint8 {
			if len(data) < int(length)+arrayLen {
				return nil, 0, fmt.Errorf("data length insufficient for byte array")
			}
			reflect.Copy(v, reflect.ValueOf(data[length:length+uint32(arrayLen)]))
			length += uint32(arrayLen)
		} else {
			for i := 0; i < arrayLen; i++ {
				if len(data) < int(length+1) {
					return nil, 0, fmt.Errorf("data length insufficient for array element")
				}
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
		// Validate slice length to prevent overflow when converting to int
		if item_len > uint64(len(data)-int(length)) {
			return nil, 0, fmt.Errorf("slice length %d exceeds remaining data %d", item_len, len(data)-int(length))
		}

		length += l

		// Fast path for []byte - use copy instead of element-by-element
		if t.Elem().Kind() == reflect.Uint8 {
			if len(data) < int(length)+int(item_len) {
				return nil, 0, fmt.Errorf("data length insufficient for byte slice")
			}
			byteSlice := make([]byte, item_len)
			copy(byteSlice, data[length:length+uint32(item_len)])
			length += uint32(item_len)
			return byteSlice, length, nil
		}

		v.Set(reflect.MakeSlice(t, int(item_len), int(item_len)))
		for i := 0; i < int(item_len); i++ {
			if len(data) < int(length+1) {
				return nil, 0, fmt.Errorf("data length insufficient for slice element")
			}
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

		// Use cached kv pair slice type
		kvPairSliceType := getKVPairSliceType(keyType, valueType)

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
func SaveObject(path string, obj interface{}, withJSON bool) error {
	//saveObjectSerialized(path, obj, withJSON)
	return saveObjectConcurrent(path, obj, true)
}

func saveObjectConcurrent(path string, obj interface{}, withJSON bool) error {
	// TODO: strip any bin/json/hex extensions from path
	basePath := strings.TrimSuffix(path, filepath.Ext(path))

	codecPath := fmt.Sprintf("%s.bin", basePath)
	jsonPath := fmt.Sprintf("%s.json", basePath)
	hexPath := fmt.Sprintf("%s.hex", basePath)

	jsonEnabled := withJSON
	codecEnabled := true
	hexEnabled := false

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	switch v := obj.(type) {
	default:
		var codecEncode []byte
		var err error
		if codecEnabled || hexEnabled {
			codecEncode, err = Encode(v)
			if err != nil {
				return fmt.Errorf("error encoding object for binary/hex: %w", err)
			}
		}

		if codecEnabled {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := os.WriteFile(codecPath, codecEncode, 0644); err != nil {
					errChan <- fmt.Errorf("error writing codec file: %w", err)
				}
			}()
		}

		if hexEnabled {
			wg.Add(1)
			go func() {
				defer wg.Done()
				hexString := fmt.Sprintf("0x%x\n", codecEncode)
				if err := os.WriteFile(hexPath, []byte(hexString), 0644); err != nil {
					errChan <- fmt.Errorf("error writing hex file: %w", err)
				}
			}()
		}

		if jsonEnabled {
			wg.Add(1)
			go func() {
				defer wg.Done()
				//jsonEncode, err := json.MarshalIndent(v, "", "    ")
				jsonEncode, err := json.Marshal(v)
				if err != nil {
					log.Info(log.G, "Error encoding object to JSON", "err", err, "object", PrintObject(v))
					errChan <- fmt.Errorf("error encoding object to JSON: %w", err)
					return
				}
				if err := os.WriteFile(jsonPath, jsonEncode, 0644); err != nil {
					errChan <- fmt.Errorf("error writing JSON file: %w", err)
				}
			}()
		}
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			// Return the first error we encounter.
			return err
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
	case firstByte < 128:
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
