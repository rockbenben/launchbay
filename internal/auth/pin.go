// Package auth 只管三件事：PIN 哈希与比对、令牌配置解析、会话 cookie 读写。
// 按 spec §11 的取舍，这里刻意不做：限速、锁定、失败计数、cookie 签名。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const pinAlgo = "sha256"

// PinMinLen / PinMaxLen 是 PIN 的长度范围，**判据只有这一处**。
//
// 同一个数要出现在两处——服务端那条通路判它、模板上那个 minlength/maxlength
// 印它。两处各写一个字面量的话，改一处要记得改另外两处，而漏掉的那一处症状是
// "界面上限与实际判的对不上"（浏览器拦住了、curl 拦不住）。
//
// 为什么住在 auth 而不是某一层：它判的是**这个 PIN 本身合不合法**，而 auth
// 正是唯一判这件事的地方（ValidPinFormat、HashPIN、VerifyPIN 都在这儿）。
const (
	PinMinLen = 4
	PinMaxLen = 6
)

var ErrBadPin = fmt.Errorf("PIN 要 %d-%d 位数字", PinMinLen, PinMaxLen)

// ValidPinFormat 只接受 ASCII 数字。全角数字不算合法：有人用输入法打成
// 全角时要的是"这条被拒"，而不是"PIN 悄悄变成另一个值"。
func ValidPinFormat(pin string) bool {
	if len(pin) < PinMinLen || len(pin) > PinMaxLen {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// HashPIN 产出 "sha256$<盐>$<摘要>"。
func HashPIN(pin string) (string, error) {
	if !ValidPinFormat(pin) {
		return "", ErrBadPin
	}
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("取随机盐: %w", err)
	}
	sum := sha256.Sum256(append(salt, pin...))
	return fmt.Sprintf("%s$%s$%s", pinAlgo, hex.EncodeToString(salt), hex.EncodeToString(sum[:])), nil
}

func VerifyPIN(stored, pin string) bool {
	algo, saltHex, wantHex, err := splitPinHash(stored)
	if err != nil || algo != pinAlgo {
		return false
	}
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(wantHex)
	if err != nil {
		return false
	}
	if !ValidPinFormat(pin) {
		// 仍然算一次，免得"格式错"成一个能用耗时量出来的早退分支。
		pin = "0000"
	}
	sum := sha256.Sum256(append(salt, pin...))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}

// HasPin 回答"这一份设了 PIN 吗"（2026-10-12）。
//
// **空哈希就是"不设防"**，这是新增的那条约定：建档时可以不设 PIN，此后
// `Profile.PIN` 为空串。判据只有这一处——`VerifyPIN` 仍然只认三段式哈希，
// 空串在它那里一律 false，所以"空档案能不能进"这个问题不会有两个答案。
func HasPin(stored string) bool { return stored != "" }

// VerifyEntry 是**进门**那一步的判据，而 VerifyPIN 是"这个 PIN 对不对"的判据。
// 两者分开的理由：一份没有 PIN 的档案，进来不需要任何 PIN——不是"空 PIN 也算对"，
// 而是**这一份本来就没有要核对的东西**。混进 VerifyPIN 的话，`VerifyPIN("", "")`
// 与 `VerifyPIN("", "1234")` 会给出同一个答案，而它们在语义上该是同一件事的两种写法。
func VerifyEntry(stored, pin string) bool {
	if !HasPin(stored) {
		return true
	}
	return VerifyPIN(stored, pin)
}

func splitPinHash(stored string) (algo, salt, digest string, err error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 {
		return "", "", "", errors.New("PIN 哈希形状应为 algo$盐$摘要")
	}
	return parts[0], parts[1], parts[2], nil
}
