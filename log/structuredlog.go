package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const (
	// Convention: Set-language-teamname
	TeamName = "A-go-cn" // vs "A-go-jamduna"
)

type StructuredLog struct {
	Time     time.Time       `json:"time"`
	Sender   string          `json:"sender_id"`
	MsgType  string          `json:"msg_type"`
	MsgJSON  json.RawMessage `json:"json_encoded"`
	Metadata *string         `json:"metadata,omitempty"`
	Elapsed  uint32          `json:"elapsed,omitempty"`
	MsgCodec string          `json:"codec_encoded,omitempty"`
}

var fieldOrder = []string{"time", "team", "sender_id", "msg_type", "json_encoded", "metadata", "elapsed", "codec_encoded"}

// Custom JSON marshaling to preserve field order and omit zero/empty values.
func (l StructuredLog) MarshalJSON() ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteByte('{')
	writeField := func(key string, val []byte) {
		if buf.Len() > 1 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(buf, `"%s":`, key)
		buf.Write(val)
	}
	for _, f := range fieldOrder {
		switch f {
		case "time":
			b, _ := json.Marshal(l.Time)
			writeField(f, b)
		case "team":
			b, _ := json.Marshal(TeamName)
			writeField(f, b)
		case "sender_id":
			b, _ := json.Marshal(l.Sender)
			writeField(f, b)
		case "msg_type":
			b, _ := json.Marshal(l.MsgType)
			writeField(f, b)
		case "json_encoded":
			writeField(f, l.MsgJSON)
		case "metadata":
			if l.Metadata != nil {
				b, _ := json.Marshal(*l.Metadata)
				writeField(f, b)
			}
		case "elapsed":
			if l.Elapsed != 0 {
				b, _ := json.Marshal(l.Elapsed)
				writeField(f, b)
			}
		case "codec_encoded":
			if l.MsgCodec != "" {
				b, _ := json.Marshal(l.MsgCodec)
				writeField(f, b)
			}
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func toMap(kv ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}

func parseUint32(v interface{}) uint32 {
	switch t := v.(type) {
	case int:
		return uint32(t)
	case int64:
		return uint32(t)
	case float64:
		return uint32(t)
	case uint32:
		return t
	case uint64:
		return uint32(t)
	case string:
		if n, err := strconv.ParseUint(t, 10, 32); err == nil {
			return uint32(n)
		}
	}
	return 0
}
