package api

// 파일 탐색기. 파일은 권한 도우미가 로그인한 사용자의 권한으로 다룬다(fs.names, fs.statmany, fs.info, fs.read, fs.write).
// 경로는 모두 절대 경로이고, 대시보드가 보이는 폴더(홈, 공유 폴더, 디스크) 안이어야 한다.

// SiteFiles는 파일 탐색기의 전역 설정이다(site.json).
type SiteFiles struct {
	// Shares는 관리자 세션이 OMV에서 받은 공유 폴더 목록이다(이름표). 실제로 닿을 수 있는 범위는 도우미가 정한다.
	Shares []SharedFolder `json:"shares,omitempty"`
	// SharesUpdatedAt은 Shares를 마지막으로 받은 시각(RFC 3339)이다. 비어 있으면 받은 적이 없다.
	SharesUpdatedAt string `json:"sharesUpdatedAt,omitempty"`
	// ShowDisks는 관리자에게 /srv 아래 디스크의 맨 위까지 보이는지다(디스크 최상위 보기).
	ShowDisks bool `json:"showDisks,omitempty"`
	// FollowPrivileges는 OMV 공유 폴더 권한(Privileges)을 파일 탐색기에도 적용하는지다. 권한이 없는 공유 폴더는
	// 보이지 않고, 읽기만 허락된 공유 폴더에는 쓰지 못한다(파일 시스템 권한도 함께 적용된다).
	FollowPrivileges bool `json:"followPrivileges,omitempty"`
}

// SharedFolder는 OMV 공유 폴더 하나다(ShareMgmt.enumerateSharedFolders).
type SharedFolder struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	// Path는 공유 폴더의 절대 경로다(OMV absdirpath).
	Path string `json:"path"`
	// Privileges는 OMV에 정한 사용자·그룹 권한이다(정하지 않은 사용자·그룹은 없다). 비어 있으면 정한 권한이 없다.
	Privileges []SharePrivilege `json:"privileges,omitempty"`
}

// SharePrivilege는 공유 폴더 권한 하나다(OMV privileges.privilege). Perms는 SharePerms* 중 하나다.
type SharePrivilege struct {
	Type  string `json:"type" tstype:"'user' | 'group'"`
	Name  string `json:"name"`
	Perms int    `json:"perms"`
}

// 공유 폴더 권한 값(OMV와 같다).
const (
	SharePermsNone      = 0
	SharePermsReadOnly  = 5
	SharePermsReadWrite = 7
)

// FilesRoots는 GET /api/files/roots의 응답이다. 파일 탐색기 사이드바에 처음 보이는 폴더다.
type FilesRoots struct {
	// Home은 로그인한 사용자의 홈 폴더다. 권한 도우미가 쓸 수 있는 홈이 없으면 없다.
	Home *FilesRoot `json:"home,omitempty"`
	// Shares는 공유 폴더 스냅숏이다(이름순).
	Shares []FilesShare `json:"shares"`
	// Disks는 /srv 아래 마운트다. 스냅숏이 없거나(Fallback) 관리자가 디스크 최상위 보기를 켰을 때 관리자에게만 있다.
	Disks []FilesRoot `json:"disks"`
	// Fallback은 공유 폴더 스냅숏을 아직 받지 못해 디스크를 대신 보인다는 뜻이다.
	Fallback bool `json:"fallback"`
	// SharesUpdatedAt은 스냅숏을 받은 시각(RFC 3339)이다.
	SharesUpdatedAt string `json:"sharesUpdatedAt,omitempty"`
	// FollowPrivileges는 OMV 공유 폴더 권한 따르기가 적용되고 있다는 뜻이다(켜져 있고 스냅숏이 있을 때).
	FollowPrivileges bool `json:"followPrivileges,omitempty"`
}

