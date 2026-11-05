package main

import (
	"fmt"
	"os"
	"strings"

	"maru/backend/internal/omv/omvfake"
)

// loadExtraRPC는 FAKEOMV_EXTRA_RPC(콜론으로 나눈 dev/fake-rpc.json 경로)의 파일을 읽어 합친다. 형식은
// omvfake.ParseExtraRPC다. 여러 파일이 같은 메서드를 적으면 앞 파일의 것을 쓰고 logf로 알린다(개발 패키지 두 개가
// 같은 플러그인 메서드를 흉내 낼 수 있다). 읽을 수 없거나 형식이 틀린 파일은 오류다(조용히 빠지면 개발자가 헷갈린다).
func loadExtraRPC(list string, logf func(string, ...any)) (map[string]omvfake.ExtraRPC, error) {
	out := map[string]omvfake.ExtraRPC{}
	from := map[string]string{}
	for _, path := range strings.Split(list, ":") {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		m, err := omvfake.ParseExtraRPC(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for name, e := range m {
			service, method, _ := strings.Cut(name, ".")
			key := strings.ToLower(service) + "." + method
			if prev, ok := from[key]; ok {
				logf("fake omv: %s in %s is ignored, %s already defines it", name, path, prev)
				continue
			}
			from[key] = path
			out[name] = e
		}
	}
	return out, nil
}
