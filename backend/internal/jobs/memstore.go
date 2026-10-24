package jobs

import (
	"bytes"
	"slices"
	"sync"
	"time"
)

// MemoryStore는 메모리에만 두는 Store다(상태 디렉터리가 없을 때, 테스트). 다시 시작하면 사라진다.
type MemoryStore struct {
	mu      sync.Mutex
	current *Record
	results map[string][]Result
	outputs map[string]*memOutput
	// Now는 출력 정리의 기준 시각이다.
	Now func() time.Time
}

type memOutput struct {
	data     []byte
	modified time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{results: map[string][]Result{}, outputs: map[string]*memOutput{}, Now: time.Now}
}

func (s *MemoryStore) LoadCurrent() (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil, nil
	}
	r := cloneRecord(*s.current)
	return &r, nil
}

func (s *MemoryStore) SaveCurrent(r *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r == nil {
		s.current = nil
		return nil
	}
	c := cloneRecord(*r)
	s.current = &c
	return nil
}

func (s *MemoryStore) LoadResults(user string) ([]Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneResults(s.results[user]), nil
}

func (s *MemoryStore) SaveResults(user string, rs []Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(rs) == 0 {
		delete(s.results, user)
		return nil
	}
	s.results[user] = cloneResults(rs)
	return nil
}

func (s *MemoryStore) out(id string) *memOutput {
	o, ok := s.outputs[id]
	if !ok {
		o = &memOutput{}
		s.outputs[id] = o
	}
	return o
}

func (s *MemoryStore) AppendOutput(id string, b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.out(id)
	o.data = append(o.data, b...)
	o.modified = s.Now()
	return nil
}

func (s *MemoryStore) ReadOutput(id string, off int64, n int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.outputs[id]
	if !ok || off >= int64(len(o.data)) {
		return nil, nil
	}
	end := min(off+int64(n), int64(len(o.data)))
	return slices.Clone(o.data[off:end]), nil
}

func (s *MemoryStore) TruncateOutput(id string, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.out(id)
	if size < int64(len(o.data)) {
		o.data = o.data[:size]
	}
	return nil
}

func (s *MemoryStore) DropOutputHead(id string, keep int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.out(id)
	drop := HeadToDrop(o.data, keep)
	o.data = slices.Clone(o.data[drop:])
	return drop, nil
}

func (s *MemoryStore) DeleteOutput(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.outputs, id)
	return nil
}

func (s *MemoryStore) PruneOutputs(before time.Time, keep string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, o := range s.outputs {
		if id != keep && o.modified.Before(before) {
			delete(s.outputs, id)
		}
	}
	return nil
}

// HeadToDrop은 data에서 뒤 keep바이트 정도를 남길 때 앞에서 버릴 바이트 수다. 잘린 줄이 남지 않게 버릴 자리
// 뒤의 첫 줄바꿈까지 더 버린다(줄바꿈이 없으면 keep만 남긴다). Store 구현이 DropOutputHead에서 쓴다.
func HeadToDrop(data []byte, keep int64) int64 {
	if int64(len(data)) <= keep {
		return 0
	}
	drop := int64(len(data)) - keep
	if i := bytes.IndexByte(data[drop:], '\n'); i >= 0 {
		drop += int64(i) + 1
	}
	return drop
}

func cloneRecord(r Record) Record {
	r.Handle = slices.Clone(r.Handle)
	r.Before = slices.Clone(r.Before)
	return r
}

func cloneResults(rs []Result) []Result {
	out := slices.Clone(rs)
	for i := range out {
		out[i].Items = slices.Clone(out[i].Items)
		if out[i].Count != nil {
			c := *out[i].Count
			out[i].Count = &c
		}
	}
	return out
}
