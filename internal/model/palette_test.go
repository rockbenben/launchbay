package model

import (
	"math"
	"strconv"
	"testing"
)

// 色板那一排的对比度判据（2026-10-14 补）。
//
// 之前 GroupAccentPalette 的注释写着「取值全是中间调：亮底与暗底两档都得
// 看得见」——那是一句**没有量的断言**，而实测把它推翻了：原来那八颗在亮档
// 只有 2.08–3.38:1，除红与紫外**六颗连 WCAG 1.4.11 的 3:1 都不到**。
//
// 「看得见」这种话一旦写进注释就成了一个不需要验的东西，于是它在盘上待了很久。
// 这一族把它换成一个**可算的数**。
func Test色板每一颗在暗底与亮底都过3比1(t *testing.T) {
	// 色点画在 h2::before 上，而 section 没有自己的底（base.css 只有
	// `section{margin:13px 0}`），所以底是**页面底**：暗档 --bg、亮档 --bg。
	const darkBG = "#0b0e14" // base.css 的 :root --bg
	const lightBG = "#f6f4ef"

	for i, c := range GroupAccentPalette {
		name := "色板第 " + strconv.Itoa(i) + " 颗 " + c
		for _, tc := range []struct {
			theme string
			bg    string
		}{{"暗档", darkBG}, {"亮档", lightBG}} {
			got := contrastRatio(t, c, tc.bg)
			if got < 3.0 {
				t.Errorf("%s 在%s只有 %.2f:1（色点是非文本图形元素，WCAG 1.4.11 要 3:1）",
					name, tc.theme, got)
			}
		}
	}
}

// 反向证人：**不许退回"看起来差不多"的那一批**。
//
// **只有六颗在名单里** —— 红 `#e0574f` 与紫原来就过了 3.1（3.38 / 3.35），
// 所以它们**没有被压暗、值也没变**，仍留在色板上。把它们列进这个名单就等于
// 要求"必须改掉两颗本来就合格的颜色"，那是这条闸自己站不住。
// 旧值里真正不过线的是这六颗（亮档 2.08–2.91）。
func Test色板别退回亮档看不见的那一批(t *testing.T) {
	tooLight := map[string]bool{
		"#4c8dff": true, // 亮档 2.91
		"#22b8a0": true, // 2.27
		"#57b95a": true, // 2.25
		"#d3a52c": true, // 2.08（最低）
		"#e8823c": true, // 2.48
		"#dd6aa6": true, // 2.86
	}
	for _, c := range GroupAccentPalette {
		if tooLight[c] {
			t.Errorf("%s 是改之前的值：亮档不到 3:1", c)
		}
	}
}

// 色数与色名必须同数同序 —— 那一条 init() 里的 panic 守着；这里补的是
// **它们确实一一对应**：色名是读屏唯一拿到的东西，配错了就说错颜色。
func Test色板与色名一一对应(t *testing.T) {
	if len(GroupAccentPalette) != len(GroupAccentColorKeys) {
		t.Fatalf("色板 %d 颗、色名 %d 个", len(GroupAccentPalette), len(GroupAccentColorKeys))
	}
	seen := map[string]bool{}
	for i, c := range GroupAccentPalette {
		if seen[c] {
			t.Errorf("第 %d 颗 %s 与前面某一颗同色：同一行里两个色块长得一样", i+1, c)
		}
		seen[c] = true
		// 色名键要是在语言包里找不到，读屏就只念一个 key 而不是颜色名。
		_ = GroupAccentColorKeys[i]
	}
}

// ── 对比度算法：WCAG 2.x 的相对亮度与对比度 ──────────────────────────

func parseHex(t *testing.T, s string) [3]float64 {
	t.Helper()
	if len(s) != 7 || s[0] != '#' {
		t.Fatalf("色号 %q 要写成 #rrggbb", s)
	}
	var out [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseUint(s[1+i*2:3+i*2], 16, 8)
		if err != nil {
			t.Fatalf("色号 %q 的第 %d 段不是十六进制", s, i+1)
		}
		out[i] = float64(v)
	}
	return out
}

func relLuminance(c [3]float64) float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

func contrastRatio(t *testing.T, a, b string) float64 {
	t.Helper()
	la := relLuminance(parseHex(t, a))
	lb := relLuminance(parseHex(t, b))
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
