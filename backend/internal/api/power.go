package api

// 다시 시작 예약의 시각 범위. 지금부터 PowerScheduleMinAheadSeconds 뒤 이상, PowerScheduleMaxAheadSeconds 뒤 이하만
// 예약한다(권한 도우미도 7일 상한을 다시 확인한다).
const (
	PowerScheduleMinAheadSeconds = 60
	PowerScheduleMaxAheadSeconds = 7 * 24 * 60 * 60
)

// PowerSchedule은 systemd-logind에 예약된 종료다. 대시보드 밖(SSH의 shutdown 등)에서 한 예약도 보인다.
type PowerSchedule struct {
	// At은 예약 시각(RFC 3339, UTC)이다.
	At string `json:"at"`
	// Mode는 종료 종류다. reboot(다시 시작), poweroff(시스템 종료), other(그 밖의 종료)다.
	Mode string `json:"mode" tstype:"'reboot' | 'poweroff' | 'other'"`
	// By는 대시보드에서 예약한 사용자 이름이다. 대시보드 밖에서 예약했으면 없다.
	By string `json:"by,omitempty"`
}

// PowerScheduleResponse는 GET·PUT·DELETE /api/power/schedule의 응답이다.
type PowerScheduleResponse struct {
	// Schedule은 지금 예약이다. 없으면 null이다.
	Schedule *PowerSchedule `json:"schedule"`
	// Now는 NAS의 지금 시각(RFC 3339, NAS 시간대의 오프셋)이다. 브라우저 시계와의 차이를 잴 때와 TimeZone을 모를 때
	// NAS 시각을 보일 때 쓴다.
	Now string `json:"now"`
	// TimeZone은 NAS 시간대의 IANA 이름이다(예: Asia/Seoul). 알 수 없으면 없다.
	TimeZone string `json:"timeZone,omitempty"`
}

// PowerScheduleRequest는 PUT /api/power/schedule의 요청 본문이다. 다시 시작을 예약한다(이미 있는 예약은 바뀐다).
type PowerScheduleRequest struct {
	// At은 다시 시작할 시각(RFC 3339)이다. 초 아래는 버린다.
	At string `json:"at"`
}
