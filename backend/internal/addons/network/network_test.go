package network

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestParseNetDev(t *testing.T) {
	f, err := os.Open("testdata/net-dev")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names, c, err := ParseNetDev(f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "lo,eth0,docker0,veth1234,eth1,wg0" {
		t.Errorf("names %v", names)
	}
	want := Counters{RxBytes: 123456789, RxPackets: 1000, RxErrors: 1, RxDropped: 2,
		TxBytes: 987654321, TxPackets: 2000, TxErrors: 3, TxDropped: 4}
	if c["eth0"] != want {
		t.Errorf("eth0 %+v", c["eth0"])
	}
}

func TestParseNetDevRejectsGarbage(t *testing.T) {
	_, _, err := ParseNetDev(strings.NewReader("eth0: 1 2 3 4 5 6 7 8 x 10 11 12 13 14 15 16\n"))
	if err == nil {
		t.Error("want error")
	}
}

func TestNetwork(t *testing.T) {
	dev, _ := os.ReadFile("testdata/net-dev")
	proc := fstest.MapFS{"net/dev": {Data: dev}}
	sys := fstest.MapFS{
		"eth0/operstate": {Data: []byte("up\n")},
		"eth0/speed":     {Data: []byte("2500\n")},
		"eth0/device":    {Data: []byte("../../../0000:00:1f.6"), Mode: os.ModeSymlink},
		"eth1/operstate": {Data: []byte("down\n")},
		"eth1/device":    {Data: []byte("../../../0000:00:1f.7"), Mode: os.ModeSymlink},
		// speed 읽기 실패(링크 내려감)는 파일이 없는 것으로 흉내 낸다.
		"docker0/operstate": {Data: []byte("down\n")},
		"docker0/speed":     {Data: []byte("-1\n")},
		"wg0/speed":         {Data: []byte("-1\n")},
	}
	clk := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := NewReaderFS(proc, sys, func() time.Time { return clk })
	clk = clk.Add(1500 * time.Millisecond)
	res, err := r.Network()
	if err != nil {
		t.Fatal(err)
	}
	if res.SampledAtMs != 1500 || len(res.Sources) != 1 {
		t.Errorf("sampledAtMs %d sources %v", res.SampledAtMs, res.Sources)
	}
	var got []string
	for _, i := range res.Interfaces {
		got = append(got, i.Name)
	}
	if strings.Join(got, ",") != "eth0,eth1,docker0,wg0" {
		t.Fatalf("order %v", got)
	}
	eth0 := res.Interfaces[0]
	if eth0.Virtual || eth0.State != "up" || eth0.SpeedMbps != 2500 || eth0.RxBytes != 123456789 || eth0.TxDropped != 4 {
		t.Errorf("eth0 %+v", eth0)
	}
	if eth1 := res.Interfaces[1]; eth1.Virtual || eth1.State != "down" || eth1.SpeedMbps != 0 {
		t.Errorf("eth1 %+v", eth1)
	}
	if d := res.Interfaces[2]; !d.Virtual || d.SpeedMbps != 0 {
		t.Errorf("docker0 %+v", d)
	}
	if w := res.Interfaces[3]; !w.Virtual || w.State != "unknown" {
		t.Errorf("wg0 %+v", w)
	}
}
