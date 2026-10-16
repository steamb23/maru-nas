package proto

// OpenResult는 OpOpen의 결과다.
type OpenResult struct {
	// Ticket은 이후 요청에 쓰는 비밀 값이다. 로그·응답에 넣지 않는다.
	Ticket string `json:"ticket"`
	// CanElevate는 이 사용자가 root 권한으로 상승할 수 있는지(그룹 조건)다.
	CanElevate bool `json:"canElevate"`
}

// StatusResult는 OpStatus의 결과다.
type StatusResult struct {
	User       string `json:"user"`
	Pending    bool   `json:"pending"`
	CanElevate bool   `json:"canElevate"`
	// ElevatedSeconds는 상승이 끝날 때까지 남은 초다. 상승하지 않았으면 0이다.
	ElevatedSeconds int `json:"elevatedSeconds"`
}

// ElevateResult는 OpElevate의 결과다.
type ElevateResult struct {
	ElevatedSeconds int `json:"elevatedSeconds"`
}

// Group은 그룹 하나다.
type Group struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Identity는 id 연산의 결과로, worker가 실제로 가진 권한이다.
type Identity struct {
	UID    int     `json:"uid"`
	GID    int     `json:"gid"`
	User   string  `json:"user"`
	Groups []Group `json:"groups"`
}

// 항목 종류(Entry.Type)
const (
	TypeFile    = "file"
	TypeDir     = "dir"
	TypeSymlink = "symlink"
	TypeOther   = "other"
)

// Entry는 파일 하나의 정보다. 심볼릭 링크는 따라가지 않은 링크 자신의 정보다.
type Entry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	// Mode는 권한 비트(setuid·setgid·sticky 포함, 0o7777 안)다.
	Mode uint32 `json:"mode"`
	// ModTime은 수정 시각(Unix 초)이다.
	ModTime int64 `json:"modTime"`
	// Target은 심볼릭 링크가 가리키는 경로(그대로)다.
	Target string `json:"target,omitempty"`
	// LinkType은 심볼릭 링크를 허용 범위 안에서 따라간 대상의 종류다. 깨졌거나 범위 밖이면 빈 값이다.
	LinkType string `json:"linkType,omitempty"`
}

// PathParams는 경로 하나를 받는 연산(fs.stat, fs.mkdir)의 인자다.
type PathParams struct {
	Path string `json:"path"`
}

// ListResult는 fs.list의 결과다.
type ListResult struct {
	Entries []Entry `json:"entries"`
	// Truncated는 항목이 많아 일부만 담았는지다.
	Truncated bool `json:"truncated"`
}

// ReadParams는 fs.read의 인자다. Length가 0이면 끝까지 읽는다.
type ReadParams struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
	Length int64  `json:"length,omitempty"`
}

// ReadResult는 fs.read의 결과다. 응답 머리의 Size만큼 내용이 뒤따른다.
type ReadResult struct {
	// FileSize는 파일 전체 크기다.
	FileSize int64 `json:"fileSize"`
	ModTime  int64 `json:"modTime"`
}

// WriteParams는 fs.write의 인자다. 요청 머리의 Size만큼 내용이 뒤따른다.
type WriteParams struct {
	Path string `json:"path"`
	// Overwrite가 false면 같은 이름이 있을 때 exists로 거절한다.
	Overwrite bool `json:"overwrite,omitempty"`
}

// RenameParams는 fs.rename의 인자다. 대상이 이미 있으면 exists로 거절한다.
type RenameParams struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RemoveParams는 fs.remove의 인자다. Recursive가 false면 빈 폴더와 파일만 지운다.
type RemoveParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}