// FilesRoot는 사이드바 항목 하나다.
type FilesRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FilesShare는 사이드바의 공유 폴더 하나다.
type FilesShare struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Reachable은 권한 도우미가 닿을 수 있는 곳(/srv 아래 마운트)인지다. 루트 파일시스템의 공유 폴더는 아직 열 수 없다.
	Reachable bool `json:"reachable"`
	// ReadOnly는 OMV 공유 폴더 권한이 이 사용자에게 읽기만 허락한다는 뜻이다(공유 폴더 권한 따르기).
	ReadOnly bool `json:"readOnly,omitempty"`
}

// FilesListing은 GET /api/files/list의 응답이다. 폴더의 이름 목록을 서버가 세션별로 잠시 두고 ID로 가리킨다.
type FilesListing struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Total은 폴더의 항목 수다(찾기를 적용하지 않은 수).
	Total int `json:"total"`
	// Truncated는 항목이 상한(30만 개)을 넘어 일부만 읽었다는 뜻이다.
	Truncated bool `json:"truncated"`
	// Writable은 이 사용자가 이 폴더에 파일을 만들 수 있는지다(올리기).
	Writable bool `json:"writable"`
	// ShareReadOnly는 OMV 공유 폴더 권한이 읽기만 허락해 쓰지 못한다는 뜻이다(공유 폴더 권한 따르기). Writable도 false다.
	ShareReadOnly bool `json:"shareReadOnly,omitempty"`
}

// FilesPage는 GET /api/files/list/{id}의 응답이다. 정렬·찾기를 적용한 보기에서 offset부터의 항목이다.
type FilesPage struct {
	// Total은 보기의 항목 수다(찾기를 적용한 수).
	Total  int        `json:"total"`
	Offset int        `json:"offset"`
	Items  []FileItem `json:"items"`
}

