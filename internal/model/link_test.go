package model_test

import (
	"github.com/rockbenben/launchbay/internal/model"
	"net/netip"
	"testing"
)

func TestLANShapeOf_HostAndIPv6(t *testing.T) {
	for _, tc := range []struct{ host, mode, rule string }{
		{"10.0.0.5:7600", "lan", "private-ipv4"},
		{netip.AddrFrom4([4]byte{172, 16, 0, 1}).String(), "lan", "private-ipv4"},
		{netip.AddrFrom4([4]byte{192, 168, 1, 1}).String(), "lan", "private-ipv4"},
		{netip.AddrFrom4([4]byte{172, 32, 0, 1}).String(), "wan", "wan-domain"},
		{"127.0.0.1:7600", "lan", "loopback"},
		{"[::1]:7600", "lan", "loopback"},
		{"[fd00::5]:7600", "lan", "ula"},
		{"[fe80::5]:7600", "lan", "link-local"},
		{netip.AddrFrom4([4]byte{169, 254, 0, 1}).String(), "lan", "link-local"},
		{"[2001:db8::5]:7600", "wan", "wan-domain"},
		{"[::ffff:10.0.0.5]:7600", "wan", "wan-domain"},
		{"nas", "lan", "bare-host"},
		{"NAS.LOCAL.:7600", "lan", "local-domain"},
		{"nas.home.arpa", "lan", "local-domain"},
		{"nas.example.com:7600", "wan", "wan-domain"},
		{netip.AddrFrom4([4]byte{203, 0, 113, 9}).String() + ":7600", "wan", "wan-domain"},
		{"", "lan", "fallback"},
		{"nas:bad", "lan", "fallback"},
		{"[fd00::5]oops", "lan", "fallback"},
		{"[[::1]]:7600", "lan", "fallback"},
		{"[fe80::1%eth0]:7600", "lan", "link-local"},
		{"[fe80::1%eth'0]:7600", "lan", "fallback"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			got := model.LANShapeOf(tc.host, nil)
			if string(got.Mode) != tc.mode || got.Rule != tc.rule {
				t.Fatalf("%+v, want %s/%s", got, tc.mode, tc.rule)
			}
		})
	}
	if got := model.LANShapeOf("NAS.EXAMPLE.COM:7600", []string{"nas.example.com"}); got.Rule != "lan-host" {
		t.Fatal(got)
	}
}
