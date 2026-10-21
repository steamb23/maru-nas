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
	// Home은 티켓 사용자의 홈 디렉터리다. 파일 연산의 기준으로 쓸 수 없는 홈(/, 빈 값)이면 비운다.
	Home string `json:"home,omitempty"`
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
	// ModTimeNs는 수정 시각(Unix 나노초)이다. 같은 초 안의 변경을 가려낼 때 쓴다.
	ModTimeNs int64 `json:"modTimeNs"`
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
	FileSize  int64 `json:"fileSize"`
	ModTime   int64 `json:"modTime"`
	ModTimeNs int64 `json:"modTimeNs"`
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

// NamesResult는 fs.names의 결과다. 응답 머리의 Size만큼 이름 목록(EncodeName 형식)이 뒤따른다.
type NamesResult struct {
	// Count는 본문에 담은 항목 수다.
	Count int `json:"count"`
	// Truncated는 항목이 MaxNames·MaxNamesBytes를 넘어 일부만 담았는지다.
	Truncated bool `json:"truncated"`
	// DirModTimeNs는 폴더 자신의 수정 시각(Unix 나노초)이다. 항목이 더해지거나 빠지면 바뀐다.
	DirModTimeNs int64 `json:"dirModTimeNs"`
	// Writable은 이 사용자가 폴더 안에 파일을 만들 수 있는지(faccessat W_OK|X_OK)다.
	Writable bool `json:"writable"`
}

// StatManyParams는 fs.statmany의 인자다. Names는 Dir 바로 안의 이름(경로 구분자 없음)이다.
type StatManyParams struct {
	Dir   string   `json:"dir"`
	Names []string `json:"names"`
}

// StatItem은 fs.statmany의 항목 하나다. 읽지 못했으면 Code만 있다(읽는 사이에 지워졌으면 not_found).
type StatItem struct {
	Entry
	Code string `json:"code,omitempty"`
	// Locked는 폴더(심볼릭 링크 아님)인데 이 사용자가 들어가 목록을 볼 수 없는지(R_OK|X_OK)다.
	Locked bool `json:"locked,omitempty"`
}

// StatManyResult는 fs.statmany의 결과다. Items는 Names와 같은 순서다.
type StatManyResult struct {
	Items []StatItem `json:"items"`
}

// Access는 이 사용자가 파일을 읽고 쓸 수 있는지(faccessat)다. 폴더는 들어가 목록을 볼 수 있는지, 안에
// 파일을 만들 수 있는지다.
type Access struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// InfoResult는 fs.info의 결과다. 심볼릭 링크는 ACL·Access를 읽지 않는다(faccessat이 링크를 따라가므로).
type InfoResult struct {
	Entry
	UID int `json:"uid"`
	GID int `json:"gid"`
	// Owner와 Group은 /etc/passwd·/etc/group에서 찾은 이름이다. 없으면 빈 값이다.
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	// ACL은 POSIX ACL(접근 또는 기본)이 있는지다. 확인하지 못했으면 nil이다.
	ACL    *bool   `json:"acl,omitempty"`
	Access *Access `json:"access,omitempty"`
}

// UpgradeApplyParams는 upgrade.apply의 인자다. 꼴은 config.ValidUpgradeTarget·ValidUpgradeSetting으로 검사하고,
// 진행할 수 있는지(마지막 확인, 저장소 상태, 설정 값)는 업그레이드 서비스가 다시 확인한다.
type UpgradeApplyParams struct {
	// Target은 확인 단계에서 정한 대상 커밋(40자리 16진수)이다.
	Target string `json:"target"`
	// Settings는 업그레이드 계획이 물은 설정의 이름과 값이다.
	Settings map[string]string `json:"settings,omitempty"`
}

// SSHKeyParams는 ssh.authorize·ssh.revoke의 인자다.
type SSHKeyParams struct {
	// PublicKey는 authorized_keys 형식의 ed25519 공개 키("ssh-ed25519 <base64>")다. 주석은 받지 않는다.
	PublicKey string `json:"publicKey"`
}

// TrashResult는 fs.trash의 결과다. Trash는 넣은 휴지통 디렉터리의 절대 경로이고, 항목은 그 안의 files/<Name>,
// info/<Name>.trashinfo다.
type TrashResult struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
}

// 휴지통 종류(TrashVolume.Kind)
const (
	// TrashHome은 홈 휴지통(~/.local/share/Trash)이다.
	TrashHome = "home"
	// TrashShared는 디스크 맨 위의 .Trash/<uid>다(관리자가 만든 .Trash, sticky).
	TrashShared = "shared"
	// TrashOwn은 디스크 맨 위의 .Trash-<uid>다(이미 있거나 사용자가 만들 수 있다).
	TrashOwn = "own"
	// TrashNone은 쓸 수 있는 휴지통이 없다는 뜻이다(영구 삭제만).
	TrashNone = "none"
)

// TrashVolume은 기준 디렉터리 하나의 휴지통이다.
type TrashVolume struct {
	// Top은 기준 디렉터리(홈 또는 /srv 아래 마운트 지점)다.
	Top  string `json:"top"`
	Kind string `json:"kind"`
	// Shared는 디스크 맨 위에 모든 사용자가 쓰는 .Trash(sticky 폴더)가 있는지다.
	Shared bool `json:"shared,omitempty"`
}

// TrashDirsResult는 fs.trashdirs의 결과다.
type TrashDirsResult struct {
	Volumes []TrashVolume `json:"volumes"`
}

// TrashItem은 휴지통 항목 하나다.
type TrashItem struct {
	// Trash는 휴지통 디렉터리의 절대 경로, Name은 그 안의 이름이다.
	Trash string `json:"trash"`
	Name  string `json:"name"`
	// Path는 원래 경로(절대)다.
	Path string `json:"path"`
	// DeletedAt은 지운 시각(Unix 초)이다. trashinfo의 DeletionDate를 읽지 못하면 0이다.
	DeletedAt int64  `json:"deletedAt"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
	// ModTime은 항목의 수정 시각(Unix 초)이다.
	ModTime int64 `json:"modTime"`
	// BadName은 이름이나 원래 경로가 UTF-8이 아니라는 뜻이다. JSON으로 바이트를 옮길 수 없어 되돌리거나 지울 수 없다.
	BadName bool `json:"badName,omitempty"`
}

// TrashListResult는 fs.trashlist의 결과다(지운 시각이 늦은 것부터).
type TrashListResult struct {
	Items []TrashItem `json:"items"`
	// Truncated는 항목이 많아 일부만 담았는지다.
	Truncated bool `json:"truncated"`
}

// TrashItemParams는 휴지통 항목 하나를 가리키는 인자다(fs.trashpurge). Trash는 사용자의 휴지통 디렉터리여야 한다.
type TrashItemParams struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
}

// TrashRestoreParams는 fs.trashrestore의 인자다. 항목을 To(원래 경로나 같은 폴더의 새 이름)로 되돌린다.
// To는 휴지통과 같은 기준 디렉터리 안이어야 하고(아니면 cross_device), 이미 있으면 exists다.
type TrashRestoreParams struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
	To    string `json:"to"`
}

// TrashPrepareParams는 trash.prepare의 인자다. Disk는 /srv 아래 마운트 지점(도우미의 기준 디렉터리)이다.
type TrashPrepareParams struct {
	Disk string `json:"disk"`
}
