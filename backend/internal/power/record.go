package power

import "sync"

// Record는 대시보드가 다시 시작을 예약할 때 남기는 기록이다. 예약 파일과 USEC·Mode가 모두 같을 때만 쓴다.
type Record struct {
	// USEC는 예약한 시각(마이크로초)이다.
	USEC int64 `json:"usec"`
	// Mode는 logind 종류 이름이다(늘 reboot).
	Mode string `json:"mode"`
	// By는 예약한 대시보드 사용자 이름이다.
	By string `json:"by"`
}

// Store는 예약 기록 하나를 둔다. 상태 디렉터리가 있으면 prefs.FileStore, 없으면 MemoryStore다.
type Store interface {
	// PowerRecord는 기록을 읽는다. 없으면 ok가 false다.
	PowerRecord() (rec Record, ok bool, err error)
	SetPowerRecord(Record) error
	// ClearPowerRecord는 기록을 지운다. 없어도 성공이다.
	ClearPowerRecord() error
}

// MemoryStore는 메모리에 두는 Store다(다시 시작하면 사라진다).
type MemoryStore struct {
	mu  sync.Mutex
	rec *Record
}

// NewMemoryStore는 빈 MemoryStore다.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (m *MemoryStore) PowerRecord() (Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec == nil {
		return Record{}, false, nil
	}
	return *m.rec, true, nil
}

func (m *MemoryStore) SetPowerRecord(r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = &r
	return nil
}

func (m *MemoryStore) ClearPowerRecord() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = nil
	return nil
}
