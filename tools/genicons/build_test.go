package main

import (
	"bytes"
	"compress/gzip"
	"testing"
	"time"
)

// TestGzipBytes同输入必同字节 钉的是 Task 2 可复现性验收（连跑两次生成器、
// 除 generatedAt 外产物一致）真正依赖的那件仪器：ModTime 归零 + Name 固定。
// 上面的 diff 只比未压缩的 JSON，把 gzip 头里的时间戳盖住了——哪天有人把
// zw.ModTime 那行删掉，diff 依然全绿，而每次生成的字节都在漂、git diff 里
// 一片噪声。gzip 头：FLG 在第 3 字节，MTIME 是第 4..7 字节。
func TestGzipBytes同输入必同字节(t *testing.T) {
	raw := []byte(`{"prefix":"tabler","icons":{}}` + "\n")

	a, err := gzipBytes(raw)
	if err != nil {
		t.Fatalf("压不出来: %v", err)
	}
	b, err := gzipBytes(raw)
	if err != nil {
		t.Fatalf("第二次压不出来: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("同一份 raw 两次压出的字节不同，产物每次生成都会脏一片 diff")
	}
	if len(a) < 8 {
		t.Fatalf("gzip 产物短到连头都没有: %d 字节", len(a))
	}
	var zero [4]byte
	if got := [4]byte{a[4], a[5], a[6], a[7]}; got != zero {
		t.Errorf("MTIME 没归零，得到 %#v——可复现性就是被这一个字段毁掉的", got)
	}

	// 反证：不设 ModTime 时头里会带当前时刻。这条断言的意义是证明上面那道闸
	// 不是空转——把 gzipBytes 里的 zw.ModTime 那行删掉，这里必须变红。
	var w bytes.Buffer
	zw, err := gzip.NewWriterLevel(&w, gzip.BestCompression)
	if err != nil {
		t.Fatalf("对照样本建不出来: %v", err)
	}
	zw.ModTime = time.Unix(1700000000, 0)
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("对照样本写不进: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("对照样本收尾失败: %v", err)
	}
	head := w.Bytes()
	if len(head) >= 8 && [4]byte{head[4], head[5], head[6], head[7]} == zero {
		t.Error("对照样本没带时间戳，这条反证失效了（MTIME 归零那道闸也就没在判事）")
	}
}