// FileItem은 목록의 항목 하나다. names=1로 이름만 받으면 State가 pending이고 정보가 없다.
type FileItem struct {
	Name string `json:"name"`
	// Type은 항목 종류다. unknown은 파일시스템이 종류를 주지 않았고 아직 정보를 읽지 않았다는 뜻이다.
	Type string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other' | 'unknown'"`
	// State는 정보를 읽었는지다. gone은 읽는 사이에 지워졌다는 뜻, error는 읽지 못했다는 뜻이다.
	State string `json:"state" tstype:"'ok' | 'pending' | 'gone' | 'error'"`
	// Size는 일반 파일의 크기(바이트)다.
	Size int64 `json:"size,omitempty"`
	// ModTime은 수정 시각(Unix 밀리초)이다.
	ModTime int64 `json:"modTime,omitempty"`
	// Locked는 폴더인데 이 사용자가 들어갈 수 없다는 뜻이다.
	Locked bool `json:"locked,omitempty"`
	// LinkType은 심볼릭 링크를 따라간 대상의 종류다(깨졌거나 범위 밖이면 없다).
	LinkType string `json:"linkType,omitempty" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	// BadName은 이름이 UTF-8이 아니라 열거나 내려받을 수 없다는 뜻이다(Name은 바꿔 그린 이름이다).
	BadName bool `json:"badName,omitempty"`
}

// FilesFindResponse는 GET /api/files/list/{id}/find의 응답이다. 만들거나 이름을 바꾼 항목을 목록에서 고를 때 쓴다.
type FilesFindResponse struct {
	// Index는 보기(정렬·찾기)에서 그 이름의 자리다. 보기에 없으면 -1이다.
	Index int `json:"index"`
}

// FilesUploadCheckRequest는 POST /api/files/upload/check의 본문이다. PUT /api/files/upload로 본문을 보내기 전에
// 보안 확인·세션·이름·범위처럼 본문 없이 알 수 있는 거절을 먼저 받는다. 통과하면 응답 본문 없이 204다.
type FilesUploadCheckRequest struct {
	// Path는 올릴 파일의 절대 경로다.
	Path string `json:"path"`
}

// FilesMkdirRequest는 POST /api/files/mkdir의 본문이다. 응답은 만든 폴더의 FileItem이다.
type FilesMkdirRequest struct {
	// Path는 만들 폴더의 절대 경로다.
	Path string `json:"path"`
}

// FilesRenameRequest는 POST /api/files/rename의 본문이다. 이름을 바꾸거나 다른 폴더로 옮긴다(같은 디스크 안).
// 응답은 옮긴 뒤의 FileItem이다. To가 이미 있으면 file_exists이고 덮어쓰지 않는다.
type FilesRenameRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// FilesDeleteRequest는 POST /api/files/delete의 본문이다. 되돌릴 수 없는 삭제이고 응답 본문은 없다(204).
type FilesDeleteRequest struct {
	Path string `json:"path"`
	// Recursive면 폴더 안의 항목까지 모두 지운다. 아니면 파일과 빈 폴더만 지운다(not_empty).
	Recursive bool `json:"recursive"`
}

// FilesTrashRequest는 POST /api/files/trash의 본문이다. 항목을 휴지통으로 옮긴다.
type FilesTrashRequest struct {
	Path string `json:"path"`
}

// FilesCopyRequest는 POST /api/files/copy의 본문이다. Dir 안의 Names(512개까지)를 To 폴더에 같은 이름으로
// 복사한다(다른 디스크로 끌어 놓기). 원본은 그대로 두고, To에 같은 이름이 있으면 그 이름은 복사하지 않는다.
type FilesCopyRequest struct {
	Dir   string   `json:"dir"`
	Names []string `json:"names"`
	To    string   `json:"to"`
}

// FilesSearchEvent는 GET /api/files/search 응답(text/event-stream)의 사건 하나다(data 줄의 JSON). 진행(Hits, Dirs)이
// 여러 번 오고 마지막에 Result나 Error 하나가 온다. 시작하기 전의 실패는 사건이 아니라 보통의 오류 응답이다.
type FilesSearchEvent struct {
	// Hits는 앞 사건 뒤에 새로 찾은 항목이다.
	Hits []FilesSearchHit `json:"hits,omitempty"`
	// Dirs는 지금까지 읽은 폴더 수, Skipped는 읽지 못해 건너뛴 폴더 수다.
	Dirs    int                `json:"dirs"`
	Skipped int                `json:"skipped,omitempty"`
	Result  *FilesSearchResult `json:"result,omitempty"`
	// Error는 검색을 시작한 뒤 멈춘 오류이고, Status는 같은 오류를 응답으로 받았을 때의 HTTP 상태다.
	Error  *ErrorResponse `json:"error,omitempty"`
	Status int            `json:"status,omitempty"`
}

// FilesSearchHit은 찾은 항목 하나다. Path는 절대 경로, Type은 FileItem.Type과 같다(종류를 모르면 unknown).
type FilesSearchHit struct {
	Path string `json:"path"`
	Type string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other' | 'unknown'"`
}

// FilesSearchResult는 검색의 끝이다. Truncated면 찾은 항목이 상한(2000개)에 닿아 멈췄다.
type FilesSearchResult struct {
	Dirs      int  `json:"dirs"`
	Skipped   int  `json:"skipped,omitempty"`
	Truncated bool `json:"truncated,omitempty"`
}

// FilesCopyEvent는 POST /api/files/copy 응답(text/event-stream)의 사건 하나다(data 줄의 JSON). Progress가 여러 번
// 오고 마지막에 Result나 Error 하나가 온다. 시작하기 전의 실패는 사건이 아니라 보통의 오류 응답이다.
type FilesCopyEvent struct {
	Progress *FilesCopyProgress `json:"progress,omitempty"`
	Result   *FilesCopyResult   `json:"result,omitempty"`
	// Error는 복사를 시작한 뒤 멈춘 오류이고, Status는 같은 오류를 응답으로 받았을 때의 HTTP 상태다.
	Error  *ErrorResponse `json:"error,omitempty"`
	Status int            `json:"status,omitempty"`
}

