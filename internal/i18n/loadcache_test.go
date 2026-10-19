package i18n

import (
	"sync"
	"testing"
)

// 那张缓存**每个请求都会碰**（每渲染一页都要 Load 一次），而 HTTP 服务器是一
// 请求一 goroutine：两个请求同时首次加载同一种语言时就会**同时写**这张 map。
// Go 的运行时对并发写 map 是 **fatal error**——整个进程挂掉，**不是**可 recover
// 的 panic（症状读作「导航台自己重启了」，病因却只有两行）。所以那一格必须加锁
// （见 i18n.go 里 cachedMu 那一段）。
//
// 这条是**压**出来的：从空缓存开始，几十个 goroutine 同时 Load 两种语言。
//   - 没有 -race 时它靠运行时的 map 写检测兜（撞上就整个测试进程挂掉，症状与
//     产品里的崩法一样）；
//   - 有 -race 时它是确定的（CI 的 race 那一步，2026-10-19 加）。
//
// **为什么从空缓存开始**：写路径只在 miss 上发生，而别的测试可能已经把两种语言
// 都热了——那时的并发 Load 全是命中读，压根碰不到写那一侧，于是这条闸会在
// 旧代码上照样绿。
func TestLoad_并发加载不写坏那张缓存(t *testing.T) {
	// **从空缓存开始**：写路径只在 miss 上发生，而别的测试可能已经把两种语言
	// 都热了——那时的并发 Load 全是命中读，压根碰不到写那一侧，于是这条闸会在
	// 旧代码上照样绿。
	//
	// **直接清这张 map，不走锁**：这是测试专用的一次性重置，而它发生在所有
	// goroutine 起来之前（顺序由下面的 WaitGroup 保证），所以它不构成竞态——
	// 竞态检测器也不会报它（先写后读，中间有 happens-before）。生产代码里那张
	// map 只在锁下动，那条纪律不因为这一行破例。
	cached = map[Lang]*Catalog{}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := Zh
			if i%2 == 1 {
				l = En
			}
			c := Load(l)
			if c == nil {
				t.Error("Load 返回了 nil")
				return
			}
			if c.Lang() != l {
				t.Errorf("Load(%s) 给回来的 catalog 是 %s：拿错了语言", l, c.Lang())
			}
		}(i)
	}
	wg.Wait()
}
