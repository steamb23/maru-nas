package config

// Version은 마루 NAS 버전이다(SemVer 0.y.z). 정보 페이지와 OMV를 호출할 때 쓰는 User-Agent에 들어가고,
// 설치기는 설치 기록과 업그레이드 계획에 쓴다.
// OMV는 세션을 User-Agent에 묶으므로 바꾸면 백엔드를 재시작할 때처럼 모든 사용자가 다시 로그인한다.
const Version = "0.3.1"
