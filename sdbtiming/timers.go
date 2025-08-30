//go:build benchprofile

package sdbtiming

import (
	"sort"
	"sync"
	"time"
)

type sample struct{ d time.Duration }
type bucket struct {
	mu   sync.Mutex
	list []sample
}

type Recorder struct {
	mu   sync.RWMutex
	data map[string]*bucket
}

func New() *Recorder { return &Recorder{data: make(map[string]*bucket)} }

func (r *Recorder) Add(name string, d time.Duration) {
	r.mu.RLock()
	b, ok := r.data[name]
	r.mu.RUnlock()
	if !ok {
		r.mu.Lock()
		if b = r.data[name]; b == nil {
			b = &bucket{}
			r.data[name] = b
		}
		r.mu.Unlock()
	}
	b.mu.Lock()
	b.list = append(b.list, sample{d: d})
	b.mu.Unlock()
}

func (r *Recorder) Start(name string) func() {
	start := time.Now()
	return func() { r.Add(name, time.Since(start)) }
}

type Row struct {
	Name     string
	Count    int
	Total    time.Duration
	Mean     time.Duration
	P50, P95 time.Duration
	Max      time.Duration
}

func (r *Recorder) Snapshot() []Row {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Row, 0, len(r.data))
	for name, b := range r.data {
		b.mu.Lock()
		s := make([]time.Duration, len(b.list))
		for i, v := range b.list {
			s[i] = v.d
		}
		b.mu.Unlock()

		if len(s) == 0 {
			continue
		}
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })

		var total time.Duration
		for _, d := range s {
			total += d
		}
		p95Index := int(float64(len(s))*0.95) - 1
		if p95Index < 0 {
			p95Index = 0
		}
		row := Row{
			Name:  name,
			Count: len(s),
			Total: total,
			Mean:  time.Duration(int64(total) / int64(len(s))),
			P50:   s[len(s)/2],
			P95:   s[p95Index],
			Max:   s[len(s)-1],
		}
		out = append(out, row)
	}
	// Sort by total time descending
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}
