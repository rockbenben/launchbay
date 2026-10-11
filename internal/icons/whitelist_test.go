package icons

import "testing"

// TestValidAttrValue_正则的两面 把 attrValueRE 的两个方向各钉一遍：
//
//   - 正向控制：产物里真实会出现的值必须放行。正则被**写窄**（误拒）时，
//     全套测试里只有下面 legal 这五行会响。
//   - 逐字符类断言：对每一个被具名禁掉的字符各断一次"含它的值一律被拒"。
//     这一半是本轮补的正向证人——旧的「值里藏引号」用例（index_test.go）
//     把 `"` `<` `>` 塞进同一个串，把正则换成只禁 `<` 的 `^[^<]*$` 时它靠
//     那一处的 `<` 照样能红：那是连带命中，不是"引号被拒"这件事被钉住。
//     逐字符各断一次之后，「禁的是一组具名字符」与「只禁 `<`」这两种实现
//     在这条测试下必然一绿一红，再放宽一个字符就会响。
//
// 第二道闸在渲染层（whitelist.go 与 spec §11 的口径：渲染时还要再转义一次，
// Task 9 落地），但白名单这道闸被写宽时原本没人会响——这是这条测试存在的全部理由。
func TestValidAttrValue_正则的两面(t *testing.T) {
	// 正向控制（同时也是"不含任何禁字符的合法值必须被接受"这一侧的断言）。
	for _, legal := range []string{"M3 12h18", "0 0 1 1", "currentColor", "12", ".5"} {
		if !ValidAttrValue(legal) {
			t.Errorf("合法值 %q 被拒了：正则写窄就体现在这一行", legal)
		}
	}
	// 逐字符类：每个禁字符单独出场，且都不带 `<`，免得又靠连带命中蒙过去。
	for _, c := range []string{`"`, "'", "<", ">", "&", "="} {
		for _, v := range []string{c, "M3 12h18" + c, c + "currentColor"} {
			if ValidAttrValue(v) {
				t.Errorf("含 %q 的值 %q 居然放行：正则被写宽（例如只禁 <）就体现在这一行", c, v)
			}
		}
	}
}

func Test白名单谓词真的在判事(t *testing.T) {
	if IsDrawable("use") || IsDrawable("script") {
		t.Error("use/script 不该算可绘制元素")
	}
	if !IsDrawable("path") || !IsDrawable("polyline") {
		t.Error("path/polyline 必须在白名单内")
	}
	if IsKeptAttr("stroke-width") || IsKeptAttr("onclick") {
		t.Error("呈现属性与事件属性都不该进产物")
	}
	if !IsKeptAttr("points") {
		t.Error("polyline 靠 points 活")
	}
	// brief 里的 "a b\"c" 是同一个串：内联双引号在 Go 字面量里必须转义，
	// 否则测试文件自己都编译不过。
	for bad := range map[string]bool{`"><script>`: true, "a b\"c": true, "x='y'": true} {
		if ValidAttrValue(bad) {
			t.Errorf("ValidAttrValue(%q) 不该为真", bad)
		}
	}
	if !ValidAttrValue("M3 12h18") || !ValidAttrValue("currentColor") || !ValidAttrValue("rotate(45 12 12)") {
		t.Error("合法值被拒了：路径指令、颜色名、transform 都得放行")
	}
	// 层默认值：判错方向 = tabler 的实心变体被当成默认丢掉（外观错，无报错）
	if !IsLayerDefaultFill("stroke", "none") || IsLayerDefaultFill("stroke", "currentColor") {
		t.Error("stroke 层的默认 fill 判错了")
	}
	if !IsLayerDefaultFill("fill", "currentColor") || IsLayerDefaultFill("fill", "none") {
		t.Error("fill 层的默认 fill 判错了")
	}
	// B10：层的枚举只有 layerFillDefaults 的键这一份，IsLayer 是加载侧与
	// 渲染侧共用的唯一入口。已知两层各断 true、未知值（含空串与大小写变体）
	// 各断 false——把 IsLayer 换回字面量抄进某个 switch 并删掉谓词，这里编译就红。
	if !IsLayer("stroke") || !IsLayer("fill") {
		t.Error("stroke/fill 是已知层，IsLayer 说不认识")
	}
	for _, notLayer := range []string{"", "outline", "Stroke", "FILL", "both"} {
		if IsLayer(notLayer) {
			t.Errorf("IsLayer(%q) 不该为真——层枚举不许被放宽", notLayer)
		}
	}
}
