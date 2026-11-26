# launchbay

> Mọi dấu trang bạn từng quên mình đã lưu, trên một trang duy nhất — tự lưu trữ, ngoại tuyến, không cần đăng ký.

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](../../LICENSE) [![365 Open Source Plan #042](https://img.shields.io/badge/365%20Open%20Source%20Plan-%23042-1f6feb)](https://github.com/rockbenben/365opensource)

[▶ Bắt đầu nhanh](#bắt-đầu-nhanh) · [GHCR](https://github.com/rockbenben/launchbay/pkgs/container/launchbay) · [docker-compose.yml](https://raw.githubusercontent.com/rockbenben/launchbay/refs/heads/main/docker-compose.yml)

[English](../../README.md) · [简体中文](../../README.zh.md) · [繁體中文](README.zh-Hant.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Español](README.es.md) · [Português](README.pt.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Italiano](README.it.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [हिन्दी](README.hi.md) · [বাংলা](README.bn.md) · [ไทย](README.th.md) · [Türkçe](README.tr.md) · **Tiếng Việt** · [Bahasa Indonesia](README.id.md)

![Trang hồ sơ launchbay: tìm kiếm trên thanh trên cùng, các nhóm ở thanh bên và thẻ ô](../../docs/home-1280.vi.webp)

Đưa tất cả các địa chỉ bạn không bao giờ nhớ vào một trang duy nhất: bảng quản trị NAS, trang cấu hình router, mạng nội bộ công ty, bảng điều khiển giám sát. Đặt nó làm trang chủ trình duyệt — **một cú nhấp chuột, không vòng quay tải trang.**

Chạy trên chính cỗ máy của bạn — NAS gia đình, Raspberry Pi, router phần mềm, hoặc một máy chủ trong mạng nội bộ — để các dấu trang của bạn không bao giờ chạm tới máy chủ của người khác.

## Phạm vi hỗ trợ

| Khía cạnh | Khả năng đáp ứng |
|---|---|
| Cách chạy | `docker compose up -d`, hoặc tệp nhị phân tĩnh (amd64 / arm64) |
| Nơi chạy | NAS gia đình, Raspberry Pi, router phần mềm, hoặc một máy trong mạng nội bộ |
| Đối tượng sử dụng | Một cá nhân, một gia đình hoặc một nhóm nhỏ — mỗi người một hồ sơ; quản trị viên quản lý tài khoản, không quản lý nội dung |
| Trình duyệt | Mọi trình duyệt hiện đại; khi tắt JavaScript, tất cả dấu trang vẫn hiển thị và bấm được bình thường |
| Cần cài đặt gì | Chỉ Docker: không thư viện bên thứ ba, không cơ sở dữ liệu, không cần Node khi xây dựng |
| Nơi lưu dữ liệu | Một thư mục thông thường `data/`; sao lưu chỉ đơn giản là sao chép thư mục đó |
| Ngôn ngữ giao diện | 18 ngôn ngữ giao diện được liệt kê ở trên. Mỗi hồ sơ có thể chọn ngôn ngữ riêng. |

## Tính năng chính

**Lưu và dọn dẹp**

- **Thả vào là lưu**: kéo một liên kết từ thanh dấu trang, thanh địa chỉ hoặc một tab khác vào một nhóm. Không có hộp thoại phiền phức — vị trí thả chính là đích đến.
- **Kéo để sắp xếp lại**: đổi vị trí trong nhóm, chuyển sang nhóm khác hoặc kéo vào tiêu đề nhóm để thêm vào cuối. Nhấn giữ hoặc dùng phím tắt để chọn nhiều mục, sau đó gắn thẻ, di chuyển hoặc xóa cùng một lúc.
- **Lưu từ bất kỳ trang nào**: kéo bookmarklet vào thanh dấu trang; một cú nhấp chuột sẽ lấy địa chỉ trang hiện tại, bấm thêm một lần nữa để xác nhận lưu.
- **Nhập từ trình duyệt của bạn**: tải lên tệp HTML được xuất từ Chrome / Edge / Firefox và nhận báo cáo đối chiếu từng mục — mục nào đã vào, mục nào không và lý do tại sao. Trang web có thể nhập ngay hoặc xem báo cáo trước; CLI sẽ ghi dữ liệu ngay trong cùng một lượt trừ khi bạn thêm `--dry-run`.
- **Mang theo toàn bộ hồ sơ**: một tệp duy nhất chứa các dấu trang **và** giao diện của trang (chủ đề, bố cục, chế độ xem, dòng phụ, tên trang, ngôn ngữ, chế độ địa chỉ). Mã PIN, danh tính hồ sơ và ảnh nền được cố ý loại trừ.

**Tìm kiếm**

- **Nhóm và tìm kiếm**: thanh trên cùng tìm kiếm tên, địa chỉ, ghi chú và thẻ nhãn; nhấn Enter là máy chủ trả về kết quả — hoàn toàn không cần JavaScript. Khi bật tập lệnh, nhấn `/` hoặc dùng bảng lệnh `⌘/Ctrl K`.
- **Nhóm nổi bật**: đưa các nhóm bạn dùng nhiều nhất thành dải thẻ lớn ở đầu trang.
- **Hơn 10.000 biểu tượng, hoạt động ngoại tuyến**: Tabler và Simple Icons được nén gzip tích hợp thẳng vào tệp nhị phân; favicon của một trang web được nạp một lần rồi phục vụ từ bộ nhớ đệm cục bộ.

**Ở nhà và khi đi xa**

- **Hai địa chỉ cho mỗi dấu trang**: đặt một địa chỉ chính và một địa chỉ WAN tùy chọn. Chế độ "Tự động" sẽ theo địa chỉ bạn dùng để mở launchbay; "Cài đặt → Địa chỉ dấu trang" cho phép ghi đè thủ công cho trình duyệt hiện tại. Tìm kiếm và sao chép sẽ dùng địa chỉ đang có hiệu lực, và các dấu trang không có địa chỉ WAN sẽ tiếp tục dùng địa chỉ chính. Phía sau reverse proxy, hãy nhớ giữ nguyên tiêu đề `Host` ban đầu.

**Bản thân trang điều khiển**

- **Một bảng điều khiển duy nhất**: thanh trên cùng → Cài đặt. Nơi đây tập trung giao diện và ngôn ngữ của trang, danh sách nhóm, sao lưu và lịch sử, cùng với **Mã PIN của tôi** — tự khóa hoặc mở khóa hồ sơ của chính bạn mà không cần quản trị viên.
- **Hoạt động khi mất mạng**: trang gần nhất được lưu trong bộ nhớ đệm trình duyệt; các dấu trang thêm vào khi ngoại tuyến — hoặc khi máy chủ tạm ngừng — sẽ xếp hàng chờ và tự động đồng bộ lại khi kết nối lại; nhấn **Gửi ngay** nếu sự cố nằm ở việc máy chủ chưa bật.
- **Tập lệnh và AI có chìa khóa riêng**: một REST API đi kèm mã thông báo (token) theo phạm vi hồ sơ dành cho tiện ích mở rộng, CLI và tự động hóa. Mã thông báo thao tác vượt ngoài hồ sơ được chỉ định sẽ nhận phản hồi 403.
- Khôi phục các phiên bản dấu trang trước đó trong phần cài đặt sao lưu và lịch sử.

## Bắt đầu nhanh

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
```

> [!IMPORTANT]
> Trước khi khởi động, đặt `LAUNCHBAY_ADMIN_TOKEN` riêng trong `docker-compose.yml`.

**Linux / NAS**

```bash
mkdir -p ./data
sudo chown 65534:65534 ./data
```

```bash
docker compose up -d
```

Không cần `.env`, không cần cài Go — hình ảnh được tải về trực tiếp.

Mở **http://<địa chỉ IP nội bộ của máy này>:7600** từ bất kỳ thiết bị nào trong mạng LAN — **không dùng `localhost`**, vì nó đại diện cho "chính thiết bị này" và sẽ cố mở trên chiếc điện thoại bạn đang gõ. Nếu ngồi trực tiếp trước máy chủ đó, `http://localhost:7600` hoạt động bình thường.

Hình ảnh dùng thẻ `:latest`; lệnh `up` đầu tiên sẽ tải bản phát hành mới nhất. Máy đã chạy trước đó vẫn giữ hình ảnh cục bộ, vì vậy hãy chạy `docker compose pull` trước khi muốn cập nhật.

Khi truy cập lần đầu, bạn sẽ vào trang thiết lập một lần: đặt tên cho hồ sơ đầu tiên và đặt mã PIN 4–6 chữ số. Không có thông tin đăng nhập mặc định nào, và trang này sẽ đóng vĩnh viễn ngay khi có bất kỳ hồ sơ nào tồn tại trên đĩa. Mở hồ sơ và lưu dấu trang đầu tiên.

**"Không có mã PIN" là một hộp kiểm riêng biệt, không phải là việc để trống ô nhập.** Việc để trống mã PIN sẽ bị từ chối; chỉ khi tích chọn **Không có mã PIN** thì mới tạo một hồ sơ mở cho tất cả mọi người trong mạng LAN. Điều này là có chủ ý — sai lầm tai hại là gõ thiếu số rồi lỡ tay bấm Enter, làm lộ dấu trang cá nhân cho cả nhà. Hồ sơ không có mã PIN sẽ không hiện ô nhập PIN trên trang lựa chọn. Bạn có thể thêm hoặc gỡ bỏ sau này trong mục **Mã PIN của tôi**; việc gỡ bỏ chỉ đòi hỏi mã PIN cũ khi hồ sơ đang có mã PIN.

Hồ sơ đầu tiên **không phải là quản trị viên**. Cỗ máy được quản lý bằng `LAUNCHBAY_ADMIN_TOKEN`: mở `/admin`, nhập khóa một lần và bạn sẽ nhận được cookie dành riêng cho quyền quản trị (thời hạn 30 ngày).

**Những người dùng sau không cần chờ quản trị viên.** Trang chọn hồ sơ có mục thu gọn **"Cái mới cho chính mình"**: mở ra, nhập tên và mã PIN là hồ sơ được tạo (`POST /new`; đường dẫn API máy là `POST /api/v1/profiles/new`). Không cần bất kỳ chứng chỉ nào, nên mọi thiết bị trong mạng LAN đều có thể tạo; thẻ hồ sơ sẽ xuất hiện và quản trị viên có thể xóa bất cứ lúc nào. Nó tạo ra một hồ sơ **trống rỗng**, gắn liền với mã PIN riêng và không cấp bất kỳ quyền truy cập nào vào dấu trang của người khác. Tên hỗ trợ tối đa 60 ký tự; nếu đặt trùng tên với người khác thì hai thẻ sẽ không thể phân biệt.

**Xóa hồ sơ là hành động duy nhất không thể đảo ngược**, vì vậy hệ thống sẽ hỏi lại khóa quản trị một lần nữa: trang xác nhận sẽ nói rõ có bao nhiêu dấu trang và bản chụp nhanh sẽ biến mất, và bạn phải gõ lại `LAUNCHBAY_ADMIN_TOKEN`. Đường dẫn máy (`DELETE /api/v1/profiles`) chỉ chấp nhận khóa này; chỉ có cookie quản trị viên sẽ nhận phản hồi 403. Các bản chụp nhanh và ảnh nền riêng tư sẽ đi theo hồ sơ, do đó hoàn nguyên không thể cứu được dữ liệu đã xóa.

Cũng trên trang đó có menu thả xuống **Ngôn ngữ giao diện**; hãy chọn ngôn ngữ bạn muốn khi thiết lập. Thao tác này đặt **mặc định cho toàn máy**; sau đó mỗi người có thể tự chọn ngôn ngữ riêng trong bảng điều khiển của mình, và thay đổi của quản trị viên chỉ ảnh hưởng đến những ai chưa từng tự chọn ngôn ngữ.

### Khóa quản trị và mã thông báo

| Biến môi trường | Bắt buộc? | Phạm vi quản lý |
|---|---|---|
| `LAUNCHBAY_ADMIN_TOKEN` | **Bắt buộc**, một khóa đơn | Chỉ quản lý tài khoản: tạo/xóa hồ sơ, đặt mã PIN tùy ý, vô hiệu hóa/kích hoạt, xem trạng thái và lịch sử tổng hợp giữa các hồ sơ. Mọi điểm cuối nội dung (`/api/v1/data`, `/batch`, `/export`, …) đều trả về 403 — không thể đọc dấu trang |
| `LAUNCHBAY_WRITE_TOKENS` | Tùy chọn, mặc định để trống | Mã thông báo ghi theo phạm vi hồ sơ (`name:secret:profile`, tức tên:khóa_bí_mật:hồ_sơ). Chỉ tiện ích mở rộng trình duyệt, CLI (`revert` / `import-bookmarks` / `backup` / `restore` / `profile fix-names`), và AI hoặc tập lệnh mới cần đến |

> **Tại sao mặc định lại để trống?** Trang dấu trang, trang quản trị, hoàn nguyên và nhập dữ liệu đều chạy dựa trên cookie phiên; hoàn toàn không cần mã thông báo. Ba công cụ cần đến mã thông báo đều do người dùng chủ động cài đặt. Việc cung cấp sẵn một giá trị mặc định sẽ vô tình cấp cho mọi cỗ máy một chiếc chìa khóa miễn phí có thể ghi vào hồ sơ `dad` (đây là ID hồ sơ phổ biến nhất).

Bản thân hồ sơ không có thuộc tính "là quản trị viên hay không". Trang quản trị hiển thị các tác vụ cấp tài khoản kèm theo số lượng mục, phiên bản, thời điểm sửa đổi gần nhất và tình trạng có đặt PIN hay không của từng hồ sơ.

> **ID hồ sơ được tạo từ tên và không nhất thiết phải là `dad`.** Các chữ cái và chữ số ASCII sẽ được chuyển thành slug (`Dad` → `dad`, `nas box` → `nas-box`). Tên có dấu hoặc ký tự phi ASCII không tạo được slug hợp lệ, nên sẽ tự động rơi vào chuỗi số `home` / `home-2`. Khi đó mã thông báo có phạm vi `…:dad` sẽ không khớp với bất kỳ hồ sơ nào và nhận lỗi 403 — hãy sửa phạm vi của mã thông báo thành ID thực tế (khuyên dùng), hoặc đặt tên hồ sơ đầu tiên bằng ký tự ASCII.

Cổng máy chủ lưu trữ mặc định là 7600. Trên một số máy Windows, Hyper-V có thể chiếm dụng một dải cổng (môi trường thử nghiệm từng ghi nhận dải 7532–7631); nếu lệnh `up` thất bại, hãy chọn một cổng trống ngoài dải này: `LAUNCHBAY_HOST_PORT=8901 docker compose up -d` (cổng bên trong container không đổi). Tên dự án và tên container được cố định là `launchbay`, do đó `docker logs launchbay` và `docker exec launchbay …` sẽ hoạt động bất kể bạn clone vào thư mục nào.

### Tiện ích mở rộng trình duyệt (tùy chọn)

`extension/` là tiện ích mở rộng dành cho Chrome / Edge (Manifest V3): nhấp chuột phải vào trang web đang xem để lưu vào hồ sơ chỉ định, hoặc lưu toàn bộ các tab trong cửa sổ cùng lúc. Nó không nằm sẵn trong hình ảnh; bạn cần tự cài đặt thủ công.

Tiện ích mở rộng không thể đọc cookie phiên nên cần một mã thông báo theo phạm vi hồ sơ. Hãy điền `LAUNCHBAY_WRITE_TOKENS`, khởi động lại dịch vụ, sau đó thực hiện:

1. Mở `chrome://extensions`, bật **Chế độ dành cho nhà phát triển** ở góc trên bên phải
2. Nhấp vào **Tải tiện ích đã giải nén**, chọn thư mục `extension/`
3. Mở trang tùy chọn của tiện ích và điền địa chỉ máy (không có dấu gạch chéo ở cuối), ID hồ sơ mặc định và chuỗi khóa bí mật từ mã thông báo
4. Nhấp **Kiểm tra máy này**

Nếu kiểm tra nhận kết quả 403, đó không phải lỗi mạng: điều đó có nghĩa là mã thông báo và ID hồ sơ không khớp nhau. Hãy sửa lại phạm vi trong `LAUNCHBAY_WRITE_TOKENS`, khởi động lại và kiểm tra lại.

Sau khi cài đặt: nhấp chuột phải vào bất kỳ trang nào → lưu; trang tùy chọn cũng có thể nhập toàn bộ cây dấu trang của trình duyệt, chỉ yêu cầu quyền đọc dấu trang trong lần sử dụng đầu tiên. Cả 3 kênh nhập (web, tiện ích, CLI) đều **chỉ nhập vào một hồ sơ trống** — nhập là hành động thay thế toàn bộ dấu trang của hồ sơ chứ không phải thêm nối tiếp. Chrome sẽ cảnh báo "Đọc và thay đổi dữ liệu của bạn trên tất cả các trang web" vì tiện ích cần `http://*/*` và `https://*/*` để phản hồi thao tác nhấp chuột phải; mã thông báo chỉ lưu trong `chrome.storage.local`, không bao giờ được đồng bộ lên đám mây. Trình duyệt Firefox hiện chưa được kiểm chứng.

### Quyền hạn thư mục dữ liệu

Dữ liệu mặc định nằm tại `./data` cạnh `docker-compose.yml`, và luôn là `/data` bên trong container. Để đổi vị trí, hãy sửa dòng `volumes`:

```yaml
    volumes:
      - /volume1/docker/launchbay:/data
```

Trên Windows, đường dẫn tuyệt đối như `D:` sẽ xung đột với dấu hai chấm; hãy dùng cú pháp đầy đủ:

```yaml
    volumes:
      - type: bind
        source: D:/launchbay-data
        target: /data
```

Container chạy dưới định danh **uid 65534**, trong khi thư mục trên máy chủ thường do root sở hữu — dẫn đến việc tạo hồ sơ, nhập dữ liệu và ghi dấu trang đều thất bại. Hãy phân quyền cho người dùng đó trên máy chủ:

```bash
sudo chown -R 65534:65534 ./data
```

Đừng dùng `--user root` để lách: điều đó cho phép dịch vụ có quyền ghi vào mọi tệp trên máy của bạn với tư cách root. Quá trình kiểm tra sức khỏe tạo và xóa một tệp thăm dò sau mỗi lần kiểm tra, vì vậy quyền hạn sai sẽ hiển thị là **unhealthy** trong `docker ps`, và nhật ký sẽ in ra chính lệnh chown ở trên. Trang thiết lập ban đầu cũng nêu rõ thông tin này (`uid 65534` trong container; người dùng hiện tại nếu chạy trực tiếp tệp nhị phân).

## Cách sử dụng

| Nhu cầu của bạn | Nơi thực hiện |
|---|---|
| Thay đổi giao diện trang, chế độ xem, nhóm, hình nền, chế độ địa chỉ hoặc cách mở dấu trang | 'Cài đặt' trên thanh trên cùng, hoặc `POST /prefs`, `/prefs/background`, `/prefs/group`, `/prefs/tabs` |
| Đổi mã PIN của chính mình | 'Mã PIN của tôi' trong bảng điều khiển cá nhân, hoặc `POST /prefs/pin` / `POST /api/v1/profiles/self/pin` |
| Tạo hoặc xóa hồ sơ, đặt mã PIN cho người khác, vô hiệu hóa ai đó | `/admin` (hoặc liên kết quản trị ở cuối trang lựa chọn). Xóa sẽ yêu cầu nhập lại khóa; CLI quản trị chỉ hỗ trợ `profile create` và `profile rebuild` |
| Tạo thêm một hồ sơ cho chính mình | Mục thu gọn 'Cái mới cho chính mình' trên trang lựa chọn, hoặc `POST /api/v1/profiles/new` (không cần chứng chỉ) |
| Dọn dẹp các mục có 'tên chính là địa chỉ' | `launchbay profile fix-names --profile <id>` (thêm `--dry-run` trước) |
| Để AI hoặc tập lệnh sửa đổi dấu trang | `POST /api/v1/batch` kèm `expectedRev` |
| Khôi phục một hồ sơ bị lỗi | `POST /api/v1/profiles/rebuild`, hoặc `launchbay profile rebuild` |
| Xem lịch sử tất cả các thay đổi | `GET /api/v1/history` (chỉ ghi hành động và đường dẫn, không ghi nội dung) |
| Sao lưu hoặc di chuyển toàn bộ hồ sơ | `GET /api/v1/backup` / `POST /api/v1/restore`, `launchbay backup` / `restore`, hoặc Sao lưu và lịch sử trong Cài đặt |

Các điểm cuối API máy khác: `POST /api/v1/usage` ghi nhận số lượt nhấp, `POST /api/v1/resolve` chuyển đổi URL thành tiêu đề và biểu tượng, và `GET /api/v1/status` là bản tóm tắt toàn bộ hồ sơ dành cho quản trị viên.

Bản sao lưu không bao gồm mã PIN, danh tính hồ sơ và ảnh nền. Bản sao lưu có chứa dấu trang chỉ có thể nhập vào một hồ sơ trống; bản sao lưu chỉ chứa cài đặt có thể áp dụng bất cứ lúc nào. Quá trình nhập kiểm tra phiên bản dưới khóa hồ sơ và trả về 409 `[rev-conflict]` kèm `currentRev` nếu xảy ra xung đột đồng thời. Cài đặt được ghi theo cách này không làm tăng số phiên bản và không xuất hiện trong mục Hoàn nguyên.

**Danh sách điểm cuối đầy đủ, phần thân yêu cầu và mã lỗi có trong [AGENTS.md](../../AGENTS.md)** — hợp đồng chuẩn xác dành cho AI, được kiểm thử khóa chặt với bảng định tuyến. Tài liệu này không lặp lại nội dung đó.

**Thói quen sử dụng CLI**: kết quả thành công xuất ra stdout, lỗi xuất ra stderr, và mọi thông báo lỗi đều trích dẫn mã lỗi của máy chủ (`[rev-conflict]`, `[ops-invalid]`). Lệnh `import-bookmarks` nhận đường dẫn tệp dưới dạng tham số vị trí và thực hiện ghi ngay trong cùng lần chạy in báo cáo (dùng `--dry-run` để dừng trước khi ghi); sao lưu và xuất dữ liệu truyền tải toàn bộ phản hồi dưới dạng luồng nên xử lý tốt các tệp trên 4 MiB. `--help` và `-h` trả về mã 0, lỗi cú pháp trả về 2, các lỗi khác trả về 1. `revert --to 0` bị từ chối như một lỗi cú pháp và in ra chính xác câu lệnh `curl` có thể dùng.

**Không cần JavaScript là mức sàn, không phải mức trần.** Tìm kiếm là một biểu mẫu HTML, tạo hồ sơ và đặt mã PIN là các lượt gửi form gốc, và khi tắt tập lệnh mọi dấu trang vẫn nhìn thấy và nhấp chuột được bình thường. Các liên kết bỏ qua tích hợp sẵn cho phép nhảy thẳng tới nội dung chính. Một lớp cải tiến lũy tiến phía trên bổ sung thêm khung thêm dấu trang, chỉnh sửa tại chỗ, kéo thả sắp xếp, bảng lệnh ⌘K và hỗ trợ ngoại tuyến; nếu lớp này gặp sự cố, tuyến đường không cần JS vẫn luôn sẵn sàng.

## Giới hạn

- **Chỉ dùng trong mạng LAN, không có HTTPS.** Nội dung dấu trang là tệp JSON thuần túy trên đĩa; hãy đặt một reverse proxy TLS phía trước nếu muốn mở ra ngoài Internet.
- **Tệp `profile.json` bị hỏng sẽ khóa hồ sơ đó.** Lệnh `revert` không thể khắc phục được; `POST /api/v1/profiles/rebuild` (hoặc `launchbay profile rebuild`) chỉ ghi lại duy nhất tệp đó và không chạm vào bất kỳ byte dữ liệu dấu trang, bản chụp nhanh hay lịch sử nào.
- **Thời gian làm nguội khi lấy biểu tượng**: một nguồn gốc (origin) gặp lỗi sẽ không được thử lại trong vòng 24 giờ. Cơ chế này ngăn chặn việc liên tục thăm dò các trang web bị lỗi.
- **Không có danh sách chia sẻ**: bạn không thể chia sẻ một dấu trang với gia đình hoặc xuất bản một trang công khai. Giữ dữ liệu độc lập cho mỗi cá nhân là mục đích chính của ứng dụng.
- **Chưa được kiểm chứng trên NAS thực tế.** Mọi vòng kiểm thử nghiệm thu đều chạy trên Windows + Docker Desktop; chỉ tiêu LCP mạng LAN, khởi động nguội trên NAS, tốc độ khung hình cuộn 1.000 dấu trang và độ trễ trúng bộ nhớ đệm ngoại tuyến vẫn chưa được đo đạc thực tế.
- **Không có tiện ích thời tiết hay đồng hồ.** Chúng nằm ngoài định hướng của trang này. Văn bản chân trang và nội dung dấu trang đều đi qua `POST /api/v1/batch`; việc quản lý tệp thuộc về máy chủ.
- **Các ngôn ngữ giao diện chỉ bao quát giao diện, không dịch văn bản dành cho lệnh.** Chi tiết lỗi `detail` của API, lý do xác thực trường và báo cáo nhập cho tập lệnh đọc vẫn giữ bằng tiếng Trung (báo cáo trên trang web sẽ theo ngôn ngữ trang). Phía gọi cần văn bản ổn định để so sánh; tiêu đề dấu trang, tên nhóm và URL là nội dung của người dùng nên cũng không được dịch.
- **Mã thông báo mặc định để trống**, vì vậy tiện ích mở rộng và CLI sẽ chưa hoạt động cho đến khi bạn cấu hình; ngược lại, khóa quản trị viên là bắt buộc phải có khi khởi động.

## Nguồn gốc hình ảnh Docker

compose chỉ kéo hình ảnh đã xuất bản; việc biên dịch là nhiệm vụ của CI. Khi gắn thẻ `v*` (hoặc kích hoạt quy trình thủ công), `.github/workflows/docker.yml` sẽ xây dựng cho amd64 và arm64 rồi đẩy lên Docker Hub và GHCR. Các thẻ phát hành là `<version>`, `<major>.<minor>` và `<major>`, cùng với `latest` và chuỗi sha rút gọn của commit; phiên bản hình ảnh chỉ lấy từ thẻ git.

Để bỏ qua bước clone mã nguồn, bạn có thể chạy hình ảnh trực tiếp:

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="khoa-quan-tri-cua-ban" \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

## Giới thiệu về 365 Open Source Plan

Dự án **#042** của [365 Open Source Plan](https://github.com/rockbenben/365opensource) — một người + AI, hơn 300 dự án mã nguồn mở trong một năm.

[Gửi ý tưởng của bạn →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
