package httpapi

import (
	_ "embed"
	"net/http"
)

// openAPIDoc 是手写的机器可读契约。为什么手写不生成：生成的文档只反映
// 代码"长什么样"，不会反映 spec 要求的行为（409 的形状、by 的枚举、
// 只写当前档案这类约定）；而骗 AI 的恰恰是行为。漂移由上面三条契约测试挡。
//
//go:embed openapi.json
var openAPIDoc []byte

// handleOpenAPI 直接吐内嵌字节，不解码再编码：文档里的键序也是给人/AI
// 读的一部分，重新编码会漂。
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(openAPIDoc)
}
