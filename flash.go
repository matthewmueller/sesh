package sesh

import "slices"

// Flashes holds string values by case-sensitive key. Initialize it before
// calling Set or Add, for example with make(Flashes).
type Flashes map[string][]string

// Get returns the first value for key, or an empty string if none exists.
func (f Flashes) Get(key string) string {
	if values := f[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

// Values returns the values for key.
func (f Flashes) Values(key string) []string {
	return f[key]
}

// Set replaces the values for key with value.
func (f Flashes) Set(key, value string) {
	f[key] = []string{value}
}

// Add appends value to the values for key.
func (f Flashes) Add(key, value string) {
	f[key] = append(f[key], value)
}

// Del removes key and its values.
func (f Flashes) Del(key string) {
	delete(f, key)
}

// Clear removes all keys and values.
func (f Flashes) Clear() {
	clear(f)
}

// flashManager reads flashes received by the current request and queues flashes
// for the next request. Use it within Manager.Middleware; outside middleware,
// reads return empty values and writes do nothing.
type flashManager[Data any] struct {
	manager *Manager[Data]
}

// Get returns the first current value for key without consuming it.
func (f *flashManager[Data]) Get(r Request, key string) string {
	if s, ok := f.manager.session(r.Context()); ok {
		return s.current.Get(key)
	}
	return ""
}

// All returns a deep copy of the current request's flashes.
func (f *flashManager[Data]) All(r Request) Flashes {
	flashes := make(Flashes)
	if s, ok := f.manager.session(r.Context()); ok {
		for key, values := range s.current {
			flashes[key] = slices.Clone(values)
		}
	}
	return flashes
}

// Set replaces the values for key for the next request.
func (f *flashManager[Data]) Set(r Request, key, value string) {
	if s, ok := f.manager.session(r.Context()); ok {
		if s.next == nil {
			s.next = make(Flashes)
		}
		s.next.Set(key, value)
	}
}

// Add appends a value for key for the next request.
func (f *flashManager[Data]) Add(r Request, key, value string) {
	if s, ok := f.manager.session(r.Context()); ok {
		if s.next == nil {
			s.next = make(Flashes)
		}
		s.next.Add(key, value)
	}
}

// Del removes a pending key without changing current flashes.
func (f *flashManager[Data]) Del(r Request, key string) {
	if s, ok := f.manager.session(r.Context()); ok {
		s.next.Del(key)
	}
}

// Clear removes all pending flashes without changing current flashes.
func (f *flashManager[Data]) Clear(r Request) {
	if s, ok := f.manager.session(r.Context()); ok {
		s.next.Clear()
	}
}
