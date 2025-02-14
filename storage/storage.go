package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/colorfulnotion/jam/common"
	"github.com/syndtr/goleveldb/leveldb"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// go get go.opentelemetry.io/otel
// go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
// go get go.opentelemetry.io/otel/propagation
// go get go.opentelemetry.io/otel/sdk/trace
// go get go.opentelemetry.io/otel/sdk/resource
// go get go.opentelemetry.io/otel/trace
// go get go.opentelemetry.io/otel/semconv/v1.26.0
// go mod tidy

type LogMessage struct {
	Payload  interface{}
	Timeslot uint32
	Self     bool
}

// StateDBStorage struct to hold the LevelDB instance
type StateDBStorage struct {
	db      *leveldb.DB
	logChan chan LogMessage

	// OpenTelemetry stuff
	Tp                       *sdktrace.TracerProvider
	WorkPackageContext       context.Context
	BlockContext             context.Context
	BlockAnnouncementContext context.Context
	SendTrace                bool
	NodeID                   uint16
}

const (
	ImportDASegmentShardPrefix  = "is_"
	ImportDAJustificationPrefix = "ij_"
	AuditDABundlePrefix         = "ab_"
	AuditDASegmentShardPrefix   = "as_"
	AuditDAJustificationPrefix  = "aj_"
)

// NewStateDBStorage initializes a new LevelDB store
func NewStateDBStorage(path string) (*StateDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	s := StateDBStorage{
		db:      db,
		logChan: make(chan LogMessage, 100),
	}
	s.InitTracer("JAM")
	return &s, nil
}

// ReadKV reads a value for a given key from the LevelDB store
func (store *StateDBStorage) ReadKV(key common.Hash) ([]byte, error) {
	data, err := store.db.Get(key.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("ReadKV %v Err: %v", key, err)
	}
	return data, nil
}

// WriteKV writes a key-value pair to the LevelDB store
func (store *StateDBStorage) WriteKV(key common.Hash, value []byte) error {
	return store.db.Put(key.Bytes(), value, nil)
}

// Close closes the LevelDB store
func (store *StateDBStorage) DeleteK(key common.Hash) error {
	return store.db.Delete(key.Bytes(), nil)
}

// Close closes the LevelDB store
func (store *StateDBStorage) Close() error {
	return store.db.Close()
}

func (store *StateDBStorage) ReadRawKV(key []byte) ([]byte, bool, error) {
	data, err := store.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("ReadRawKV %v Err: %v", key, err)
	}
	return data, true, nil
}

func (store *StateDBStorage) ReadRawKVWithPrefix(prefix []byte) ([][2][]byte, error) {
	iter := store.db.NewIterator(nil, nil)
	defer iter.Release()

	var keyvals [][2][]byte
	for iter.Next() {
		key := iter.Key()
		if len(key) >= len(prefix) && string(key[:len(prefix)]) == string(prefix) {
			keyval := [2][]byte{key, iter.Value()}
			keyvals = append(keyvals, keyval)
		}
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("ReadRawKVWithPrefix %v Err: %v", prefix, err)
	}
	return keyvals, nil
}

func (store *StateDBStorage) WriteRawKV(key []byte, value []byte) error {
	return store.db.Put(key, value, nil)
}

func (store *StateDBStorage) DeleteRawK(key []byte) error {
	return store.db.Delete(key, nil)
}

func (store *StateDBStorage) WriteLog(obj interface{}, timeslot uint32) {
	msg := LogMessage{
		Payload:  obj,
		Timeslot: timeslot,
	}
	store.logChan <- msg
}

func (store *StateDBStorage) GetChan() chan LogMessage {
	return store.logChan
}

func TestTrace(host string) bool {
	resp, err := http.Post(fmt.Sprintf("http://%s/v1/traces", host), "application/json", bytes.NewBuffer([]byte(`{}`)))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (store *StateDBStorage) InitTracer(serviceName string) error {
	// run jaeger web locally:
	// docker run --rm --name jaeger -p 16686:16686 -p 4317:4317 -p 4318:4318 -p 5778:5778 -p 9411:9411 jaegertracing/jaeger:2.3.0
	// http://localhost:16686/search

	host := "jaeger.jamduna.com:4318" // localhost:4318 // jaeger.jamduna.com:4318
	store.SendTrace = TestTrace(host)

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpoint(host),
		otlptracehttp.WithInsecure())
	if err != nil {
		return err
	}

	tp := sdktrace.NewTracerProvider(
		// sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName))),
	)

	store.Tp = tp

	otel.SetTextMapPropagator(propagation.TraceContext{})
	return nil
}

func (store *StateDBStorage) UpdateWorkPackageContext(ctx context.Context) {
	store.WorkPackageContext = ctx
}

func (store *StateDBStorage) UpdateBlockContext(ctx context.Context) {
	store.BlockContext = ctx
}

func (store *StateDBStorage) UpdateBlockAnnouncementContext(ctx context.Context) {
	store.BlockAnnouncementContext = ctx
}

func (store *StateDBStorage) CleanWorkPackageContext() {
	store.WorkPackageContext = context.Background()
}

func (store *StateDBStorage) CleanBlockContext() {
	store.BlockContext = context.Background()
}

func (store *StateDBStorage) CleanBlockAnnouncementContext() {
	store.BlockAnnouncementContext = context.Background()
}