// FilesCopyProgress는 복사의 진행이다. Scanning이면 아직 복사할 양을 세는 중이라 Total이 늘어난다. Files는 폴더가
// 아닌 항목 수, Bytes는 파일 내용의 바이트 수, Current는 지금 복사하는 항목의 Dir 안 상대 경로다.
type FilesCopyProgress struct {
	Scanning   bool   `json:"scanning,omitempty"`
	Files      int64  `json:"files"`
	Bytes      int64  `json:"bytes"`
	TotalFiles int64  `json:"totalFiles"`
	TotalBytes int64  `json:"totalBytes"`
	Current    string `json:"current,omitempty"`
}

// FilesCopyResult는 복사의 결과다. Skipped는 폴더 안에서 복사하지 않은 특수 파일(FIFO, 소켓, 장치)의 수다.
type FilesCopyResult struct {
	Copied  []string           `json:"copied"`
	Failed  []FilesCopyFailure `json:"failed"`
	Skipped int                `json:"skipped"`
}

// FilesCopyFailure는 복사하지 못한 이름 하나와 그 이유(ErrorResponse.Code와 같은 값)다.
type FilesCopyFailure struct {
	Name string `json:"name"`
	Code string `json:"code" tstype:"NonNullable<ErrorResponse['code']>"`
}

// FilesTrashResult는 POST /api/files/trash의 응답이다. 넣은 휴지통과 그 안의 이름이다(되돌릴 때 쓴다).
type FilesTrashResult struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
}

// TrashVolume은 홈 폴더나 디스크 하나의 휴지통이다.
type TrashVolume struct {
	// Top은 홈 폴더나 디스크(/srv 아래 마운트 지점)의 경로다. 그 안의 항목은 이 휴지통에 들어간다.
	Top string `json:"top"`
	// Kind는 휴지통 종류다: home(홈 휴지통), shared(디스크의 .Trash/<uid>), own(디스크의 .Trash-<uid>),
	// none(휴지통이 없어 영구 삭제만 할 수 있다).
	Kind string `json:"kind" tstype:"'home' | 'shared' | 'own' | 'none'"`
	// Shared는 디스크 맨 위에 모든 사용자가 쓰는 .Trash가 있는지다.
	Shared bool `json:"shared"`
}

// FilesTrashVolumes는 GET /api/files/trash/volumes의 응답이다.
type FilesTrashVolumes struct {
	Volumes []TrashVolume `json:"volumes"`
	// Items는 휴지통의 항목 수다(정보를 읽지 않고 센 값, 사이드바의 개수).
	Items int `json:"items"`
}

// TrashItem은 휴지통 항목 하나다.
type TrashItem struct {
	// Trash는 휴지통 디렉터리, Name은 그 안의 이름이다(되돌리기·영구 삭제에 그대로 보낸다).
	Trash string `json:"trash"`
	Name  string `json:"name"`
	// Path는 원래 경로다.
	Path string `json:"path"`
	// DeletedAt은 지운 시각(Unix 밀리초)이다. 모르면 0이다.
	DeletedAt int64  `json:"deletedAt"`
	Type      string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	Size      int64  `json:"size"`
	// ModTime은 수정 시각(Unix 밀리초)이다.
	ModTime int64 `json:"modTime"`
	// BadName은 이름이나 원래 경로가 UTF-8이 아니라 되돌리거나 지울 수 없다는 뜻이다.
	BadName bool `json:"badName,omitempty"`
}

// FilesTrashList는 GET /api/files/trash의 응답이다(지운 시각이 늦은 것부터).
type FilesTrashList struct {
	Items []TrashItem `json:"items"`
	// Truncated는 항목이 많아 일부만 담았다는 뜻이다.
	Truncated bool `json:"truncated"`
}

