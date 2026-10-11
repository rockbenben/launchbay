package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// Settings 是**站级**设置：不属于任何一个档案，也不进 profile.json。
//
// 为什么语言不进 profile.json：语言是一家人共用的阅读偏好，而项目的一条硬边界是
// 「管理员看不见别人的书签、内容各存各的」。把语言放进档案，等于给了任何一份档案的
// 所有者改动**全机**显示语言的权力——那既是权限模型的破口，也让 Language 从一个
// 纯显示设置变成了需要仲裁的状态。放在站级并只让管理员改，这条边界才是干净的。
//
// 为什么站级设置不是档案级设置的特例：档案级的 theme/layout 存在 profile.json 里
// 随档案走（P-③），而这份文件在 data/ 根下、删档不动它、换人不动它。两者的生命周期
// 不同，所以是两个东西而不是同一份数据上的两个字段。
type Settings struct {
	// Language 是界面语言。**缺字段 = 中文**（i18n.Default），于是升级到带多语言的
	// 版本不改变任何一个人的现有界面。零值 i18n.Lang("") 经 ParseLang 落回默认。
	Language i18n.Lang `json:"language"`
}

// SettingsPath 是站级设置的落盘位置。data/ 根下，与 profiles/ 平级——
// 删掉 profiles/<id> 不动它，重建一台机器时它跟着 data 目录走。
func (s *Store) SettingsPath() string { return filepath.Join(s.root, "settings.json") }

// Language 读站级语言，**任何读不出来的情况都落回默认**。
//
// 刻意不返回 error：语言是纯显示层。一个显示配置读不出来就让整台机器每一页都
// 500，是把「装饰」抬到「生死开关」的高度。读不出来时界面显示中文——
// 一定能用、也一定是这个项目原本的样子，而用户看到的是中文而不是一屏错误页。
//
// 三种读不出来：文件不存在（首次装机）、读不了（权限）、内容是坏的。都落默认。
//
// **刻意不做内存缓存**：这样管理员在管理页改完立刻生效，不必重启容器；缓存会
// 让「改了不生效」变成一个要靠重启才能绕开的怪问题，而这里省下的那点 IO 连
// profile.json 的一次读都不如——页面本来就要读它。
func (s *Store) Language() i18n.Lang {
	data, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		return i18n.Default
	}
	var st Settings
	if err := json.Unmarshal(data, &st); err != nil {
		return i18n.Default
	}
	// 文件里存了不认识的值（手改写错、旧版本留下）也落默认，与 ParseLang 一份判据。
	return i18n.ParseLang(string(st.Language))
}

// SetLanguage 写站级语言。
//
// 落盘走 WriteFileJSON：与 bookmarks.json / profile.json 同一套编码与原子替换，
// 不另起炉灶。写入前归一化——传进来的值若不受支持，一律落成默认，**不把垃圾
// 写进磁盘**：存一份自己读不回来的配置，是那种几个月后才有人发现的坏形状。
func (s *Store) SetLanguage(l i18n.Lang) error {
	if !l.Valid() {
		l = i18n.Default
	}
	if err := s.WriteFileJSON(s.SettingsPath(), Settings{Language: l}); err != nil {
		return errors.Join(ErrInvalid, err)
	}
	return nil
}
