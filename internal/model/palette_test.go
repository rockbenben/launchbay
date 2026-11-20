package model

import "testing"

// 色板的对比度判据**不在这一侧**：色点画在 h2::before 上，底是 base.css 的
// --bg，所以那条判据住在 render 的 TestCSS_分组色板在两种页面底上都过3比1——
// 底色从 CSS 现读，改色板不会让两枚手抄的十六进制悄悄过期。
//
// 2026-11-19 之前它住在这里，底色是两枚手抄的值；当天改色板（--bg #0b0e14 →
// #131b29、#f6f4ef → #f0f4fa）之后那两枚还停在旧值，闸照绿——它量的是一个
// 屏幕上已经不存在的底。这一侧只留下面两条。

// 反向证人：**不许退回"看起来差不多"的那一批**。
//
// **只有六颗在名单里** —— 红 `#e0574f`（亮档 3.37）与紫 `#9b6ce7`（亮档 3.34，
// 2026-10-14 由 #9b6ce8 微调了一位）本来就过 3:1，所以它们**没有被压暗**，仍留在
// 色板上。把它们列进这个名单就等于要求"必须改掉两颗本来就合格的颜色"，那是这条闸
// 自己站不住。
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