// FilesTrashRestoreRequest는 POST /api/files/trash/restore의 본문이다. 항목을 To(원래 경로나 같은 폴더의 새 이름)로
// 되돌린다. 응답은 되돌린 FileItem이다.
type FilesTrashRestoreRequest struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
	To    string `json:"to"`
}

// FilesTrashItemRequest는 POST /api/files/trash/purge의 본문이다. 휴지통 항목 하나를 영구 삭제한다(204).
type FilesTrashItemRequest struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
}

// SiteTrashDisk는 디스크 하나의 공유 휴지통(.Trash) 상태다(관리자).
type SiteTrashDisk struct {
	// Name은 디스크 이름표(라벨, 없으면 마운트 지점 이름), Path는 마운트 지점이다.
	Name string `json:"name"`
	Path string `json:"path"`
	// Shared는 디스크 맨 위에 모든 사용자가 쓰는 .Trash(1777)가 있는지다.
	Shared bool `json:"shared"`
}

// SiteTrashDisks는 GET·POST /api/site/files/trash의 응답이다(관리자).
type SiteTrashDisks struct {
	Disks []SiteTrashDisk `json:"disks"`
}

// SiteTrashPrepareRequest는 POST /api/site/files/trash의 본문이다. 그 디스크 맨 위에 .Trash(1777)를 만든다
// (root 권한 상승 필요).
type SiteTrashPrepareRequest struct {
	Disk string `json:"disk"`
}

// FilesPrepareResponse는 POST /api/files/list/{id}/prepare의 응답이다. Filled가 Total이면 날짜·크기순으로 볼 수 있다.
type FilesPrepareResponse struct {
	Filled int `json:"filled"`
	Total  int `json:"total"`
}

// FileInfo는 GET /api/files/info의 응답이다(정보 패널).
type FileInfo struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Type     string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"`
	Mode     uint32 `json:"mode"`
	Target   string `json:"target,omitempty"`
	LinkType string `json:"linkType,omitempty" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	// Owner, Group은 이름이다. 찾지 못했으면 없다(화면은 UID, GID를 보인다).
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	// ACL은 POSIX ACL이 있는지다. 확인하지 못했으면 없다(심볼릭 링크 포함).
	ACL *bool `json:"acl,omitempty"`
	// Access는 이 사용자가 읽고 쓸 수 있는지다. 확인하지 못했으면 없다(심볼릭 링크 포함).
	Access *FileAccess `json:"access,omitempty"`
	// ShareUUID는 이 경로가 속한 공유 폴더다(관리자에게 OMV ACL 화면 링크를 줄 때 쓴다). 관리자에게만 준다.
	ShareUUID string `json:"shareUuid,omitempty"`
	// ShareReadOnly는 OMV 공유 폴더 권한이 읽기만 허락한다는 뜻이다(공유 폴더 권한 따르기). Access.Write도 false다.
	ShareReadOnly bool `json:"shareReadOnly,omitempty"`
}

// FileAccess는 이 사용자의 권한이다. 폴더는 들어가 목록을 보는 것이 읽기, 안에 만드는 것이 쓰기다.
type FileAccess struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// FilesSettings는 GET /api/site/files의 응답이자 POST /api/site/files/refresh의 응답이다(관리자).
type FilesSettings struct {
	Shares           []SharedFolder `json:"shares"`
	SharesUpdatedAt  string         `json:"sharesUpdatedAt,omitempty"`
	ShowDisks        bool           `json:"showDisks"`
	FollowPrivileges bool           `json:"followPrivileges"`
}

// FilesSettingsRequest는 PUT /api/site/files의 본문이다. 보내지 않은 설정은 그대로 둔다.
type FilesSettingsRequest struct {
	ShowDisks        *bool `json:"showDisks,omitempty"`
	FollowPrivileges *bool `json:"followPrivileges,omitempty"`
}
