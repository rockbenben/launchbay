package auth

import (
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := HashPIN("4812")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPIN(h, "4812") {
		t.Fatalf("自己的 PIN 验不过: %s", h)
	}
	if VerifyPIN(h, "4813") {
		t.Fatal("错 PIN 竟然通过")
	}
}

func TestSaltedSoSamePinGivesDifferentHash(t *testing.T) {
	a, err := HashPIN("4812")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPIN("4812")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("没有随机盐：同 PIN 同哈希")
	}
}

func TestPinFormat(t *testing.T) {
	for _, ok := range []string{"1234", "123456"} {
		if !ValidPinFormat(ok) {
			t.Fatalf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"123", "1234567", "12a4", "", "１２３４"} {
		if ValidPinFormat(bad) {
			t.Fatalf("%q 应非法", bad)
		}
	}
	if _, err := HashPIN("12a4"); err == nil {
		t.Fatal("HashPIN 必须自己再拦一次格式，不能只靠调用方")
	}
}

func TestVerifyRejectsMalformedStored(t *testing.T) {
	// "sha256$6162$zz"：盐是合法十六进制、摘要不是——hex.DecodeString
	// 的摘要分支此前没有夹具（账本 T6 minor），代码走错这步无人报警。
	for _, stored := range []string{"", "sha256$a", "sha256$a$b$c", "md5$a$b", "sha256$zz$b", "sha256$6162$zz"} {
		if VerifyPIN(stored, "4812") {
			t.Fatalf("畸形 stored 竟然通过: %q", stored)
		}
	}
}

// 全角数字必须走"格式不合法"这条路，而不是变成一个能过的 PIN。
// HashPIN 的错误必须断言：吞掉它（旧写法 a, _ :=）时，哈希一旦生成
// 失败，Verify("", 全角) 照样 false，测试就空过了——它什么都没钉。
func TestFullWidthPinNeverMatches(t *testing.T) {
	h, err := HashPIN("1234")
	if err != nil {
		t.Fatalf("夹具哈希必须真生成: %v", err)
	}
	if VerifyPIN(h, "１２３４") {
		t.Fatal("全角数字被判通过")
	}
}
