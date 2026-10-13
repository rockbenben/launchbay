package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// 改动历史那一屏（2026-10-13）。
//
// 数据一直有：`history.jsonl` 从写通路落地那天起就逐行记着"谁、什么时候、
// 通过哪条路、动了哪一条"，`GET /api/v1/history` 也一直读得出来。缺的是**屏**——
// 于是"改动有历史可查"那句话（spec §2）在界面上是空的，而 §8.6 早就写了命令面板
// 里该有"看本档案历史"这一格，它没地方可去。
//
// 谁能看：**只有当前会话自己的那一份**，档案从会话推导、不接受申报。这与 /revert
// 是同一条纪律（2026-10-10 的管理员边界：管理员管账号、不碰内容），所以管理员
// 登着这一页看到的也只是自己那一份的动作线。要跨档案聚合是
// `GET /api/v1/history` 不带 profile 那一发，只给管理权。
//
// 页面上说的是**动作**，不是内容：历史里只有 `{op, path}`，没有 value——那是
// 刻意的（§4.6：一记内容，这行字就变成第二份书签数据，还会带出地址与备注）。
// 所以这一页答"谁动过我的书签"，答不了"改成了什么样子"。想回去，出口是
// 同一排上那枚「回退」。

const (
	// historyRowsOnPage 是一页印多少行动作。再多就不是"看最近改了什么"，
	// 而是把整条审计线搬进一次响应。
	historyRowsOnPage = 100
	// historyOpsOnRow 是一行印几条短句。一次批量改 30 条是常见形状（AI 与导入
	// 都这么写），全列出来那一行就比这一页其余部分都长；截断**要说出来**，
	// 少列不说才是骗人（与回退页的分组名、批量对话框的"另外还有 292 条"同纪律）。
	historyOpsOnRow = 6
	// historyNameReads 是一页最多为"这个 id 叫什么"去开几份快照。见 historyDict。
	historyNameReads = 12
)

// itemFieldKeys 与 groupFieldKeys 把 ops 的字段白名单翻成人话。
//
// **键的集合归 ops 管**（`itemFields`/`groupFields`），这里只负责"这个字段
// 在页面上叫什么"。写侧将来加一个可改字段而这里没跟上，那句短句会落成
// `history.op.item.field` 那一条带原字段名的兜底话——说得出动了哪一格，
// 只是没给它起名字。
var itemFieldKeys = map[string]string{
	"name": "history.op.item.name", "url": "history.op.item.url",
	"icon": "history.op.item.icon", "desc": "history.op.item.desc",
	"alias": "history.op.item.alias", "tags": "history.op.item.tags",
	"aiNote": "history.op.item.aiNote",
}

var groupFieldKeys = map[string]string{
	"title": "history.op.group.title", "layout": "history.op.group.layout",
	"accent": "history.op.group.accent",
}

// byChannels 是渠道那一格的话。`by` 是调用方自报的口径（§4.6），七个值。
//
// 认不出来的值**原样印**，不落"未知"：这台机器的前身写过 `sync-template`
// 那种现在已经不存在的渠道，把历史里真有的东西说成"未知"，等于在这一页上
// 撒谎说它没发生过。
var byChannels = map[string]string{
	"ai": "history.by.ai", "bookmarklet": "history.by.bookmarklet",
	"extension": "history.by.extension", "web": "history.by.web",
	"import": "history.by.import", "cli": "history.by.cli",
}

func (s *Server) channelLabel(by string) string {
	if key, ok := byChannels[by]; ok {
		return s.t(key)
	}
	return by
}

