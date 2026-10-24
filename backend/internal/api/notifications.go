package api

// 알림 수준. Notice.Level에 들어간다.
const (
	NoticeInfo    = "info"
	NoticeSuccess = "success"
	NoticeWarning = "warning"
	NoticeError   = "error"
)

// 서버 알림 코드. Notice.Code에 들어가며 프론트가 코드로 문구를 고른다.
const (
	// NoticeSample은 개발 서버의 가짜 알림(MARU_DEV_NOTICES)과 테스트에만 쓰는 코드다. 운영 알림 출처는 쓰지 않는다.
	NoticeSample = "sample"
	// NoticeAptUpdates는 설치할 수 있는 APT 업데이트가 있다는 알림이다(Count: 패키지 수, Items: 패키지 이름).
	NoticeAptUpdates = "aptUpdates"
	// NoticeRebootRequired는 업데이트를 마치려면 다시 시작해야 한다는 알림이다(Items: 요청한 패키지 이름).
	NoticeRebootRequired = "rebootRequired"
	// NoticeJobSucceeded·Failed·Unknown은 서버 백그라운드 작업의 결과다(Job에 작업, 지울 수 있음). 시작한 사용자에게만 보인다.
	NoticeJobSucceeded = "jobSucceeded"
	NoticeJobFailed    = "jobFailed"
	NoticeJobUnknown   = "jobUnknown"
)

// 알림 동작 종류. NoticeAction.Kind에 들어간다.
const (
	// NoticeActionApp은 대시보드 앱을 연다. 앱이 설정이면 Page의 페이지로 연다.
	NoticeActionApp = "app"
	// NoticeActionWorkbench는 OMV 워크벤치의 Route 화면을 워크벤치 탭으로 연다.
	NoticeActionWorkbench = "workbench"
	// NoticeActionPower는 전원 대화상자를 연다.
	NoticeActionPower = "power"
)

// NoticeAction은 알림을 눌렀을 때 할 일이다.
type NoticeAction struct {
	Kind string `json:"kind" tstype:"'app' | 'workbench' | 'power'"`
	// App은 Kind가 app일 때 프론트 앱 id다. 프론트가 모르는 id면 동작을 보이지 않는다.
	App string `json:"app,omitempty"`
	// Page는 App이 설정일 때 열 페이지 id다.
	Page string `json:"page,omitempty"`
	// Route는 Kind가 workbench일 때 OMV 워크벤치 라우트다(앞 슬래시 없이, 예: system/updatemgmt/updates).
	Route string `json:"route,omitempty"`
}

// Notice는 서버가 알리는 상태 하나다(업데이트가 있음 등). 조건이 풀리면 목록에서 빠지며 사용자가 지울 수 없다.
// 예외로 작업 결과 알림(Dismissible)은 사용자가 지운다. 문구는 보내지 않고 Code와 값(Count, Items)만 보낸다.
type Notice struct {
	// ID는 알림의 고정 이름이다. 읽음 상태(NotificationPrefs.Seen)의 키다.
	ID   string `json:"id"`
	Code string `json:"code" tstype:"'sample' | 'aptUpdates' | 'rebootRequired' | 'jobSucceeded' | 'jobFailed' | 'jobUnknown'"`
	// Level은 Notice* 수준 중 하나다.
	Level string `json:"level" tstype:"'info' | 'success' | 'warning' | 'error'"`
	// Revision은 내용이 바뀔 때만 달라지는 값이다(내용 해시). 바뀌면 다시 읽지 않은 알림이 된다.
	Revision string `json:"revision"`
	// Since는 조건이 생긴 시각(RFC 3339)이다. 출처가 모르면 없다.
	Since string `json:"since,omitempty"`
	// Count는 개수(업데이트할 패키지 수 등)다. 문구의 복수형에 쓴다.
	Count *int `json:"count,omitempty"`
	// Items는 이름 목록(패키지 이름 등)이다. 앞쪽 일부만 담는다.
	Items  []string      `json:"items,omitempty"`
	Action *NoticeAction `json:"action,omitempty"`
	// Dismissible이면 사용자가 지울 수 있다(DELETE /api/notifications/{id}). 작업 결과 알림만 그렇다.
	Dismissible bool `json:"dismissible,omitempty"`
	// Job은 작업 결과 알림의 작업이다. 프론트가 그 작업의 출력을 연다.
	Job *NoticeJob `json:"job,omitempty"`
}

// NoticeJob은 작업 결과 알림이 가리키는 작업이다.
type NoticeJob struct {
	ID    string `json:"id"`
	Kind  string `json:"kind" tstype:"'apt-update' | 'apt-upgrade'"`
	State string `json:"state" tstype:"'running' | 'succeeded' | 'failed' | 'unknown'"`
	Code  string `json:"code,omitempty" tstype:"'apt_failed' | 'apt_core_removal' | 'job_failed' | 'job_lost' | 'job_reboot' | 'job_timeout'"`
}

// NotificationsResponse는 GET /api/notifications의 응답이다.
type NotificationsResponse struct {
	// Notices는 이 사용자가 볼 수 있는 서버 알림이다(관리자 전용 알림은 관리자에게만).
	Notices []Notice `json:"notices"`
	// Seen은 이 사용자가 읽은 알림의 id → Revision이다. 지금 Notices에 있는 id만 담는다.
	Seen map[string]string `json:"seen"`
	// Tasks는 진행 중인 서버 백그라운드 작업이다(관리자에게만, 아니면 빈 목록).
	Tasks []Job `json:"tasks"`
}

// NotificationPrefs는 사용자의 알림 읽음 상태다.
type NotificationPrefs struct {
	// Seen은 읽은 서버 알림의 id → 읽을 때의 Revision이다. 프론트는 지금 목록에 있는 id만 보낸다.
	Seen map[string]string `json:"seen"`
}

// NotificationPrefsRequest는 PUT /api/preferences/notifications의 요청 본문이다.
type NotificationPrefsRequest struct {
	Notifications *NotificationPrefs `json:"notifications"`
}
