// Package network는 네트워크 애드온이다. /proc/net/dev의 누적 카운터와 /sys/class/net의 링크 상태를 읽는다.
package network

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Counters는 /proc/net/dev 한 줄의 누적 값이다.
type Counters struct {
	RxBytes, RxPackets, RxErrors, RxDropped uint64
	TxBytes, TxPackets, TxErrors, TxDropped uint64
}

// ParseNetDev는 /proc/net/dev를 읽어 인터페이스 이름 → 누적 카운터를 반환한다. 순서도 함께 준다.
//
// 형식: 머리글 두 줄 뒤에 "이름: 수신 8개 송신 8개". 수신은 bytes packets errs drop fifo frame
// compressed multicast, 송신은 bytes packets errs drop fifo colls carrier compressed 순서다.
func ParseNetDev(r io.Reader) (names []string, counters map[string]Counters, err error) {
	counters = make(map[string]Counters)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if name == "" || len(f) < 16 {
			continue
		}
		var v [16]uint64
		for i := range v {
			if v[i], err = strconv.ParseUint(f[i], 10, 64); err != nil {
				return nil, nil, fmt.Errorf("net/dev %s: %w", name, err)
			}
		}
		names = append(names, name)
		counters[name] = Counters{
			RxBytes: v[0], RxPackets: v[1], RxErrors: v[2], RxDropped: v[3],
			TxBytes: v[8], TxPackets: v[9], TxErrors: v[10], TxDropped: v[11],
		}
	}
	return names, counters, sc.Err()
}
