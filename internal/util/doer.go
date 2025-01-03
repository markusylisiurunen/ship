package util

import "sync"

type Doer struct {
	mux sync.RWMutex
	err error
}

func (d *Doer) Do(f func() error) *Doer {
	d.mux.RLock()
	if d.err != nil {
		d.mux.RUnlock()
		return d
	}
	d.mux.RUnlock()
	err := f()
	d.mux.Lock()
	d.err = err
	d.mux.Unlock()
	return d
}

func (d *Doer) Err() error {
	d.mux.RLock()
	defer d.mux.RUnlock()
	return d.err
}
