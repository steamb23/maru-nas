package api

// AddonSDKLevel은 이 대시보드가 지원하는 애드온 패키지 SDK API 수준이다. 패키지 manifest의 sdk가 이보다 크면
// "호환 안 됨"이다. 하위 호환을 지키며 올린다(.claude/docs/addon-packages.md).
const AddonSDKLevel = 1