// handleHistoryPage 是 GET /history。
func (s *Server) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.selfProfileOnly(w, r)
	if !ok {
		return
	}
	// 档案整个没了（会话指着一个已删的档案）→ 回选择页，那里会重新列一遍。
	//
	// **坏档不挡**：书签读不动的时候这一页恰恰是要看的那一页——它告诉你
	// 最后一次是谁动的，而它一个字都不必读 bookmarks.json。与 /revert 同一处置。
	p, err := s.deps.Store.LoadProfile(id)
	if errors.Is(err, store.ErrNotFound) {
		redirectPicker(w)
		return
	}
	if err != nil && !errors.Is(err, store.ErrCorrupt) && !errors.Is(err, store.ErrInvalid) {
		writeInternalError(w, err)
		return
	}
	name := id
	if err == nil && strings.TrimSpace(p.Title) != "" {
		name = p.Title
	}

	entries, err := s.deps.Store.ReadHistory(id, 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	dict := s.historyDict(id)

	shown := entries
	if len(shown) > historyRowsOnPage {
		shown = shown[:historyRowsOnPage]
	}
	rows := make([]render.HistoryRow, 0, len(shown))
	for _, e := range shown {
		rows = append(rows, s.historyRow(e, dict))
	}

	view := render.HistoryView{
		Profile: id,
		Heading: s.t("history.heading", name),
		Explain: s.t("history.explain"),
		Rows:    rows,
	}
	// 截断结论只在真的截断时才有。**"一共记了 N 次"说的是这一份**，不是这台
	// 机器：审计线是全机器共用的一个文件，那句话说成机器会被读成"别人家的改动
	// 也混在这一页里"，而这一页要避免的正是那种暗示。
	if len(entries) > len(shown) {
		view.Foot = s.t("history.foot", historyRowsOnPage, len(entries))
	}
	page, err := s.deps.Render.HistoryPage(view)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// historyRow 把一个历史行烤成页面上的一行。
func (s *Server) historyRow(e store.HistoryEntry, dict *historyDict) render.HistoryRow {
	// 名字要在写短句**之前**补：一行的前几条可能引用了一个盘上已经不存在的条目，
	// 而那一版恰好是能认出它名字的那一版。
	dict.need(e.Rev - 1)
	row := render.HistoryRow{
		At:      e.At,
		Channel: s.channelLabel(e.By),
		Rev:     e.Rev,
		Count:   len(e.Ops),
	}
	// 「谁」与「哪条路」在这一行里各说一件事，但管理员那一族两句话会打架：
	// 机器那一面不带 X-LB-By 时 `by` 缺省记成 `web`（写侧的默认，不是这一行的故事），
	// 于是页面上出现「网页上 · 由管理员经手」这种自相矛盾的一句话。
	//
	// 判据按**经手人是谁**分，不按动作类型分：
	//   · `admin` 是这台机器上管理权的名字，不是某个人自取的名字——它比渠道
	//     那一格更有信息量，所以**换掉**渠道；
	//   · 档案作用域令牌那一格存的是**人自己起的令牌名**（`dp-dad`、`claude`……），
	//     它回答的是"家里那几把钥匙里的哪一把"，渠道那一格回答不了——所以两格**都留**。
	//   · 经手人就是这一份自己（本人会话）时什么都再多说：每一行都印一次
	//     「由 dad」等于把这页写成日志转储（§6.1：代管下线后两者是同一件事）。
	switch {
	case e.Actor == "" || e.Actor == e.Profile:
	case e.Actor == adminActor:
		row.Channel = s.t("history.actor.admin")
	default:
		row.Actor = s.t("history.actor", e.Actor)
	}
	for i, op := range e.Ops {
		if i == historyOpsOnRow {
			// 那句截断话说的是**没列出来的条数**，不是总数：读的人已经从
			// 上面看到了 6 条，再说"一共 30 条"要他自己减一次。
			row.More = s.t("history.ops.more", len(e.Ops)-historyOpsOnRow)
			break
		}
		row.Ops = append(row.Ops, s.opPhrase(op, dict))
	}
	row.Ops = s.collapseRepeats(row.Ops)
	return row
}

// collapseRepeats 把**相邻重复**的短句并成一句带次数。
//
// 为什么需要它：审计行只记 `{op, path}`，而"新增一个分组"那条的 path 就是
// `groups` ——组名在 value 里，**根本没被记下来**（§4.4）。于是导入或 AI 一次
// 建五组的那一行会印出五句一模一样的「新增了一个分组」，占掉六条里的五条，
// 却一次也没说出是哪几个组。合并成「新增了一个分组 ×5」既省地方，又把
// "发生了五次"这件本来就有用的事说出来。
//
// **只并相邻的**：不相邻的两句相同（改 A、改 B、改 A）并起来会让人以为
// 中间那件事没发生过——这一页是审计线，宁可啰嗦不可撒谎。
func (s *Server) collapseRepeats(ops []string) []string {
	out := make([]string, 0, len(ops))
	for i := 0; i < len(ops); {
		j := i + 1
		for j < len(ops) && ops[j] == ops[i] {
			j++
		}
		if j-i > 1 {
			out = append(out, s.t("history.op.times", ops[i], j-i))
		} else {
			out = append(out, ops[i])
		}
		i = j
	}
	return out
}

// opPhrase 把一条 `{op, path}` 说成一句人话。
//
// 三条出路，按"这句话能不能说准"排：
//  1. 账号类动作（`profile/pin`、`profile/disabled`、`profile/rebuild`、
//     `snapshots/<rev>`）不归书签文法管，各有各的一句话。
//  2. 书签树里的形状走 ops.ShapeOf——文法只有写侧那一份。
//  3. 都不像就**原样印路径**。这一支看着难看，但它是唯一不撒谎的兜底：
//     历史里真有这样一行，界面却说"没这东西"，等于把这一页变成假账。
func (s *Server) opPhrase(op store.HistoryOp, dict *historyDict) string {
	if op.Op == "revert" && strings.HasPrefix(op.Path, "snapshots/") {
		return s.t("history.op.revert", strings.TrimPrefix(op.Path, "snapshots/"))
	}
	switch op.Path {
	case "profile/pin":
		return s.t("history.op.pin")
	case "profile/disabled":
		return s.t("history.op.disabled")
	case "profile/rebuild":
		return s.t("history.op.rebuild")
	}

	switch sh := ops.ShapeOf(op.Path); sh.Kind {
	case ops.ShapeGroups:
		return s.t("history.op.group.add")
	case ops.ShapeGroup:
		return s.t("history.op.group.remove", dict.groupName(sh.GroupID))
	case ops.ShapeGroupField:
		name := dict.groupName(sh.GroupID)
		if key, ok := groupFieldKeys[sh.Field]; ok {
			return s.t(key, name)
		}
		return s.t("history.op.group.field", name, sh.Field)
	case ops.ShapeItems:
		return s.t("history.op.item.add", dict.groupName(sh.GroupID))
	case ops.ShapeItem:
		label := dict.itemName(sh.GroupID, sh.ItemID)
		if op.Op == "move" {
			return s.t("history.op.item.move", label)
		}
		return s.t("history.op.item.remove", label)
	case ops.ShapeItemField:
		label := dict.itemName(sh.GroupID, sh.ItemID)
		if key, ok := itemFieldKeys[sh.Field]; ok {
			return s.t(key, label)
		}
		return s.t("history.op.item.field", label, sh.Field)
	}
	return s.t("history.op.other", op.Path)
}

// historyDict 是"这个 id 叫什么"。
//
// 名字不在历史里：审计行只记 `{op, path}`，而 path 用的是 id（§4.4 的寻址键，
// 条目改过名也还认得它）。于是"谁动过我的书签"这句话里，那条书签得另外找名字。
//
// 找法分两层：
//  1. **先看盘上现在这一版**。绝大多数 id 都还在，一次解码就够；而"它现在叫什么"
//     也正是最该印出来的那个名字（一条书签改过名，人认的是它今天那个名字）。
//  2. **已经被删掉的**才去开那一改动之前的快照：`rev` 那一次的"之前"是
//     `snapshots/<rev-1>`——WriteSnapshot 在落盘之前拍，所以盘上是 rev 时，
//     最新那份快照标的还是 rev-1（/revert 那一页预选最新快照靠的是同一件事实）。
//     这一层按需开、最多开 historyNameReads 版：删掉的那条正是**最需要名字**的
//     那些（"AI 删了 3 条"而不给名字，等于把这一页最该说的话咽回去），
//     所以这一层不能省；它也不能无限开，一版书签在最坏的夹具上是几百 KB。
//
// 两层都认不出（快照已经被 50 份的窗口裁掉了）就印 id 本身：id 是屏幕上
// 本来就看得见的东西，而编一个名字是假话。
type historyDict struct {
	st     *store.Store
	id     string
	sep    string
	groups map[string]string
	items  map[string]string
	// opened 记"这一版开过了"，成功的与失败的都记：一次开不动的快照
	// 不该在同一页里被重试 12 次。
	opened map[int64]bool
	reads  int
}

func (s *Server) historyDict(id string) *historyDict {
	d := &historyDict{st: s.deps.Store, id: id, sep: s.t("history.sep"),
		groups: map[string]string{}, items: map[string]string{}, opened: map[int64]bool{}}
	// 坏档、读不动、不存在——都当"现在这一版没有可借的名字"，不拦这一页。
	if snap, err := s.deps.Store.Current(id); err == nil {
		d.fill(snap.B)
	}
	return d
}

func (d *historyDict) fill(b *model.Bookmarks) {
	if b == nil {
		return
	}
	for _, g := range b.Groups {
		if strings.TrimSpace(g.Title) != "" {
			d.groups[g.ID] = g.Title
		}
		for _, it := range g.Items {
			// 键带分组：两个分组各有一个 id 为 `test` 的条目是真能出现的
			// （导入把嵌套文件夹拍平时就会），只按条目 id 存会让它们共用一个名字。
			if strings.TrimSpace(it.Name) != "" {
				d.items[g.ID+"/"+it.ID] = it.Name
			}
		}
	}
}

// need 为"rev 之前那一版"补名字。rev <= 0 没有之前（账号类动作不产生书签版本）。
func (d *historyDict) need(rev int64) {
	if rev <= 0 || d.opened[rev] || d.reads >= historyNameReads {
		return
	}
	d.opened[rev] = true
	d.reads++
	raw, err := d.st.LoadSnapshot(d.id, rev)
	if err != nil {
		return
	}
	b, err := store.DecodeBookmarks(d.st.BookmarksPath(d.id), raw)
	if err != nil {
		return
	}
	d.fill(b)
}

func (d *historyDict) groupName(gid string) string {
	if n, ok := d.groups[gid]; ok {
		return n
	}
	return gid
}

// itemName 是「自建服务 › 群晖 DSM」那一句。分隔符走语言包（`history.sep`）
// 而不是在这里拼一个字符：它是屏幕上的字，而屏上的字只有一份、住在语言包里
// （AGENTS.md 那条纪律一）。今天两种语言用同一个 ` › `，将来要分开也是改语言包。
func (d *historyDict) itemName(gid, iid string) string {
	name := iid
	if n, ok := d.items[gid+"/"+iid]; ok {
		name = n
	}
	return d.groupName(gid) + d.sep + name
}
