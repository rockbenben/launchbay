package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/rockbenben/launchbay/internal/model"
)

// 背景图的两道闸，与图标缓存那一族同形（大小 + 后缀），但**都不收 svg**。
//
// 4 MiB 是这一格自己的上限：一张手机壁纸（1080×2400 的 JPEG）通常 1–3 MB，
// 而 nav 电视上解一张 12 MP 的照片要花的时间够人以为页面坏了。本案没有图像
// 处理依赖（tools/depguard 钉着零新依赖），所以那道"缩到合适大小"做不了，
// 只能在这里说清上限并让人自己先缩。
// MaxBackgroundBytes 是上传体积上限（4 MiB）。
//
// 为什么是它：**这一格没法替人缩图**。本案零新依赖（tools/depguard 钉着），于是
// 没有解码器、没有缩放、没有重编码——一张手机原图（4000×3000 那个尺寸）几 MB 是
// 常态，而它在 nav 电视上解起来够人以为页面坏了。4 MiB 是"绝大多数手机壁纸与
// 截图放得下、而一张桌面级原图放不下"的那条线；真的更大就该让人先自己缩一次，
// 而不是让这台机器替他做一件它做不了的事。
//
// 超了是**字段形状错**那一族：422 + fieldError{field:"file"}，而不是 413——表单这一格
// 的值不合法，与"这个字段拼错了"是同一件要告诉人的事，而 413 在浏览器表单通路上
// 只会端出一句裸状态码。
const MaxBackgroundBytes = 4 << 20

// BackgroundExts 是能落盘的后缀。与 model.ValidBackground 的正则、
// httpapi 的 backgroundContentTypes 是同一件事的三处表述，改一处要改三处
// （与 iconExts 那条"四处表述"同一个纪律，这里是三处：值域收窄了）。
var BackgroundExts = map[string]bool{"png": true, "jpg": true, "webp": true, "gif": true}

// BackgroundExtFor 把嗅探出来的 media type 映射成落盘后缀。
//
// **判据是嗅探出来的类型，不是浏览器给的 Content-Type**（与 icons.Fetcher.Image
// 同一条纪律）：后者是第三方声明的身份。一张叫 .png 的 jpeg 就该按 jpeg 存，
// 而端出的 Content-Type 与字节对不上时浏览器会拒绝渲染它（nosniff）。
// 认不出来的类型返回空串，由调用方据此说人话。
func BackgroundExtFor(mediaType string) string {
	switch mediaType {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	}
	return ""
}

// SaveBackground 存一份背景并回它的名字（`<16hex>.<ext>`）。
//
// 内容哈希命名：换一张图必然换一个 URL，浏览器那份 immutable 缓存立刻失效，
// 不会把上一张当这一张端出来（而"按档案 + 时间戳"命名做不到这件事）。
//
// 写完**不删旧的那一份**：删它要回答"还有谁在用"，而这个问题在按档案分目录
// 之后答案永远是"没人"——所以删除是 RemoveBackground 那条显式动作的事，
// 这里只管写（换背景是高频动作，删旧文件是低频且有风险的那半边）。
func (s *Store) SaveBackground(id string, data []byte, ext string) (string, error) {
	if !model.ValidSlug(id) {
		return "", fmt.Errorf("%w: 档案 id %q 不是 ASCII slug", ErrInvalid, id)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: 背景图是零字节", ErrInvalid)
	}
	if len(data) > MaxBackgroundBytes {
		return "", fmt.Errorf("%w: 背景图 %d 字节，超过 %d 上限", ErrInvalid, len(data), MaxBackgroundBytes)
	}
	if !BackgroundExts[ext] {
		return "", fmt.Errorf("%w: 背景图后缀 %q 不在表内", ErrInvalid, ext)
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:])[:16] + "." + ext
	if err := os.MkdirAll(s.BackgroundDir(id), 0o755); err != nil {
		return "", fmt.Errorf("建背景目录: %w", err)
	}
	if err := s.WriteFileAtomic(s.BackgroundPath(id, name), data); err != nil {
		return "", err
	}
	return name, nil
}

// RemoveBackground 删一份背景。**读得到才删**：profile.json 指着的那一份若已经
// 不在盘上（换过机器、手动清过缓存），这里回 not found 而不是一个假的成功——
// 那条路径上的"删掉了"必须是真的。
func (s *Store) RemoveBackground(id, name string) error {
	if !model.ValidSlug(id) || !model.ValidBackground(name) || name == "" {
		return fmt.Errorf("%w: 背景 id/名字形状非法", ErrInvalid)
	}
	if err := os.Remove(s.BackgroundPath(id, name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: 这张背景不在盘上", ErrNotFound)
		}
		return fmt.Errorf("删背景: %w", err)
	}
	return nil
}

// ReadBackground 取一份背景的字节。**形状非法与盘上没有都给 not found**：
// 这一条不是诊断通路，回显"为什么没有"等于告诉探测者他猜对了形状
// （与 handleIconFile 那句 404 共用一句话是同一处置）。
func (s *Store) ReadBackground(id, name string) ([]byte, error) {
	if !model.ValidSlug(id) || !model.ValidBackground(name) {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(s.BackgroundPath(id, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err // 真 I/O 故障：让上层回 500，不要伪装成 404
	}
	return data, nil
}
