# launchbay

> Setiap bookmark yang sempat Anda lupakan, dalam satu halaman — self-hosted, offline, tanpa daftar.

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](../../LICENSE) [![365 Open Source Plan #042](https://img.shields.io/badge/365%20Open%20Source%20Plan-%23042-1f6feb)](https://github.com/rockbenben/365opensource) [![ci](https://img.shields.io/github/actions/workflow/status/rockbenben/launchbay/ci.yml?label=ci&branch=main)](https://github.com/rockbenben/launchbay/actions/workflows/ci.yml)

[▶ Mulai cepat](#mulai-cepat) · [⬇ Releases](https://github.com/rockbenben/launchbay/releases)

[English](../../README.md) · [简体中文](../../README.zh.md) · [繁體中文](README.zh-Hant.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Español](README.es.md) · [Português](README.pt.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Italiano](README.it.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [हिन्दी](README.hi.md) · [বাংলা](README.bn.md) · [ไทย](README.th.md) · [Türkçe](README.tr.md) · [Tiếng Việt](README.vi.md) · **Bahasa Indonesia**

![Halaman profil launchbay: pencarian bilah atas, grup bilah samping, dan kartu ubin](../../docs/home-1280.en.webp)

Kumpulkan semua alamat yang sulit Anda ingat ke dalam satu halaman: panel admin NAS, pengaturan router, intranet perusahaan, dasbor pemantauan. Jadikan beranda peramban Anda — **sekali klik, langsung sampai tanpa menunggu pemuatan.**

Berjalan di mesin Anda sendiri — NAS rumah, Raspberry Pi, soft router, atau sebuah server di intranet — sehingga bookmark Anda tidak pernah menyentuh server orang lain.

## Mengapa launchbay

Sebagian besar alat bookmark berbeda dalam empat hal: server siapa yang menyimpan data, bagaimana keluarga membaginya, apakah bisa offline, dan apakah bisa dirapikan dengan seret-lepas.

| Perbandingan | Bookmark bawaan peramban | Layanan bookmark cloud | launchbay |
|---|---|---|---|
| Lokasi data | Folder profil peramban, terikat akun | Server penyedia | **Mesin Anda sendiri** |
| Terpisah per orang | Lewat banyak profil peramban | Berdasarkan akun | **Masing-masing satu, terpisah di server** |
| Offline | Hanya lokal | Tidak berfungsi | **Cache + antrean offline** |
| Penataan seret-lepas | Struktur folder, bukan tata letak satu halaman | Sebagian besar tidak bisa | **Lepas untuk simpan, seret untuk susun ulang** |
| Siapa lagi yang bisa baca | Akun peramban Anda | Penyedia | **Tidak ada yang bisa mengakses lewat aplikasi** |

Tanpa akun, tanpa skrip pihak ketiga, tanpa telemetri. Satu-satunya permintaan ke luar adalah mengambil halaman yang Anda tambahkan (untuk membaca judulnya) dan favicon-nya; ikon yang gagal diambil akan diberi jeda (cooldown) selama 24 jam.

**Jika Anda menggunakannya sendirian, halaman bookmark bawaan peramban mungkin sudah cukup.** launchbay memberikan tiga nilai utama:

- **Membuka tanpa menunggu.** Halaman dirender di sisi server: bookmark dan ikonnya sudah berada di dalam HTML sebelum skrip apa pun berjalan. Targetnya adalah LCP tampilan pertama di LAN < 300ms; cold start pada perangkat NAS fisik belum diukur (lihat [Batasan](#batasan)).
- **Masing-masing satu, dan orang lain tidak bisa melihatnya.** Setiap profil dapat memiliki PIN 4–6 digit sendiri. Admin dapat membuat, menghapus, dan menyetel ulang PIN, tetapi setiap endpoint konten menolak kunci admin dengan 403 — **tidak ada jalur** untuk membaca bookmark Anda. Ini bukan kriptografi untuk melawan admin atau LAN (siapa pun di LAN dapat mengedit cookie untuk menyamar), melainkan cara server memisahkan profil secara tegas.
- **Kesalahan pengeditan dapat dibatalkan.** Setiap perubahan massal akan mengambil snapshot terlebih dahulu. Fitur 'Kembalikan' menampilkan berapa banyak bookmark dan grup yang ada di setiap versi lama, serta menyimpan versi saat ini sebelum pemulihan — sehingga kesalahan pengembalian pun dapat dikembalikan lagi. Riwayat hanya bertambah dan tidak pernah dihapus: setiap baris mencatat siapa yang mengubah profil ini, kapan, lewat jalur mana (halaman web, AI, ekstensi, CLI), dan entri mana yang disentuh — tanpa mencatat isi kontennya.

## Cakupan dukungan

| Aspek | Kesesuaian untuk Anda |
|---|---|
| Cara menjalankan | `docker compose up -d`, atau biner statis (amd64 / arm64) |
| Lokasi berjalan | NAS rumah, Raspberry Pi, soft router, atau satu mesin di intranet perusahaan |
| Pengguna | Sendiri, keluarga, atau tim kecil — masing-masing satu profil; admin mengelola akun, bukan konten |
| Peramban | Semua peramban modern; saat JavaScript dinonaktifkan, semua bookmark tetap terlihat dan dapat diklik |
| Kebutuhan instalasi | Hanya Docker: tanpa pustaka pihak ketiga, tanpa basis data, tanpa Node saat build |
| Lokasi data | Direktori biasa `data/`; pencadangan cukup dengan menyalin direktori tersebut |
| Bahasa antarmuka | 4 bahasa antarmuka (Tionghoa Sederhana, Tionghoa Tradisional, Inggris, Arab RTL); pilih bawaan mesin saat persiapan awal, lalu tiap profil dapat memilih bahasanya sendiri |

## Kemampuan sistem

**Menyimpan dan merapikan**

- **Lepas dan langsung tersimpan**: seret tautan dari bilah bookmark, bilah alamat, atau tab lain ke suatu grup. Tanpa kotak dialog — posisi pelepasan adalah tujuannya.
- **Seret untuk menyusun ulang**: ubah urutan di dalam grup, pindah antargrup, atau seret ke judul grup untuk menambahkan di akhir. Tekan lama atau gunakan pintasan untuk masuk ke pilihan ganda, lalu beri tag, pindahkan, atau hapus sekaligus.
- **Simpan dari halaman mana pun**: seret bookmarklet ke bilah bookmark Anda; satu klik mengambil alamat halaman yang sedang dibuka, satu klik lagi untuk menyimpan.
- **Impor dari peramban**: unggah file HTML hasil ekspor dari Chrome / Edge / Firefox dan dapatkan laporan per entri — apa yang masuk, apa yang tidak, dan alasannya. Halaman web bisa langsung mengimpor atau melihat laporan terlebih dahulu; CLI akan langsung menulis data kecuali Anda menambahkan parameter `--dry-run`.
- **Bawa seluruh profil**: satu file menampung bookmark **dan** tampilan halaman (tema, tata letak, mode tampilan, baris kedua, nama halaman, bahasa, mode alamat). PIN, identitas profil, dan gambar latar belakang sengaja tidak disertakan.

**Pencarian**

- **Grup dan pencarian**: bilah atas mencari nama, alamat, catatan, dan tag; tekan Enter dan server memberikan hasilnya — tanpa perlu JavaScript. Saat skrip aktif, tekan `/` atau gunakan palet `⌘/Ctrl K`.
- **Grup ditampilkan lebih besar**: sematkan grup yang paling sering Anda gunakan menjadi kartu besar di bagian atas halaman.
- **10.000+ ikon, tersedia offline**: Tabler dan Simple Icons dikompresi gzip ke dalam biner; favicon situs web diambil sekali lalu disajikan dari cache lokal.

**Di rumah dan di luar**

- **Dua alamat untuk setiap bookmark**: atur alamat utama dan alamat WAN opsional. Mode "Otomatis" mengikuti alamat yang Anda gunakan saat membuka launchbay; "Pengaturan → Alamat bookmark" dapat menimpanya secara manual untuk peramban saat ini. Pencarian dan penyalinan menggunakan alamat yang sedang aktif, dan bookmark tanpa alamat WAN akan tetap menggunakan alamat utama. Di balik reverse proxy, pastikan header `Host` asli dipertahankan.

**Halaman itu sendiri**

- **Satu panel pengaturan terpadu**: bilah atas → Pengaturan. Berisi tampilan dan bahasa halaman, daftar grup, cadangan & riwayat, serta **PIN saya** — kunci atau buka kunci profil Anda sendiri tanpa perlu menghubungi admin.
- **Tetap berfungsi saat offline**: halaman terakhir disimpan di cache peramban; bookmark yang ditambahkan saat offline — atau saat server mati — akan mengantre dan otomatis dikirim ulang saat peramban terhubung kembali; klik **Kirim sekarang** jika server baru saja menyala kembali.
- **Skrip dan AI memiliki kunci sendiri**: REST API disertai token dengan cakupan profil untuk ekstensi peramban, CLI, dan otomatisasi. Token yang mencoba mengakses di luar cakupan profilnya akan menerima respons 403.

## Mulai cepat

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
docker compose up -d
```

Tidak perlu file `.env`, tidak perlu instal Go — citra Docker langsung diunduh.

Buka **http://<alamat IP internal mesin ini>:7600** dari perangkat mana pun di LAN — **jangan gunakan `localhost`**, karena itu merujuk pada "perangkat ini" dan akan mencoba membuka ponsel yang sedang Anda ketik. Jika duduk langsung di depan mesin server tersebut, `http://localhost:7600` dapat digunakan.

Tag citra adalah `:latest`; perintah `up` pertama akan mengunduh rilis terbaru. Mesin yang sudah berjalan sebelumnya akan menyimpan citra lokal, jadi jalankan `docker compose pull` terlebih dahulu untuk memperbarui.

> **Ubah kunci admin bawaan terlebih dahulu.** File compose menyertakan nilai bawaan publik yaitu `launchbay123`. Siapa pun di LAN yang mengetahui nilai ini dapat membuat, menghapus profil, dan menyetel ulang PIN (mereka tetap tidak bisa membaca bookmark Anda). Masukkan kunci Anda sendiri ke `LAUNCHBAY_ADMIN_TOKEN` lalu jalankan kembali `docker compose up -d`:
>
> ```yaml
> LAUNCHBAY_ADMIN_TOKEN: "kunci-admin-anda-sendiri"
> ```

Saat pertama kali dibuka, Anda akan masuk ke halaman pengaturan awal satu kali: beri nama profil pertama dan atur PIN 4–6 digit. Tidak ada kredensial bawaan, dan halaman ini akan tertutup secara permanen begitu ada profil yang tersimpan di disk.

**"Tanpa PIN" adalah kotak centang terpisah, bukan membiarkan kolom PIN kosong.** Membiarkan PIN kosong akan ditolak; hanya dengan mencentang **Tanpa PIN** Anda akan membuat profil yang terbuka untuk semua orang di LAN. Ini disengaja — kesalahan fatal yang sering terjadi adalah kurang mengetik digit lalu menekan Enter, yang tanpa disadari memublikasikan bookmark pribadi ke seluruh rumah. Profil tanpa PIN tidak akan menampilkan kotak PIN pada halaman pemilih profil. Anda dapat menambahkan atau menghapusnya nanti di **PIN saya**; menghapus PIN hanya memerlukan PIN lama jika profil tersebut memang memiliki PIN sebelumnya.

Profil pertama **bukanlah admin**. Mesin ini dikelola menggunakan `LAUNCHBAY_ADMIN_TOKEN`: buka `/admin`, ketik kuncinya sekali, dan Anda akan menerima cookie khusus identitas admin (berlaku 30 hari).

**Orang yang bergabung belakangan tidak perlu menunggu admin.** Halaman pemilihan memiliki menu lipat **"Yang baru untuk dirimu sendiri"**: buka, masukkan nama dan PIN, maka profil akan dibuat (`POST /new`; rute mesinnya adalah `POST /api/v1/profiles/new`). Tidak memerlukan kredensial apa pun, sehingga perangkat mana pun di LAN dapat membuatnya; kartu profil akan muncul, dan admin dapat menghapusnya kapan saja. Ini membuat profil yang **kosong**, terikat pada PIN-nya sendiri, dan tidak memberikan izin akses ke bookmark orang lain. Nama mendukung hingga 60 karakter; jika menggunakan nama yang sama dengan orang lain, kedua kartu tidak dapat dibedakan.

**Menghapus profil adalah satu-satunya tindakan yang tidak dapat dibatalkan**, sehingga sistem akan meminta konfirmasi kunci mesin sekali lagi: halaman konfirmasi menjelaskan berapa banyak bookmark dan snapshot yang akan dihapus, dan Anda harus mengetik `LAUNCHBAY_ADMIN_TOKEN` sekali lagi. Rute mesin (`DELETE /api/v1/profiles`) hanya menerima kunci ini; hanya memiliki cookie admin akan menghasilkan respons 403. Snapshot dan gambar latar belakang pribadi akan ikut terhapus bersama profil, sehingga fitur pemulihan tidak dapat menyelamatkannya.

Halaman yang sama memiliki menu tarik-turun **Bahasa antarmuka**; pilih bahasa saat penyiapan awal. Ini menentukan **bawaan seluruh mesin**; setelah itu setiap orang dapat memilih bahasanya sendiri di panel masing-masing, dan perubahan oleh admin hanya memengaruhi pengguna yang belum memilih bahasa secara mandiri.

### Kunci admin dan token

| Variabel | Wajib? | Cakupan kontrol |
|---|---|---|
| `LAUNCHBAY_ADMIN_TOKEN` | **Wajib**, satu kunci | Hanya untuk manajemen akun: membuat/menghapus profil, menyetel PIN apa pun, menonaktifkan/mengaktifkan, melihat status dan riwayat lintas profil. Setiap endpoint konten (`/api/v1/data`, `/batch`, `/export`, …) merespons 403 — tidak dapat membaca bookmark |
| `LAUNCHBAY_WRITE_TOKENS` | Opsional, bawaan kosong | Token penulisan dengan cakupan profil (`name:secret:profile`, yaitu nama:rahasia:profil). Hanya diperlukan oleh ekstensi peramban, CLI (`revert` / `import-bookmarks` / `backup` / `restore` / `profile fix-names`), serta AI atau skrip |

> **Mengapa secara bawaan dibiarkan kosong?** Halaman bookmark, admin, pemulihan, dan impor semuanya berjalan menggunakan cookie sesi; token tidak diperlukan sama sekali. Tiga alat yang membutuhkan token adalah hal-hal yang sengaja dipasang oleh pengguna. Menyediakan nilai bawaan sama saja dengan memberikan kunci gratis ke semua mesin untuk memodifikasi profil `dad` (karena `dad` adalah ID profil yang paling umum).

Profil itu sendiri tidak memiliki atribut "apakah admin". Halaman admin menampilkan tindakan tingkat akun beserta jumlah item, versi, perubahan terakhir, dan status apakah profil tersebut **tanpa PIN**.

> **ID profil diturunkan dari namanya, dan tidak selalu `dad`.** Huruf dan angka ASCII diubah menjadi slug (`Dad` → `dad`, `nas box` → `nas-box`); nama non-ASCII tidak menghasilkan slug yang valid, sehingga otomatis dialihkan ke urutan `home` / `home-2`. Token dengan cakupan `…:dad` kemudian tidak akan cocok dengan profil mana pun dan menerima 403 — ubah cakupan token ke ID sebenarnya (disarankan), atau beri nama profil pertama dengan karakter ASCII.

Port host bawaan adalah 7600. Di beberapa mesin Windows, Hyper-V mungkin mencadangkan blok port tertentu (pada mesin pengujian tercatat rentang 7532–7631); jika perintah `up` gagal, pilih port bebas di luar rentang tersebut: `LAUNCHBAY_HOST_PORT=8901 docker compose up -d` (port di dalam container tidak berubah). Nama proyek dan kontainer ditetapkan sebagai `launchbay`, sehingga `docker logs launchbay` dan `docker exec launchbay …` akan selalu berfungsi tanpa memandang direktori clone.

### Ekstensi peramban (opsional)

`extension/` adalah ekstensi untuk Chrome / Edge (Manifest V3): klik kanan pada halaman yang sedang dibuka untuk menyimpannya ke profil tertentu, atau simpan semua tab dalam jendela sekaligus. Ekstensi ini tidak ada di dalam citra Docker; Anda perlu memasangnya sendiri secara manual.

Ekstensi tidak dapat membaca cookie sesi, sehingga memerlukan token dengan cakupan profil. Masukkan nilai ke `LAUNCHBAY_WRITE_TOKENS`, mulai ulang layanan, lalu ikuti langkah berikut:

1. Buka `chrome://extensions`, aktifkan **Mode pengembang** di kanan atas
2. Klik **Muat yang belum dibongkar**, lalu pilih direktori `extension/`
3. Buka halaman opsi ekstensi lalu isi alamat mesin (tanpa garis miring di akhir), ID profil bawaan, dan string rahasia dari token
4. Klik **Uji mesin ini**

Respons 403 dari pengujian bukan karena masalah jaringan: itu berarti token dan ID profil tidak cocok. Perbaiki cakupan di `LAUNCHBAY_WRITE_TOKENS`, mulai ulang layanan, lalu uji kembali.

Setelah terpasang: klik kanan pada halaman mana pun → simpan; halaman opsi juga dapat mengimpor seluruh pohon bookmark peramban, dan izin membaca bookmark hanya diminta pada penggunaan pertama. Ketiga jalur impor (web, ekstensi, CLI) **hanya dapat mengimpor ke profil yang kosong** — proses impor menggantikan seluruh bookmark profil alih-alih menambahkannya. Chrome akan memperingatkan "Membaca dan mengubah data Anda di semua situs" karena ekstensi memerlukan `http://*/*` dan `https://*/*` untuk merespons klik kanan; token hanya disimpan di `chrome.storage.local` dan tidak pernah disinkronkan. Firefox belum diverifikasi.

### Izin direktori data

Data secara bawaan disimpan di `./data` di samping `docker-compose.yml`, dan selalu menjadi `/data` di dalam kontainer. Untuk memindahkannya, ubah baris `volumes`:

```yaml
    volumes:
      - /volume1/docker/launchbay:/data
```

Pada Windows, jalur absolut seperti `D:` bertabrakan dengan titik dua; gunakan sintaks panjang:

```yaml
    volumes:
      - type: bind
        source: D:/launchbay-data
        target: /data
```

Kontainer berjalan dengan **uid 65534**, sedangkan direktori host biasanya dimiliki oleh root — sehingga pembuatan profil, impor, dan penulisan bookmark akan gagal. Berikan kepemilikan direktori kepada pengguna tersebut di host:

```bash
sudo chown -R 65534:65534 ./data
```

Jangan menyiasatinya dengan `--user root`: hal itu memungkinkan layanan menulis semua file di mesin Anda sebagai root. Pemeriksaan kesehatan membuat dan menghapus file probe pada setiap interval pemeriksaan, sehingga izin yang salah akan langsung ditandai sebagai **unhealthy** di `docker ps`, dan log akan mencetak perintah chown yang sama. Halaman persiapan awal juga menyebutkan hal yang sama (`uid 65534` di dalam kontainer; pengguna sistem Anda sendiri jika menjalankan biner secara langsung).

## Cara penggunaan

| Kebutuhan Anda | Tempat |
|---|---|
| Mengubah tampilan halaman, mode tampilan, grup, latar belakang, mode alamat, atau cara membuka bookmark | 'Pengaturan' di bilah atas, atau `POST /prefs`, `/prefs/background`, `/prefs/group`, `/prefs/tabs` |
| Mengubah PIN Anda sendiri | 'PIN saya' di panel pribadi, atau `POST /prefs/pin` / `POST /api/v1/profiles/self/pin` |
| Membuat atau menghapus profil, menyetel PIN orang lain, menonaktifkan pengguna | `/admin` (atau tautan admin di bagian bawah halaman pemilih). Menghapus memerlukan kunci lagi; CLI manajemen akun hanya mencakup `profile create` dan `profile rebuild` |
| Membuat satu profil lagi untuk diri Anda | Menu lipat 'Yang baru untuk dirimu sendiri' di halaman pemilih, atau `POST /api/v1/profiles/new` (tanpa kredensial) |
| Merapikan entri yang 'namanya adalah alamat URL' | `launchbay profile fix-names --profile <id>` (tambahkan `--dry-run` terlebih dahulu) |
| Membiarkan AI atau skrip mengubah bookmark | `POST /api/v1/batch` dengan menyertakan `expectedRev` |
| Memulihkan profil yang rusak | `POST /api/v1/profiles/rebuild`, atau `launchbay profile rebuild` |
| Melihat riwayat setiap perubahan | `GET /api/v1/history` (hanya mencatat tindakan dan jalur, bukan isi konten) |
| Mencadangkan atau memindahkan seluruh profil | `GET /api/v1/backup` / `POST /api/v1/restore`, `launchbay backup` / `restore`, atau Cadangan & riwayat di Pengaturan |

Rute mesin lainnya: `POST /api/v1/usage` mencatat klik, `POST /api/v1/resolve` mengubah URL menjadi judul dan ikon, dan `GET /api/v1/status` adalah ringkasan lintas profil untuk admin.

Pencadangan tidak menyertakan PIN, identitas profil, dan gambar latar belakang. Cadangan yang berisi bookmark hanya bisa dimasukkan ke dalam profil yang kosong; cadangan yang hanya berisi pengaturan dapat diterapkan kapan saja. Impor memeriksa revisi di bawah kunci profil dan merespons 409 `[rev-conflict]` bersama `currentRev` jika terjadi konflik simultan. Pengaturan yang ditulis dengan cara ini tidak menambah nomor revisi dan tidak muncul di fitur Kembalikan.

**Daftar lengkap endpoint, isi permintaan, dan kode kesalahan ada di [AGENTS.md](../../AGENTS.md)** — kontrak otoritatif untuk AI yang dikunci secara ketat dengan tabel rute melalui pengujian. Hal tersebut tidak diulang di sini.

**Kebiasaan penggunaan CLI**: hasil sukses dikirim ke stdout, kegagalan ke stderr, dan setiap kegagalan mengutip kode server (`[rev-conflict]`, `[ops-invalid]`). Perintah `import-bookmarks` menerima file sebagai argumen posisional dan melakukan penulisan dalam putaran yang sama dengan pencetakan laporan (`--dry-run` akan berhenti sebelum menulis); pencadangan dan ekspor mengalirkan seluruh respons secara streaming sehingga file di atas 4 MiB dapat ditangani dengan aman. `--help` dan `-h` keluar dengan kode 0, kesalahan sintaks penggunaan kode 2, kegagalan lainnya kode 1. Perintah `revert --to 0` ditolak sebagai kesalahan penggunaan dan akan mencetak perintah `curl` tepat yang dapat menjalankannya.

**Nol JavaScript adalah batas bawah, bukan batas atas.** Pencarian adalah formulir biasa, pembuatan profil dan pengaturan PIN adalah form POST asli, dan saat skrip dimatikan semua bookmark tetap terlihat dan dapat diklik. Tautan pintas bawaan memungkinkan langsung melompat ke konten utama. Di atasnya terdapat satu lapisan peningkatan progresif yang menambahkan bilah tambah, pengeditan di tempat, pengurutan seret, palet ⌘K, dan dukungan offline; jika lapisan ini bermasalah, jalur tanpa JS tetap selalu siap digunakan.

## Batasan

- **Hanya untuk jaringan LAN, tanpa HTTPS.** Konten bookmark berupa file JSON biasa di disk; pasang reverse proxy TLS di depannya sebelum mengeksposnya ke publik.
- **File `profile.json` yang rusak akan mengunci profil tersebut.** Perintah `revert` tidak dapat memperbaikinya; `POST /api/v1/profiles/rebuild` (atau `launchbay profile rebuild`) hanya menulis ulang file itu saja dan tidak menyentuh satu byte pun dari data bookmark, snapshot, atau riwayat.
- **Jeda pengambilan ikon**: domain asal yang gagal diambil tidak akan dicoba lagi selama 24 jam. Jeda 24 jam ini mencegah pemeriksaan berulang-ulang terhadap situs yang bermasalah.
- **Tidak ada daftar bersama**: Anda tidak dapat membagikan satu bookmark kepada keluarga atau memublikasikan halaman ke publik. Masing-masing menyimpan miliknya sendiri adalah tujuan utama aplikasi ini.
- **Belum divalidasi pada perangkat NAS fisik.** Semua putaran pengujian penerimaan dilakukan di Windows + Docker Desktop; target LCP di LAN, cold start di perangkat NAS asli, kecepatan frame scrolling 1.000 bookmark, dan latensi hit cache offline masih belum diukur.
- **Tidak ada widget cuaca atau jam.** Hal-hal tersebut di luar fungsi utama halaman ini. Teks footer dan konten bookmark dikelola melalui `POST /api/v1/batch`; file di disk dikelola oleh server.
- **Bahasa antarmuka hanya mencakup antarmuka, bukan teks untuk skrip.** Rincian `detail` kesalahan API, alasan validasi field, dan laporan impor yang dibaca oleh skrip tetap menggunakan bahasa Tionghoa (laporan di halaman web mengikuti bahasa halaman). Pemanggil membutuhkan teks yang stabil dan dapat dibandingkan; judul bookmark, nama grup, dan URL Anda sendiri adalah konten dan juga tidak diterjemahkan.
- **Token secara bawaan kosong**, sehingga ekstensi dan CLI dinonaktifkan sampai Anda menambahkannya; sebaliknya, kunci admin wajib ada untuk memulai layanan.

## Asal citra Docker

compose hanya mengunduh citra yang sudah dipublikasikan; pembangunan citra adalah tugas CI. Beri tag `v*` (atau jalankan alur kerja secara manual), maka `.github/workflows/docker.yml` akan membangun untuk amd64 dan arm64 lalu mengunggahnya ke Docker Hub dan GHCR. Tag rilis adalah `<version>`, `<major>.<minor>`, dan `<major>` (saat ini `0.4.0` / `0.4` / `0`), ditambah `latest` dan sha pendek commit; versi citra hanya berasal dari tag git.

Untuk melewati proses clone repositori, Anda dapat langsung menjalankan citra:

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="kunci-admin-anda-sendiri" \
  -e LAUNCHBAY_WRITE_TOKENS="ai-dad:kunci-rahasia-anda:dad" -e TZ=Asia/Shanghai \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

## Tentang 365 Open Source Plan

Proyek **#042** dari [365 Open Source Plan](https://github.com/rockbenben/365opensource) — satu orang + AI, 300+ proyek open-source dalam setahun.

[Ajukan ide Anda →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
