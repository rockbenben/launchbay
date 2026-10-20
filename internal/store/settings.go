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
// **2026-10-20 改判：语言不再是"只有这一份"。** 从前语言整个住在站级，理由
// 写在这里——"一家人共用一台导航台，谁也不该被别人改掉阅读语言，所以它不进
// profile.json"。那条理由在**管理员变成一把机器级的 key 之后**不成立了：
// 管理员不属于任何档案，而"这一份用哪种语言"是这一份自己的事。于是：
//
//   - **每一份档案自己记一份语言**（model.Profile.Language，住在 profile.json）：
//     读它自己时用自己那份，**没有就跟着这里的默认**（i18n.ProfileLang 是
//     唯一那份判据）；
//   - **这里这一份是"默认"**：管理员在管理页改它，它只影响**还没自己选过**
//     的人；已经选过的人不受影响。
//
// 改完之后，那条边界**没有被破**：从前的破口是"一份档案的所有者能改全机的
// 显示语言"（权限模型的一个洞）；现在每一份只能改自己那一份，管理员改的是
// 默认。谁也改不了别人的页面语言。
//
// 为什么站级设置不是档案级设置的特例：档案级的 theme/layout/language 存在
// profile.json 里随档案走（P-③），而这份文件在 data/ 根下、删档不动它、
// 换人不动它。两者的生命周期不同，所以是两个东西而不是同一份数据上的两个字段。
type Settings struct {
	// Language 是**站级的默认语言**：谁还没自己选过语言，就用这一个。
	//
	// 缺字段 = 中文（i18n.Default），于是升级到带多语言的版本不改变任何一个人的
	// 现有界面。零值 i18n.Lang("") 经 ParseLang 落回默认。
	Language i18n.Lang `json:"language"`
}

// SettingsPath 是站级设置的落盘位置。data/ 根下，与 profiles/ 平级——
// 删掉 profiles/<id> 不动它，重建一台机器时它跟着 data 目录走。
func (s *Store) SettingsPath() string { return filepath.Join(s.root, "settings.json") }

// Language 读站级的**默认语言**，**任何读不出来的情况都落回默认**。
//
// **它现在只有测试在用**（2026-10-20）：渲染层的 404 页与坏档页从前调它，
// 于是那两页绕过了整条链——它们既不认"有没有人设过"（这一半它塌掉了），
// 也从不看 Accept-Language，于是全新机器 + 英文浏览器上它们是全站唯一两页
// 中文（而 404 页那颗按钮又是英文的，混排）。那两页现在由 httpapi 把语言
// 传进来。
//
// **别拿它当判据**：判据是 DefaultLanguage（带"有没有人设过"那一位）与
// httpapi.machineLang（整条链）。这个塌掉的版本只适合"我只想知道盘上写了
// 什么"这种读法——测试要的就是这个。
//
// 刻意不返回 error：语言是纯显示层。一个显示配置读不出来就让整台机器每一页都
// 500，是把「装饰」抬到「生死开关」的高度。读不出来时界面显示中文——
// 一定能用、也一定是这个项目原本的样子，而用户看到的是中文而不是一屏错误页。
//
// 三种读不出来：文件不存在（首次装机）、读不了（权限）、内容是坏的。都落默认。
//
// **它现在只是"默认"**（2026-10-20）：每一份档案可以自己记一份语言
// （model.Profile.Language），它优先；没记的人用这里这一个。判据只有一处：
// i18n.ProfileLang。
//
// **刻意不做内存缓存**：这样管理员在管理页改完立刻生效，不必重启容器；缓存会
// 让「改了不生效」变成一个要靠重启才能绕开的怪问题，而这里省下的那点 IO 连
// profile.json 的一次读都不如——页面本来就要读它。
func (s *Store) Language() i18n.Lang {
	l, _ := s.DefaultLanguage()
	return l
}

// DefaultLanguage 读站级的默认语言，**第二个返回值答"有没有人设过"**。
//
// **为什么要分开**（2026-10-20）：语言的完整链条是
//
//	这一份自己记的 → 站级的默认 → **浏览器语言** → en
//
// 而链条的第三格只在"**还没人设过默认**"时才会被问到——那时"默认中文"那句
// 并不成立（谁都没说过要中文），人正在读的那一门语言（浏览器说的）才是更
// 像他的那一门。于是"文件不存在 / 读不了 / 里面是不认识的值"这三种都得
// 答"**没设过**"，而不是"默认是中文"。
//
// 读不出来一律不报错：语言是纯显示层，一个显示配置读不出来就让整台机器
// 每一页都 500，是把「装饰」抬到「生死开关」的高度。
func (s *Store) DefaultLanguage() (i18n.Lang, bool) {
	data, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		return i18n.Default, false
	}
	var st Settings
	if err := json.Unmarshal(data, &st); err != nil {
		return i18n.Default, false
	}
	// 文件里存了不认识的值（手改写错、旧版本留下）也算"没设过"：那不是
	// 一个人说"我要这个语言"，而是一份读不出来的东西。
	if !st.Language.Valid() {
		return i18n.Default, false
	}
	return st.Language, true
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

// ClearLanguage **取消**站级的默认语言，回到"没人设过"。
//
// **为什么要有它**（2026-10-20）：管理页那个下拉从前只有"中文"与"English"两档，
// 于是"**没人设过**"这一种状态**显示不出来**——下拉里预选的是浏览器说的那一门
// （那是链的第 3 格），管理员看到"English"被选中，就以为这台机器已经是英文的了；
// 而他只要按一下那颗按钮，就把"没人设过"悄悄变成了"显式设成英文"——从此整台
// 机器的默认被冻结在他的浏览器语言上。
//
// 写一份**空语言**的 settings.json（而不是删文件）：落盘走同一条原子写，
// 而读侧（DefaultLanguage）本来就把"不认识的值"与"文件不存在"一样当成"没设过"
// （DefaultLanguage 那一格）——所以这一格不需要新的读法。
func (s *Store) ClearLanguage() error {
	if err := s.WriteFileJSON(s.SettingsPath(), Settings{}); err != nil {
		return errors.Join(ErrInvalid, err)
	}
	return nil
}
