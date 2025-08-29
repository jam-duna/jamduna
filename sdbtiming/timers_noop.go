//go:build !benchprofile

package sdbtiming

import "time"

type Recorder struct{}
type Row struct {
	Name     string
	Count    int
	Total    time.Duration
	Mean     time.Duration
	P50, P95 time.Duration
	Max      time.Duration
}

func New() *Recorder                          { return &Recorder{} }
func (r *Recorder) Add(string, time.Duration) {}
func (r *Recorder) Start(string) func()       { return func() {} }
func (r *Recorder) Snapshot() []Row           { return nil }
