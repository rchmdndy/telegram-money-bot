# PRD — Telegram Money Bot

Bot Telegram untuk mencatat pengeluaran & pemasukan harian dan merekapnya per **periode penggajian** yang bisa diatur sendiri.

- **Versi dokumen:** 1.0
- **Tanggal:** 2026-09-27
- **Status:** draft untuk direview
- **Stack:** Go 1.26 (single binary) + SQLite
- **Repo:** `/home/dandy/Documents/www/telegram_money_bot`

---

## 1. Ringkasan

Pengguna mencatat transaksi lewat chat Telegram. Bot menyimpan setiap transaksi, mengelompokkannya ke kategori, dan bisa merekap total per kategori untuk rentang tanggal apa pun. Karena pengguna sedang internship dengan jadwal gaji tetap, bot mendukung **periode rekap** dengan tanggal awal & akhir yang bisa dikonfigurasi (contoh: 21 → 20 bulan berikutnya) sehingga laporan otomatis mengikuti siklus gaji, bukan bulan kalender.

Bot juga mengirim **ringkasan harian** pada jam yang bisa diatur pengguna (default 19:00): kalau hari itu sudah ada catatan, isinya ringkasan pengeluaran/pemasukan + posisi periode; kalau belum ada, isinya pengingat untuk mencatat. Reminder bisa dimatikan (default nyala).

Bot juga punya **persona** yang bisa dipilih pengguna (penjaga harta karun, cowok posesif, softboy manja, atau netral) — katalognya hardcoded di dalam binary, pengguna hanya memilih (§4.12).

## 2. Tujuan & Non-Tujuan

### 2.1 Tujuan

1. Input transaksi cepat dari HP tanpa keluar dari Telegram.
2. Rekap akurat per kategori untuk rentang tanggal bebas, plus periode gaji berulang.
3. Menghitung sisa uang per periode (pemasukan − pengeluaran).
4. Mengirim ringkasan harian setiap hari pada jam yang bisa diatur — sekaligus pengingat kalau hari itu belum ada catatan.
5. Data milik pengguna sendiri: bisa diekspor CSV dan disimpan lokal.
6. Bot punya **persona** yang bisa dipilih pengguna (§4.12) supaya mencatat uang terasa seperti berinteraksi dengan karakter, bukan menekan tombol di form.
7. **Quick input** satu baris (`o, 47k, makan, makan malam`) untuk mencatat tanpa menekan tombol berurutan (§4.13).

### 2.2 Non-Tujuan (v1)

- Tidak ada Mini App / web UI (chat-only; lihat §4.3).
- Tidak ada parsing pesan bebas / LLM. Input cepat memakai **format ketat** 4 field (§4.13); kalimat bebas di luar format itu tidak ditafsirkan.
- Tidak ada anggaran (budget) per kategori atau peringatan over-budget.
- Tidak ada multi-currency. Hanya IDR.
- Tidak ada transaksi berulang otomatis (recurring).
- Tidak ada lampiran foto struk.
- Tidak ada split bill / utang-piutang antar orang.
- Tidak ada webhook. Bot memakai long polling (butuh proses hidup; tidak butuh HTTPS).
- Tidak ada persona buatan pengguna. Katalog persona hardcoded; user hanya memilih (§4.12).

## 3. Pengguna

**Multi-user sejak awal.** Setiap Telegram user yang mengirim `/start` otomatis terdaftar: satu baris di `users`, kategori default di-seed, dan `settings` default dibuat. Tidak ada `TELEGRAM_OWNER_ID`, tidak ada approval manual.

Isolasi ditegakkan di lapisan query: setiap statement menyertakan `WHERE user_id = ?` yang diambil dari `update.Message.From.ID`. Tidak ada kueri lintas user dan tidak ada perintah admin. Test integrasi wajib membuktikan user A tidak bisa membaca, mengubah, atau menghapus data user B lewat `/hari`, `/terakhir`, `/rekap`, dan `/export`.

## 4. Ruang Lingkup Fungsional

### 4.1 Entitas data

| Entitas | Isi |
|---|---|
| **Transaction** | tanggal, tipe (expense/income), nominal (IDR, integer), kategori, catatan opsional, waktu dibuat & diubah |
| **Category** | nama, tipe (expense/income), status aktif, urutan tampil |
| **Period** | nama, tanggal awal (hari 2–28), tanggal akhir (hari 1–27, wajib = awal − 1), tanggal mulai berlaku |
| **Settings** | jam reminder, status reminder on/off, tanggal reminder terakhir terkirim, persona aktif |

### 4.2 Input transaksi

Semua input lewat **reply keyboard** (keyboard bawah yang menggantikan keyboard HP). Alur:

```
[➕ Pengeluaran] [➕ Pemasukan]
[📊 Rekap] [⚙️ Pengaturan]
```

`📊 Rekap` = `/rekap`, `⚙️ Pengaturan` = `/settings`. Tombol reply keyboard hanya pintasan — semua fungsi tetap bisa dipanggil lewat perintah teks.

**Alur tambah transaksi:**

1. Tekan `➕ Pengeluaran` (atau `➕ Pemasukan`).
2. Bot kirim inline keyboard tanggal: `[Hari ini] [Kemarin] [Pilih tanggal]`.
   - `Pilih tanggal` → bot minta input `DD/MM` atau `DD-MM-YYYY` lewat kunci `tx.prompt.date` (§4.12). Format tidak valid → bot tolak dengan pesan yang menyebut format yang diterima verbatim, state tidak berubah. `DD/MM` diartikan tahun berjalan; kalau tanggalnya jatuh di masa depan, dipakai tahun sebelumnya (pencatatan selalu untuk pengeluaran yang sudah terjadi). `DD-MM-YYYY` selalu eksplisit.
3. Bot kirim inline keyboard kategori (dari DB, hanya kategori aktif dengan tipe sesuai).
   - `[⬅️ Kembali]` untuk membatalkan.
4. Bot minta nominal lewat kunci `tx.prompt.amount` (§4.12) — contoh `netral`: "Ketik nominal (contoh: `17000`, `17k`, `17.5k`, `13,5k`)".
5. Bot tampilkan **konfirmasi**:

   ```
   Tambah pengeluaran?
   Tanggal  : 21 Sep 2026 (Senin)
   Kategori : Makan
   Nominal  : Rp 17.000
   Catatan  : -

   [✅ Simpan] [✏️ Catatan] [❌ Batal]
   ```

   Baris pertama konfirmasi memakai kunci `tx.confirm.header` (§4.12) — contoh `netral`: `Tambah pengeluaran?`. Blok di atas adalah render `netral`; label field (`Tanggal`, `Kategori`, `Nominal`, `Catatan`) dan nilainya tidak pernah diubah persona.
6. `✅ Simpan` → INSERT, balas kunci `tx.saved` (§4.12). Contoh pada persona `netral`: `Tersimpan. Total Makan hari ini: Rp 17.000`.
   `✏️ Catatan` → minta teks catatan lewat kunci `tx.prompt.note` (§4.12), maks 200 char, kembali ke langkah 5.
   `❌ Batal` → hapus state, balas kunci `tx.cancelled` (§4.12) — contoh `netral`: `Dibatalkan.`

**Aturan nominal:**
- Menerima `17000`, `17.000`, `17k`, `17K`, `17,5k`, `17.5k`, `13,5k`.
- **Suffix `k`/`K` = ×1000.** Kalau suffix ada, satu `.` atau `,` dianggap pemisah desimal: `17.5k` = 17500, `13,5k` = 13500, `17,25k` = 17250. Lebih dari satu pemisah → tolak.
- **Tanpa suffix**, `.` = pemisah ribuan dan `,` = pemisah desimal (konvensi Indonesia): `17.000` = 17000, `1.234.567` = 1234567. Hasil non-integer ditolak (`17,5` → tolak, karena tidak ada sen).
- Hasil harus bilangan bulat. Spasi diabaikan. Karakter lain → tolak.
- Batas: `1` ≤ nominal ≤ `1.000.000.000` (1 miliar). Di luar itu → tolak.
- Disimpan sebagai **integer rupiah**. Tidak ada sen.

**State machine:** satu percakapan aktif per user. `/batal` kapan pun membatalkan. State disimpan di DB (`conversations`), bukan di memori, agar restart bot tidak menghilangkan percakapan setengah jalan.

### 4.3 Keputusan: chat-only, tanpa Mini App

Dipertimbangkan dan **ditolak untuk v1**: Telegram Mini App (form HTML5 di dalam Telegram). Alasannya:

- Mini App butuh URL **HTTPS** publik. Bot long-polling tidak butuh hosting sama sekali; menambah Mini App berarti menambah Cloudflare Tunnel / VPS / domain ke stack yang seharusnya satu binary.
- Butuh verifikasi `initData` (HMAC-SHA256 dengan secret `HMAC_SHA256("WebAppData", bot_token)`) di server — jalur auth tambahan yang harus diuji.
- Semua kebutuhan di PRD ini (pilih tanggal, pilih kategori, isi nominal, lihat rekap, export CSV) tercakup oleh reply keyboard + inline button.

Direvisit kalau nanti butuh grafik atau input massal beberapa transaksi sekaligus.

### 4.4 Kategori

Kategori disimpan di DB dan **bisa dikelola lewat bot**. Seed awal:

**Pengeluaran:** Makan, Transport, Rumah Tangga, Kesehatan, Hiburan, Lainnya
**Pemasukan:** Gaji, Lain-lain

Perintah:
- `/kategori` — daftar kategori + inline button `[➕ Tambah] [✏️ Ganti nama] [🗑 Nonaktifkan]`.
- **Nonaktifkan, bukan hapus.** Kategori yang sudah dipakai transaksi tidak boleh dihapus (integritas rekap historis). Kategori nonaktif tidak muncul di keyboard input tetapi tetap tampil di rekap lama.
- Nama kategori: 1–32 karakter, unik per tipe (case-insensitive).
- Batas praktis **20 kategori aktif per tipe per user** agar keyboard tidak jadi dinding teks. Di luar itu → tolak dengan pesan (alasan dan batas tetap verbatim, §4.12).
- Balasan aksi memakai kunci persona (§4.12): `category.saved` (tambah), `category.renamed` (ganti nama), `category.deactivated` (nonaktifkan).

### 4.5 Periode rekap

**Definisi:** periode = rentang `[start_day bulan M, end_day bulan M+1]`. Contoh `start=21, end=20` → 21 Sep 2026 s/d 20 Okt 2026.

Aturan:
- `start_day` 2–28, `end_day` = `start_day - 1` sehingga `end_day` bernilai 1–27. Batas 28 mencegah periode menunjuk tanggal yang tidak ada di Februari (mis. 30 Feb) — tidak ada logika akhir-bulan khusus.
- `end_day` **harus** = `start_day - 1`, dicek di kode dan lewat `CHECK` di DB. Kombinasi lain ditolak. Kalau kamu mau periode mengikuti bulan kalender (1 → akhir bulan), gunakan `start_day=2, end_day=1`; rentangnya bergeser satu hari tetapi tidak butuh logika panjang-bulan.
- **Periode default:** `start_day=21, end_day=20`, nama "Gaji".
- **Beberapa periode berlapis waktu:** tabel `periods` punya `effective_from` (tanggal). Periode aktif untuk tanggal `D` = periode dengan `effective_from <= D` terbesar. Kalau jadwal gaji berubah (mis. jadi tanggal 25), tambahkan periode baru dengan `effective_from` = tanggal perubahan; rekap tanggal lama tetap memakai periode lama.
- `UNIQUE (user_id, effective_from)` menolak dua periode dengan tanggal mulai berlaku sama. Periode tidak bisa tumpang tindih karena resolusi selalu memilih `effective_from` terbesar yang ≤ tanggal.

Perintah:
- `/periode` — tampilkan periode aktif + tanggal berjalan, kunci `period.status` (§4.12) — contoh render `netral`: `Periode aktif: Gaji, 21 Sep 2026 – 20 Okt 2026 (hari ke-7 dari 30)`.
- `/periode set <start_day> [nama]` — buat periode baru mulai berlaku hari ini. `end_day` selalu `start_day - 1`, tidak diisi manual. Contoh: `/periode set 25` → 25 Sep – 24 Okt.
- `/periode list` — riwayat periode.
- Balasan setelah `/periode set` memakai kunci `period.saved` (§4.12).

### 4.6 Rekap

Perintah: `/rekap [periode|hari|kemarin|minggu|bulan|YYYY-MM-DD..YYYY-MM-DD]`

Arti argumen:

| Argumen | Rentang |
|---|---|
| (kosong) atau `periode` | Periode aktif saat ini (§4.5) |
| `hari` | Hari ini (WIB) |
| `kemarin` | Kemarin (WIB) |
| `minggu` | Senin–Minggu minggu ini (minggu mulai Senin) |
| `bulan` | Bulan kalender berjalan (tanggal 1 sampai akhir bulan) |
| `YYYY-MM-DD..YYYY-MM-DD` | Rentang kustom, inklusif kedua ujung |

Contoh output `/rekap` (periode aktif 21 Sep – 20 Okt):

```
📊 Rekap Gaji
21 Sep 2026 – 20 Okt 2026 (30 hari)

PENGELUARAN — Rp 414.700
  Makan          Rp 366.500  (88,4%)  7 transaksi
  Transport      Rp  26.000  ( 6,3%)  1 transaksi
  Rumah Tangga   Rp  22.200  ( 5,4%)  3 transaksi

PEMASUKAN — Rp 0
  (belum ada)

Belum ada pemasukan dicatat di rentang ini.
Rata-rata harian — Rp 13.823

Transaksi terakhir: 26 Sep — Spons, es krim Rp 6.500
```
→ header kunci `rekap.header`, baris penutup kunci `rekap.no_income` (§4.12); teks di atas adalah render persona `netral`.

Aturan:
- Nominal diformat `Rp 1.234.567` (pemisah ribuan titik).
- Persentase dibulatkan 1 desimal; karena pembulatan, jumlah persentase bisa 99,9% atau 100,1% — bukan bug.
- Kategori bernilai 0 tidak ditampilkan.
- Urut kategori berdasarkan nominal menurun.
- Kalau rentang **tidak punya pemasukan sama sekali**, baris `SALDO` (pemasukan − pengeluaran) tidak ditampilkan; sebagai gantinya muncul satu baris kunci `rekap.no_income` (§4.12) — `netral`: `Belum ada pemasukan dicatat di rentang ini.` Aturan yang sama seperti reminder (§4.8). Saldo negatif hanya ditampilkan kalau memang ada pemasukan yang tercatat dan pengeluaran melebihinya.
- Kalau periode tidak punya transaksi: tampilkan header + kunci `rekap.empty` (§4.12) — `netral`: `Belum ada transaksi di periode ini.`
- Rentang tanggal kustom: `/rekap 2026-09-01..2026-09-15` (inklusif kedua ujung).
- **Batas panjang pesan.** Telegram menolak `sendMessage` di atas 4096 karakter. Render rekap memenggal per kategori: kirim sampai mendekati 4096, sisanya sebagai pesan lanjutan dengan header `rekap.header` diulang + penanda `(lanjutan)`. Penggalan tidak boleh memotong satu baris kategori. `sendDocument` (§4.7) tidak terkena batas ini.

### 4.7 Export CSV

`/export [rentang]` — default periode aktif. Bot mengirim **dokumen CSV** lewat `sendDocument`.

Kolom: `tanggal,tipe,kategori,catatan,nominal`

```csv
tanggal,tipe,kategori,catatan,nominal
2026-09-21,expense,Makan,maksi,17000
2026-09-21,expense,Makan,maklam,15000
2026-09-22,income,Gaji,gaji september,3000000
```

Aturan:
- Nama file: `rekap-YYYYMMDD-YYYYMMDD.csv`.
- Encoding UTF-8 tanpa BOM. Pemisah koma. Field yang mengandung `,`, `"`, atau newline dibungkus tanda kutip ganda (RFC 4180).
- Catatan user di-escape sebagai data, bukan formula. Field yang diawali `=`, `+`, `-`, `@` diberi prefix `'` untuk mencegah CSV injection saat dibuka di spreadsheet.
- Rentang tanpa transaksi → kirim CSV header-only, bukan error.

### 4.8 Reminder harian

- Default **nyala**, jam **19:00**.
- Perintah: `/reminder on`, `/reminder off`, `/reminder 21:30`, `/reminder` (tampilkan status).
- Zona waktu dari env `TZ` (default `Asia/Jakarta`, WIB, UTC+7). Semua tanggal dan jam yang dilihat pengguna memakai zona ini.
- **Selalu dikirim setiap hari** (kalau reminder nyala), apa pun isi hari itu. Kalimat pembungkus mengikuti persona aktif (§4.12); baris datanya tetap. Contoh di bawah memakai persona `netral`. Isinya bercabang:

  **Hari itu belum ada transaksi:**
  ```
  Belum ada catatan hari ini (Senin, 28 Sep).
  [➕ Pengeluaran] [➕ Pemasukan]
  ```
  → kunci `reminder.empty` (§4.12); teks di atas adalah render persona `netral`.

  **Hari itu sudah ada transaksi:**
  ```
  Ringkasan Sabtu, 26 Sep
  Pengeluaran  Rp 198.200  (4 transaksi)
  Pemasukan    Rp 0

  Periode Gaji (hari ke-6 dari 30)
  Pemasukan    Rp 3.000.000
  Terpakai     Rp 414.700  (13,8% dari pemasukan)
  Sisa         Rp 2.585.300

  Rincian: Makan Rp 181.000, Rumah Tangga Rp 17.200
  ```
  → kunci `reminder.summary` (§4.12); teks di atas adalah render persona `netral`.
  Angka pengeluaran memakai data contoh §8 (26 Sep = hari ke-6 periode 21 Sep–20 Okt); baris `Pemasukan Rp 3.000.000` hanya ilustrasi, §8 tidak memuat pemasukan. Kalau periode belum punya pemasukan sama sekali, baris `Pemasukan`, `Terpakai`, dan `Sisa` diganti satu baris kunci `rekap.no_income` (§4.12) — `netral`: `Belum ada pemasukan dicatat di periode ini.` — menghindari angka negatif yang menyesatkan.
- Jam harus format `HH:MM` 24 jam, `00:00`–`23:59`. Di luar itu → tolak.
- Scheduler: satu goroutine, tick setiap 30 detik. Setiap tick, ambil semua user dengan `reminder_enabled = 1` **dan** `last_reminder_date <> :hari_ini`. Kolomnya `NOT NULL DEFAULT ''` (bukan nullable) supaya perbandingan tidak pernah menghasilkan NULL — `NULL <> 'x'` bernilai NULL di SQL, jadi user baru tidak akan pernah dapat reminder kalau kolomnya nullable. Untuk masing-masing kandidat, cek `waktu sekarang (WIB) >= reminder_time`.
- Karena multi-user, `last_reminder_date` disimpan **per user** di tabel `settings` (bukan variabel global di memori).
- Kalau bot mati melewati jam reminder, reminder **tidak** dikirim susulan setelah bot hidup (tidak ada catch-up). Alasan: reminder yang datang jam 23:00 tidak berguna.

### 4.9 Manajemen transaksi (CRUD)

Transaksi bisa **dibuat, dibaca, diubah, dan dihapus** dari chat.

| Operasi | Cara |
|---|---|
| **Create** | `➕ Pengeluaran` / `➕ Pemasukan` (§4.2) |
| **Read** | `/hari` (hari ini), `/terakhir` (10 terakhir), `/rekap` (agregat) |
| **Update** | Tombol `✏️` per baris di `/hari` dan `/terakhir` |
| **Delete** | Tombol `🗑` per baris di `/hari` dan `/terakhir` |

**Perintah:**
- `/hari` — daftar transaksi hari ini, tiap baris punya `[✏️]` dan `[🗑]`.
- `/terakhir` — 10 transaksi terakhir (lintas tanggal), tiap baris punya `[✏️]` dan `[🗑]`.
- `/batal` — batalkan percakapan yang sedang berjalan.

**Alur edit** (tekan `✏️` pada baris):

1. Bot tampilkan menu field: `[📅 Tanggal] [🏷 Kategori] [💰 Nominal] [📝 Catatan] [↔️ Tipe] [⬅️ Selesai]`.
2. Bot minta nilai baru sesuai field, memakai state yang sama seperti alur input (`tx.edit.date`, `tx.edit.amount`, `tx.edit.note`, `tx.edit.category`, `tx.edit.kind`).
3. Bot tampilkan konfirmasi berisi **nilai lama → nilai baru**:
   ```
   Ubah nominal?
   Tanggal  : 21 Sep 2026 (Senin)
   Kategori : Makan
   Nominal  : Rp 17.000 → Rp 21.000

   [✅ Simpan] [❌ Batal]
   ```

   Baris pertama konfirmasi edit memakai kunci `tx.edit.confirm.header` (§4.12) — contoh `netral`: `Ubah nominal?` (`nominal` mengikuti field yang sedang diubah).
4. `✅ Simpan` → `UPDATE`, balas kunci `tx.updated` (§4.12) — contoh `netral`: `Diperbarui: Makan Rp 21.000`. `❌ Batal` → state dihapus, tidak ada perubahan.

**Aturan edit:**
- Mengubah `tipe` (`expense` ↔ `income`) mewajibkan kategori baru, karena kategori terikat tipe. Bot langsung membuka pilihan kategori setelah tipe diubah, tidak menunggu langkah lain.
- `category_id` hasil edit tetap divalidasi milik user yang sama dan tipenya cocok (aturan yang sama seperti INSERT, §5.3).
- Kolom `updated_at` diisi setiap UPDATE; nilai `created_at` tidak diubah.

**Hapus:**
- Langsung tanpa konfirmasi kedua (tombolnya sudah spesifik per transaksi). Bot balas kunci `tx.deleted` (§4.12) — contoh `netral`: `Terhapus: Makan Rp 17.000`.
- Permanen — tidak ada soft delete dan tidak ada undo. Karena itu tidak ada perintah hapus massal.

### 4.10 Daftar perintah

| Perintah | Fungsi |
|---|---|
| `/start` | Daftarkan user baru (kalau perlu) + pesan sambutan (kunci `welcome`, §4.12) + keyboard utama |
| `/help` | Daftar perintah (kunci `help.list`, §4.12) |
| `/rekap [rentang]` | Rekap per kategori |
| `/hari` | Transaksi hari ini, tiap baris punya tombol `✏️` dan `🗑` |
| `/terakhir` | 10 transaksi terakhir dengan tombol `✏️` dan `🗑` |
| `/kategori` | Kelola kategori |
| `/periode [set\|list]` | Kelola periode rekap |
| `/reminder [on\|off\|HH:MM]` | Kelola reminder |
| `/export [rentang]` | Kirim CSV |
| `/batal` | Batalkan percakapan aktif |
| `/settings` | Ringkasan pengaturan (periode aktif, reminder, kategori, persona) |
| `/persona [set <id>]` | Lihat/ganti persona bot (§4.12) |
| *(pesan bebas)* | **Quick input** `o, 47k, makan, catatan` (§4.13) saat tidak ada percakapan aktif |

### 4.11 State percakapan

Kolom `conversations.state` hanya boleh bernilai salah satu dari:

| State | Menunggu |
|---|---|
| `idle` | Tidak ada percakapan aktif (baris boleh tidak ada) |
| `tx.date` | Input tanggal manual `DD/MM` atau `DD-MM-YYYY` |
| `tx.amount` | Nominal untuk transaksi baru |
| `tx.note` | Teks catatan (dari tombol `✏️ Catatan`) |
| `tx.confirm` | Tekan `✅ Simpan` / `✏️ Catatan` / `❌ Batal` |
| `cat.name` | Nama kategori — dipakai untuk tambah kategori baru maupun ganti nama kategori yang ada |
| `period.start_day` | Tanggal awal periode baru (`2`–`28`) |
| `reminder.time` | Jam reminder `HH:MM` |
| `tx.edit.date` | Tanggal baru untuk transaksi yang sedang diedit |
| `tx.edit.amount` | Nominal baru |
| `tx.edit.note` | Catatan baru |
| `tx.edit.kind` | Tipe baru (`expense`/`income`) |
| `quick.confirm` | Tekan `✅ Simpan` / `✏️ Catatan` / `❌ Batal` hasil quick input (§4.13) |
| `quick.note` | Catatan untuk transaksi quick input |

`payload` menyimpan data sementara sebagai JSON, mis. `{"kind":"expense","occurred_on":"2026-09-21","category_id":3,"amount":17000}`. Handler menolak pesan yang tidak cocok dengan state aktif dengan pesan spesifik, dan state tidak berubah. Pesan teks bebas saat `idle` dibalas kunci `idle.hint` (§4.12) — contoh pada persona `netral`: `Gunakan tombol di bawah, atau /help.`

Daftar state di atas ditegakkan **di kode** (konstanta Go), bukan `CHECK` di DB — supaya menambah state baru tidak butuh migrasi. Baris `conversations` dihapus saat percakapan selesai atau dibatalkan, jadi `state` tidak pernah bernilai `idle` di DB.

### 4.12 Persona bot

Bot tidak berbicara dengan nada datar. Ada **katalog persona hardcoded** yang disimpan di dalam binary; user hanya bisa **memilih**, tidak bisa menambah, mengubah, atau menghapus. Menambah/menghapus persona = ubah kode + rilis ulang.

| ID | Nama | Karakter |
|---|---|---|
| `netral` | Netral | Tanpa persona. Nada datar, informatif. Dipakai sebagai fallback dan sebagai pilihan eksplisit. |
| `penjaga` | Penjaga Harta Karun | Bergaya penjaga setia *Overlord*: agung, terukur, gelap-sopan. Menyebut dirinya "saya", pengguna "Paduka". Catatan disimpan di "buku agung" / "peti kerajaan". Tunduk hormat, bukan sok akrab. |
| `posesif` | Cowok Posesif (Bakugo) | Mengacu Bakugo (*Boku no Hero Academia*): ledakan singkat, ketus, gengsi tinggi, gampang tersulut, nada menantang. Menegur ketidakhadiran catatan, bukan besarnya pengeluaran. Panggilan: "kamu". |
| `softboy` | Softboy Manja (Deku) | Mengacu Deku (*Boku no Hero Academia*): gugup, sopan berlebihan, mudah terharu, memuji pencapaian kecil, sering "kok" / "ya!". Hangat, menyemangati. Panggilan bergantian: "cinta", "love", "sayang". |

**Aturan persona:**

1. **Angka tidak pernah dibungkus.** Nominal, tanggal, jumlah transaksi, dan persentase tampil verbatim (`Rp 414.700`, `26 Sep 2026`, `88,4%`). Persona hanya mengubah kalimat pembungkusnya. Contoh benar: `Peti Paduka terisi Rp 3.000.000.` Contoh salah: `Peti Paduka sudah terisi tiga juta rupiah.`
2. **Semua string user-facing disimpan di satu paket** `internal/persona`, bukan ditulis di handler. Ada test yang gagal kalau ada string user-facing ditulis langsung di paket `bot`.
3. **Permukaan ber-persona:** sambutan `/start`, `/help`, reminder harian, `/rekap`, header `/hari` & `/terakhir`, balasan simpan/edit/hapus/batal, daftar & balasan `/kategori`, `/periode`, `/reminder`, `/settings`, `/persona`, seluruh pesan error quick input (§4.13.4), serta `idle.hint` dan `error.generic`. Daftar lengkapnya ada di tabel kunci di bawah — tabel itu yang jadi acuan test, bukan daftar di butir ini.
4. **Pesan teknis tetap eksplisit.** Error dan validasi boleh memakai persona, tapi alasan dan batasnya harus tetap disebut verbatim. Contoh: `Peti tidak mencatat nominal nol, Paduka — masukkan angka 1–1.000.000.000.` Nominal dan batas angka tidak boleh disamarkan.
5. **Tidak pernah menghakimi nilai pengeluaran.** Persona tegas boleh menegur *ketidakhadiran catatan*, tidak boleh menyalahkan *besarnya pengeluaran*. Alasan: nagging soal uang membuat pengguna berhenti mencatat jujur, dan itu membunuh tujuan produk (rekap akurat).
6. **Export CSV bebas persona.** Isi file selalu header + baris data apa adanya, tanpa kalimat pembungkus.
7. **Persona per user**, disimpan di kolom `settings.persona`, bukan konfigurasi global.

**Perintah:**
- `/persona` — tampilkan persona aktif + daftar pilihan sebagai inline button.
- `/persona set <id>` — ganti persona aktif (`netral`, `penjaga`, `posesif`, `softboy`). ID di luar daftar → tolak dengan pesan yang menyebut ID valid.
- Perubahan berlaku untuk pesan **berikutnya**; pesan yang sudah terkirim tidak berubah.

**Contoh perbedaan nada** untuk pesan reminder saat hari itu sudah ada transaksi (isi data tetap identik):

```
[netral]
Ringkasan Sabtu, 26 Sep
Pengeluaran  Rp 198.200  (4 transaksi)

[penjaga]
Catatan peti kerajaan untuk Sabtu, 26 Sep telah kuhimpun dengan saksama.
Pengeluaran  Rp 198.200  (4 transaksi)

[posesif]
Sabtu, 26 Sep. Nih, sudah aku rapihin. Jangan sampai ada yang bolong, ya.
Pengeluaran  Rp 198.200  (4 transaksi)

[softboy]
Aku sudah rangkum Sabtu, 26 Sep buat kamu! Cek ya, semoga membantu!
Pengeluaran  Rp 198.200  (4 transaksi)
```

Baris data (`Pengeluaran Rp 198.200 (4 transaksi)`) identik di keempat persona; hanya kalimat pembungkus yang berubah.

**Kunci string yang wajib ada di setiap persona** (katalog ini yang dites; handler hanya boleh memanggil kunci, bukan menulis kalimat):

| Kunci | Dipakai di |
|---|---|
| `welcome` | `/start` |
| `tx.saved` | balasan setelah `✅ Simpan` |
| `tx.updated` | balasan setelah `✅ Simpan` di alur edit |
| `tx.deleted` | balasan setelah tombol `🗑` |
| `tx.cancelled` | setelah `❌ Batal` / `/batal` |
| `tx.prompt.amount` | permintaan nominal |
| `tx.prompt.note` | permintaan catatan |
| `tx.prompt.date` | permintaan tanggal manual |
| `tx.confirm.header` | header konfirmasi tambah |
| `tx.edit.confirm.header` | header konfirmasi edit |
| `reminder.empty` | reminder saat hari itu belum ada catatan |
| `reminder.summary` | reminder saat hari itu sudah ada catatan |
| `rekap.header` | header `/rekap` |
| `rekap.empty` | rentang tanpa transaksi |
| `rekap.no_income` | pengganti baris saldo kalau belum ada pemasukan |
| `category.saved` / `category.renamed` / `category.deactivated` | balasan `/kategori` |
| `period.status` | balasan `/periode` |
| `period.saved` | balasan `/periode set` |
| `settings.summary` | balasan `/settings` |
| `reminder.set` / `reminder.off` / `reminder.on` / `reminder.past` | balasan `/reminder` |
| `persona.changed` | balasan `/persona set` |
| `list.header` | header `/hari` & `/terakhir` (judul + total) |
| `help.list` | balasan `/help` |
| `category.list` | header daftar `/kategori` |
| `error.generic` | update gagal diproses (§5.6) |
| `idle.hint` | pesan teks bebas saat `idle` |
| `quick.format` | pesan format salah pada quick input (§4.13.4) |
| `quick.kind` | tipe `o`/`i` tidak dikenal (§4.13.4) |
| `quick.category` | kategori quick input tidak ada / ambigu (§4.13.4) |
| `quick.note_long` | catatan quick input > 200 karakter (§4.13.4) |
**Token placeholder.** String persona boleh memuat placeholder bernama dalam kurung kurawal. Placeholder diganti nilai yang sudah diformat sebelum dikirim; placeholder yang tidak dikenal adalah error program (gagal test), bukan output mentah ke user. Tabel token:

| Token | Isi | Contoh render |
|---|---|---|
| `{amount}` | Nominal terformat | `Rp 17.000` |
| `{date}` | Tanggal panjang | `Sabtu, 26 Sep 2026` |
| `{date_short}` | Tanggal pendek | `26 Sep` |
| `{day}` | Nama hari | `Sabtu` |
| `{category}` | Nama kategori | `Makan` |
| `{note}` | Catatan transaksi, `-` kalau kosong | `maksi` |
| `{kind}` | Jenis transaksi (`pengeluaran`/`pemasukan`) | `pengeluaran` |
| `{count}` | Jumlah transaksi | `4` |
| `{total}` | Total agregat | `Rp 198.200` |
| `{period}` | Nama periode | `Gaji` |
| `{start}` / `{end}` | Tanggal awal/akhir periode | `21 Sep 2026` |
| `{day_no}` / `{day_total}` | Hari ke-N dari total hari periode | `6` / `30` |
| `{pct}` | Persentase | `13,8%` |
| `{remaining}` | Sisa uang periode | `Rp 2.585.300` |
| `{start_day}` | Tanggal awal periode baru | `25` |
| `{time}` | Jam reminder | `19:00` |
| `{persona}` | Nama persona | `Penjaga Harta Karun` |
| `{list}` | ID persona valid, dipisah koma | `netral, penjaga, posesif, softboy` |
| `{input}` | Teks mentah dari user (nama kategori tidak dikenal) | `makna` |
| `{limit}` | Batas angka | `1.000.000.000` |
| `{max}` | Batas jumlah kategori | `20` |
| `{valid}` | Daftar format tanggal yang diterima | `DD/MM atau DD-MM-YYYY` |
| `{title}` | Judul daftar dari handler | `Transaksi hari ini` |

Baris data (nominal, tanggal, jumlah, persentase) **tidak boleh** dimasukkan ke token yang mengubah maknanya — token hanya menyisipkan nilai apa adanya (aturan butir 1).
**Dua kelas pemakaian token:**

1. **Token katalog** — muncul di tabel katalog di bawah. Test memverifikasi setiap token yang muncul di katalog ada di tabel ini (dan sebaliknya tidak ada token tak terdeklarasi).
2. **Token validasi** — hanya muncul di pesan error/validasi yang dibungkus persona (§4.12 butir 4), tidak di tabel katalog: `{limit}`, `{max}`, `{start_day}`, `{list}`, `{valid}`, `{note}`, `{pct}`, `{remaining}`, `{day}`, `{date_short}`. Kesepuluh token ini tetap wajib ada di `persona` karena pesan validasi yang dibungkus persona memakainya; test yang sama (kesalahan placeholder = gagal) berlaku, hanya saja test katalog tidak menuntutnya muncul di tabel.

**Katalog lengkap per persona.** Tabel di bawah adalah isi final untuk v1. Setiap sel adalah nilai kunci tersebut pada persona bersangkutan. Kolom kanan tabel kunci di atas tetap berlaku sebagai daftar kepemilikan kunci.

| Kunci | `netral` | `penjaga` | `posesif` | `softboy` |
|---|---|---|---|
| `welcome` | `Selamat datang. Catat pengeluaran dan pemasukan lewat tombol di bawah, atau /help.` | `Paduka, peti kerajaan telah dibuka. Seluruh catatan akan kujaga setia hingga akhir zaman. Tombol di bawah, atau /help.` | `Hah? Kamu dateng juga ternyata. Yaudah, catatan kamu aku yang pegang. Jangan nggak nyatet, nanti aku semprot. Tombol di bawah, atau /help.` | `Ah, kamu datang! A-aku siap bantu catat semuanya, kok. Tombol di bawah, atau /help ya!` |
| `tx.saved` | `Tersimpan. Total {category} hari ini: {amount}.` | `Tercatat dalam buku agung, Paduka. Total {category} hari ini: {amount}.` | `Cih, sudah aku catat. Total {category} hari ini: {amount}. Jangan sampai kelewat lagi!` | `Sudah aku simpan! Total {category} hari ini: {amount}. Kamu hebat, catatannya lengkap!` |
| `tx.updated` | `Diperbarui: {category} {amount}.` | `Catatan telah kukoreksi dengan khidmat, Paduka: {category} {amount}.` | `Sudah aku ubah: {category} {amount}. Jangan sampai salah dua kali, oke?!` | `Sudah aku perbarui, kok: {category} {amount}. Terima kasih ya!` |
| `tx.deleted` | `Terhapus: {category} {amount}.` | `Catatan {category} {amount} telah dilenyapkan dari buku agung, Paduka.` | `Terhapus: {category} {amount}. Beres. Jangan ada drama lagi.` | `Sudah aku hapus ya: {category} {amount}. Nggak apa-apa, kok!` |
| `tx.cancelled` | `Dibatalkan.` | `Baik, Paduka. Dibatalkan.` | `Ya, sudah aku batalkan. Ck.` | `Oke, aku batalkan ya. Nanti kalau mau lanjut, bilang aja!` |
| `tx.prompt.amount` | `Ketik nominal (contoh: 17000, 17k, 17.5k, 13,5k).` | `Berapa yang keluar dari peti kerajaan, Paduka? Nyatakan jumlahnya (contoh: 17000, 17k).` | `Berapa?! Bilang angkanya (contoh: 17000, 17k). Cepetan, aku nggak punya seharian.` | `Nominalnya berapa ya? Ketik di sini (contoh: 17000, 17k), aku bantu hitung!` |
| `tx.prompt.note` | `Ketik catatan (maks 200 karakter), atau /batal.` | `Adakah keterangan yang ingin Paduka sampaikan pada baris ini? (maks 200 karakter), atau /batal.` | `Catatannya apa? (maks 200 karakter), atau /batal. Nggak usah malu-malu.` | `Mau nulis catatan apa? (maks 200 karakter), atau /batal. Aku baca, kok!` |
| `tx.prompt.date` | `Ketik tanggal ({valid}).` | `Tanggal manakah yang Paduka maksud? Ketik {valid}.` | `Tanggalnya? Ketik {valid}. Jangan ngawur, ya.` | `Tanggal berapa? Ketik {valid} ya!` |
| `tx.confirm.header` | `Tambah {kind}?` | `Catat {kind} ini ke buku agung, Paduka?` | `Simpan {kind} ini? Pikir dulu, jangan salah lagi.` | `Aku simpan {kind} ini, ya? Bener kan datanya?` |
| `tx.edit.confirm.header` | `Ubah {kind}?` | `Perbaiki catatan {kind} ini, Paduka?` | `Ubah {kind} ini? Yaudah, cepetan.` | `Aku ubah {kind} ini, ya? Bener sekarang?` |
| `reminder.empty` | `Belum ada catatan hari ini ({date}).` | `Paduka, buku agung masih kosong untuk hari ini ({date}).` | `Heh. Hari ini ({date}) belum ada catatan sama sekali. Isi sekarang, sebelum aku yang nyariin kamu.` | `Hari ini ({date}) belum ada catatan, lho! Aku tungguin, ya. Semangat!` |
| `reminder.summary` | `Ringkasan {date}` | `Catatan peti kerajaan untuk {date} telah kuhimpun dengan saksama.` | `{date}. Nih, sudah aku rapihin. Jangan sampai ada yang bolong, ya.` | `Aku sudah rangkum {date} buat kamu! Cek ya, semoga membantu!` |
| `rekap.header` | `Rekap {period}` | `Laporan kas kerajaan — {period}` | `Rekap {period} kamu. Nih, aku cek dulu.` | `Ini rekap {period} kamu! Aku susun rapi, kok!` |
| `rekap.empty` | `Belum ada transaksi di periode ini.` | `Buku agung belum mencatat satu pun transaksi di periode ini, Paduka.` | `Nggak ada transaksi di periode ini. Kosong. Ya udah, catat mulai dari sekarang.` | `Belum ada transaksi di periode ini. Nggak apa-apa, mulai sekarang aja!` |
| `rekap.no_income` | `Belum ada pemasukan dicatat di rentang ini.` | `Belum ada pemasukan yang tercatat di buku agung untuk rentang ini, Paduka.` | `Belum ada pemasukan di rentang ini. Catat dulu! Aku nggak mau nanya dua kali.` | `Belum ada pemasukan di rentang ini. Kalau ada, kasih tahu aku ya!` |
| `category.list` | `Daftar kategori:` | `Daftar kategori peti kerajaan:` | `Daftar kategori kamu:` | `Ini daftar kategori kamu!` |
| `category.saved` | `Kategori {category} ditambahkan.` | `Kategori {category} telah ditambahkan ke buku agung, Paduka.` | `Kategori {category} sudah aku tambah. Pakai, jangan cuma didiemin.` | `Kategori {category} sudah aku tambahkan! Semoga kepakai ya!` |
| `category.renamed` | `Kategori diubah menjadi {category}.` | `Kategori buku agung telah berganti nama menjadi {category}, Paduka.` | `Sudah aku ganti jadi {category}. Kali ini aku bantu, ya.` | `Namanya sudah aku ganti jadi {category}! Gampang kan?` |
| `category.deactivated` | `Kategori {category} dinonaktifkan.` | `Kategori {category} tak lagi kuperlihatkan di buku agung, Paduka.` | `Kategori {category} sudah aku matiin. Titik.` | `Kategori {category} aku nonaktifkan ya. Nanti bisa dinyalain lagi!` |
| `period.status` | `Periode aktif: {period}, {start} – {end} (hari ke-{day_no} dari {day_total}).` | `Paduka, periode yang berlaku di peti kerajaan: {period}, {start} – {end} (hari ke-{day_no} dari {day_total}).` | `Periode kamu sekarang: {period}, {start} – {end} (hari ke-{day_no} dari {day_total}). Aku pantau terus, jadi jangan macam-macam.` | `Periode kamu yang aktif: {period}, {start} – {end} (hari ke-{day_no} dari {day_total}). Ayo semangat jaga pengeluaran!` |
| `period.saved` | `Periode {period} disimpan, mulai {start}.` | `Periode {period} telah terukir di papan kerajaan, Paduka — mulai {start}.` | `Sudah aku set. Periode {period} mulai {start}. Beres, kan?` | `Sudah aku simpan, kok! Periode {period} mulai {start}!` |
| `settings.summary` | `Pengaturan: periode {period}, reminder {time}, persona {persona}.` | `Pengaturan peti kerajaan: periode {period}, waktu pengingat {time}, persona {persona}.` | `Setelan kamu: periode {period}, reminder {time}, persona {persona}. Aku yang jaga.` | `Ini setelan kamu: periode {period}, reminder {time}, persona {persona}. Mantap!` |
| `reminder.set` | `Reminder diset {time}.` | `Pengingat kerajaan telah kusetel pada pukul {time}, Paduka.` | `Reminder sudah aku set {time}. Jangan diubah lagi sembarangan.` | `Reminder-nya sudah aku set ke {time}, ya!` |
| `reminder.off` | `Reminder dimatikan.` | `Pengingat kerajaan kumatikan untuk sementara, Paduka.` | `Reminder aku matiin. Ya udah, terserah kamu.` | `Reminder aku matikan dulu ya. Kalau butuh, tinggal bilang!` |
| `reminder.on` | `Reminder dinyalakan.` | `Pengingat kerajaan kunyalakan kembali, Paduka.` | `Reminder aku nyalain lagi. Katanya mau rajin, kan? Buktiin.` | `Reminder-nya sudah aku nyalakan lagi! Ayo kita rajin mencatat!` |
| `reminder.past` | `Reminder diset {time}. Hari ini sudah lewat, mulai berlaku besok.` | `Pengingat kusetel pukul {time}, Paduka — namun hari ini telah berlalu, maka peti akan mengingatkan esok hari.` | `Reminder aku set {time}. Hari ini udah lewat, jadi mulai besok. Salah sendiri telat nyetel.` | `Reminder aku set ke {time}, ya! Hari ini udah lewat, jadi mulai besok. Oke?` |
| `persona.changed` | `Persona diubah ke {persona}.` | `Baik, Paduka. Saya akan berbicara sebagai {persona}.` | `Oke, mulai sekarang aku jadi {persona}. Ck, jangan bikin aku nyesel.` | `Yeay, sekarang aku jadi {persona}! Aku bakal yang terbaik buat kamu!` |
| `list.header` | `{title} ({count} transaksi, total {total})` | `{title} — {count} transaksi, total {total}, Paduka.` | `{title} — {count} transaksi, total {total}. Cek satu-satu, jangan ada yang kelewat.` | `{title} — {count} transaksi, total {total}. Ini dia!` |
| `help.list` | `Perintah: /rekap /hari /terakhir /kategori /periode /reminder /export /persona /settings /batal` | `Perintah yang saya layani, Paduka: /rekap /hari /terakhir /kategori /periode /reminder /export /persona /settings /batal` | `Perintahnya ini: /rekap /hari /terakhir /kategori /periode /reminder /export /persona /settings /batal. Hafalin, aku nggak mau ngulang.` | `Perintah yang bisa kamu pakai: /rekap /hari /terakhir /kategori /periode /reminder /export /persona /settings /batal. Semangat!` |
| `error.generic` | `Terjadi kesalahan. Coba lagi.` | `Maafkan saya, Paduka — peti kerajaan menemui gangguan. Mohon coba kembali.` | `Ada yang error. Coba lagi. Jangan panik.` | `Aduh, ada error! Coba lagi ya, maaf!` |
| `idle.hint` | `Gunakan tombol di bawah, atau /help.` | `Gunakanlah tombol di bawah, atau /help, Paduka.` | `Pakai tombol di bawah, atau /help. Nggak usah bengong.` | `Kamu bisa pakai tombol di bawah, atau /help! Aku di sini, kok!` |
| `quick.format` | `Format: o/i, nominal, kategori, catatan. Contoh: o, 47k, makan, makan malam` | `Formatnya kurang tepat, Paduka: o/i, nominal, kategori, catatan. Contoh: o, 47k, makan, makan malam` | `Formatnya salah! o/i, nominal, kategori, catatan. Contoh: o, 47k, makan, makan malam. Hafalin.` | `Formatnya kayak gini ya: o/i, nominal, kategori, catatan. Contoh: o, 47k, makan, makan malam!` |
| `quick.kind` | `Tipe harus o (pengeluaran) atau i (pemasukan).` | `Tipe transaksi harus o (pengeluaran) atau i (pemasukan), Paduka.` | `o atau i. Nggak ada pilihan lain.` | `Tipenya harus o (pengeluaran) atau i (pemasukan), ya!` |
| `quick.category` | `Kategori "{input}" tidak ada. Pilihan: {list}.` | `Kategori "{input}" tak ada di buku agung, Paduka. Pilihan: {list}.` | `Kategori "{input}" nggak ada. Pilih: {list}. Jangan ngarang.` | `Kategori "{input}" belum ada. Pilihannya: {list}!` |
| `quick.note_long` | `Catatan maks 200 karakter.` | `Catatan tak boleh melebihi 200 karakter, Paduka.` | `Catatan maks 200 karakter. Potong.` | `Catatannya maks 200 karakter, ya!` |

**Catatan katalog:** `{title}` pada `list.header` diisi handler (`Transaksi hari ini` / `10 transaksi terakhir`) — teks itu bagian dari kunci yang sama dan bukan string terpisah, supaya tabel kunci tidak bertambah. `{limit}`, `{max}`, dan `{start_day}` dipakai pada pesan validasi yang menyebut batas; persona boleh membungkusnya tetapi angkanya wajib muncul.

**Default persona: `penjaga`.** Dipilih karena nada paling rendah risiko dibaca tiap hari selama setahun; user bisa ganti kapan saja dengan `/persona set`.

### 4.13 Quick input satu baris

Selain alur tombol, transaksi bisa dicatat dengan **satu pesan** saat tidak ada percakapan berjalan (§4.11 `idle`).

**Format:**

```
<tipe>, <nominal>, <kategori>, <catatan>
```

- **tipe** — `o` = pengeluaran (`expense`), `i` = pemasukan (`income`). Case-insensitive. Harus token pertama persis; `oke, 47k, makan` **bukan** quick input.
- **nominal** — aturan parse sama persis dengan §4.2 (`47k`, `2.850.000`, `13,5k`, dst). Batas `1`–`1.000.000.000`.
- **kategori** — nama kategori milik user, hanya yang **aktif** dan **tipenya cocok**. Pencocokan: exact case-insensitive dulu; kalau tidak ada, **fuzzy** (§4.13.2).
- **catatan** — sisa field, boleh mengandung koma. Maks 200 karakter (§4.2). Boleh kosong.

Tanggal selalu **hari ini (WIB)**. Tidak ada field tanggal di format ini; transaksi tanggal lampau memakai alur tombol §4.2.

Contoh:

```
o, 47k, makan, makan malam
→ pengeluaran Rp 47.000, kategori Makan, catatan "makan malam", hari ini

i, 2.850.000, gaji, gaji maganghub periode september
→ pemasukan Rp 2.850.000, kategori Gaji, catatan "gaji maganghub periode september", hari ini
```

#### 4.13.1 Kapan diaktifkan

Quick input hanya diproses kalau **kedua** syarat ini benar:

1. Tidak ada baris `conversations` untuk user (state `idle`). Kalau ada percakapan berjalan, pesan diperlakukan sebagai input state seperti biasa — tidak ada penyerobotan.
2. Baris pertama pesan, sebelum koma pertama, setelah `trim` dan lowercase, **persis** `o` atau `i`.

Kalau syarat 2 tidak terpenuhi saat `idle`, pesan dibalas `idle.hint` seperti sebelumnya (§4.11).

#### 4.13.2 Pencocokan kategori

Urutan, berhenti di kecocokan pertama:

1. **Exact** — `strings.EqualFold` terhadap nama kategori aktif milik user, dibatasi tipe yang cocok.
2. **Fuzzy** — jarak Levenshtein ≤ 2 **atau** prefix sama ≥ 4 karakter. Ambil kandidat dengan jarak terkecil; seri → tolak (ambigu).
3. **Tidak ada kandidat** → tolak, balas daftar kategori aktif bertipe sama, state tidak berubah.

Fuzzy **tidak pernah langsung menyimpan**. Hasil tebakan selalu masuk kartu konfirmasi (§4.13.3), tempat user melihat kategori yang dipilih dan bisa membatalkan.

#### 4.13.4 Pesan error

Semua pesan error memakai kunci persona (§4.12) dan menyebut alasan verbatim. State tidak berubah.

| Kondisi | Isi pesan (render `netral`) |
|---|---|
| Field kurang dari 4 | `Format: o/i, nominal, kategori, catatan. Contoh: o, 47k, makan, makan malam` |
| Tipe bukan `o`/`i` | `Tipe harus o (pengeluaran) atau i (pemasukan).` |
| Nominal tidak valid | Pesan yang sama dengan §4.2 (menyebut aturan dan batas). |
| Kategori tidak ditemukan / ambigu | `Kategori "<input>" tidak ada. Pilihan: <daftar>`. |
| Catatan > 200 char | `Catatan maks 200 karakter.` |

Kalau pesan diawali `o,`/`i,` tetapi gagal parse, bot **membalas error**, bukan `idle.hint` — supaya user tahu niatnya dikenali tetapi formatnya salah.

## 5. Arsitektur Teknis

### 5.1 Stack

| Komponen | Pilihan | Alasan |
|---|---|---|
| Bahasa | Go 1.26 | Single binary, tanpa runtime eksternal |
| Telegram | `github.com/go-telegram/bot` v1.27.0 | Zero-dependency, aktif (rilis 2026-09-11), mendukung Bot API 10.3. Alternatif `go-telegram-bot-api/v5` sudah tidak diupdate sejak 2024-08 |
| Database | SQLite via `modernc.org/sqlite` v1.59.0 | Pure Go, tanpa cgo → cross-compile & single binary tetap bisa |
| Driver layer | `database/sql` standar | Tidak butuh ORM untuk skema sekecil ini |
| Scheduler | Goroutine + `time.Ticker` (30 detik) | Tidak butuh library cron untuk 1 job |
| Transport | Long polling | Tanpa HTTPS, tanpa domain, tanpa port terbuka |

### 5.2 Struktur direktori

```
telegram_money_bot/
├── cmd/bot/main.go          # wiring, config, signal handling
├── internal/
│   ├── config/              # baca env, validasi
│   ├── storage/             # SQLite: schema, migrasi, query
│   │   ├── migrations/      # file .sql bernomor
│   │   └── *.go
│   ├── money/               # parse nominal, format Rp
│   ├── period/              # hitung rentang periode, resolve periode aktif
│   ├── report/              # agregasi rekap, render teks
│   ├── csv/                 # export CSV
│   ├── persona/             # katalog string per persona (§4.12)
│   ├── quick/               # parser quick input + fuzzy match kategori (§4.13), tanpa I/O
│   ├── bot/                 # handler update, routing perintah
│   │   ├── router.go
│   │   ├── conversation.go  # state machine input
│   │   └── handlers_*.go
│   └── scheduler/           # reminder loop
├── docs/PRD.md
├── go.mod
└── Makefile
```

`internal/` supaya tidak ada API yang bocor ke luar. Paket `money`, `period`, `report`, `csv`, `quick` murni fungsi tanpa I/O → gampang ditest. `quick` mem-parse teks jadi struct kandidat transaksi dan **tidak** menyentuh DB; handler yang mengambil daftar kategori lalu memanggil `quick.Match`.

Paket `persona` berisi katalog string per persona; handler tidak pernah menulis teks user-facing sendiri.

### 5.3 Skema database

```sql
CREATE TABLE users (
  user_id      INTEGER PRIMARY KEY,      -- Telegram user ID
  created_at   TEXT NOT NULL
);

CREATE TABLE categories (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(user_id),
  name         TEXT    NOT NULL,
  kind         TEXT    NOT NULL CHECK (kind IN ('expense','income')),
  active       INTEGER NOT NULL DEFAULT 1,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT    NOT NULL,
  UNIQUE (user_id, kind, name COLLATE NOCASE)
);

CREATE TABLE transactions (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(user_id),
  occurred_on  TEXT    NOT NULL,         -- 'YYYY-MM-DD', waktu lokal WIB
  kind         TEXT    NOT NULL CHECK (kind IN ('expense','income')),
  category_id  INTEGER NOT NULL REFERENCES categories(id),
  amount       INTEGER NOT NULL CHECK (amount > 0),
  note         TEXT    NOT NULL DEFAULT '',
  created_at   TEXT    NOT NULL,
  updated_at   TEXT    NOT NULL          -- diisi juga saat INSERT
);
CREATE INDEX idx_tx_user_date ON transactions(user_id, occurred_on);

CREATE TABLE periods (
  id             INTEGER PRIMARY KEY,
  user_id        INTEGER NOT NULL REFERENCES users(user_id),
  name           TEXT    NOT NULL,
  start_day      INTEGER NOT NULL CHECK (start_day BETWEEN 2 AND 28),
  end_day        INTEGER NOT NULL CHECK (end_day   BETWEEN 1 AND 27),
  effective_from TEXT    NOT NULL,       -- 'YYYY-MM-DD'
  created_at     TEXT    NOT NULL,
  CHECK (end_day = start_day - 1),
  UNIQUE (user_id, effective_from)
);

CREATE TABLE settings (
  user_id            INTEGER PRIMARY KEY REFERENCES users(user_id),
  reminder_enabled   INTEGER NOT NULL DEFAULT 1,
  reminder_time      TEXT    NOT NULL DEFAULT '19:00',
  last_reminder_date TEXT    NOT NULL DEFAULT '',  -- 'YYYY-MM-DD' WIB, '' = belum pernah
  persona            TEXT    NOT NULL DEFAULT 'penjaga'
                             CHECK (persona IN ('netral','penjaga','posesif','softboy'))
);

CREATE TABLE conversations (
  user_id    INTEGER PRIMARY KEY REFERENCES users(user_id),
  state      TEXT    NOT NULL,
  payload    TEXT    NOT NULL DEFAULT '{}',      -- JSON state sementara
  updated_at TEXT    NOT NULL
);
```

Catatan:
- `occurred_on` disimpan sebagai teks `YYYY-MM-DD` (format SQLite native untuk `date()`), bukan Unix timestamp — perbandingan rentang jadi perbandingan string, aman dan indexable.
- `amount` integer rupiah, bukan float.
- `kind` diduplikasi di `transactions` (selain lewat `category_id`) supaya query rekap tidak perlu join hanya untuk memisah pemasukan/pengeluaran.
- `category_id` hanya boleh menunjuk kategori milik `user_id` yang sama, dan `kind`-nya harus cocok dengan `transactions.kind`. Dicek di kode saat INSERT (SQLite tidak bisa menegakkan FK bersyarat tanpa tabel tambahan).
- `settings.persona` dibatasi `CHECK` ke ID yang ada di katalog. Katalog di kode dan daftar di `CHECK` harus sama; test membandingkan keduanya supaya tidak ada persona di kode yang ditolak DB (atau sebaliknya).
- `UNIQUE (user_id, kind, name COLLATE NOCASE)` menegakkan aturan "unik per tipe, case-insensitive" dari §4.4 di level DB, bukan hanya di kode.

### 5.4 Alur data

```
Telegram ──long poll──> bot.Start
                          │
                    router (perintah? tombol? state percakapan?)
                          │
              ┌───────────┼───────────┐
        conversation    query        scheduler
        (state machine) (rekap/dll)  (ticker 30s)
              │             │            │
              └──────> storage (SQLite) ─┘
                          │
                    render teks/CSV ──> sendMessage / sendDocument
```

### 5.5 Konfigurasi

Lewat env var:

| Variabel | Wajib | Default | Keterangan |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | ya | — | Dari @BotFather |
| `DB_PATH` | tidak | `./moneybot.db` | Lokasi file SQLite |
| `TZ` | tidak | `Asia/Jakarta` | Zona waktu bot |
| `LOG_LEVEL` | tidak | `info` | `debug`/`info`/`warn`/`error` |

`TZ` dipakai untuk meresolve lokasi waktu di runtime. Binary mengimpor `time/tzdata` (embed) supaya zona waktu tetap benar di image tanpa `/usr/share/zoneinfo` (mis. container `scratch`).

Bot gagal start (exit code 1) dengan pesan jelas kalau env wajib kosong, token invalid, atau DB tidak bisa dibuka/dimigrasi.

### 5.6 Error handling

- **Update gagal diproses:** log dengan `update_id`, balas kunci persona `error.generic` (§4.12) — contoh pada persona `netral`: `Terjadi kesalahan. Coba lagi.` Jangan pernah panic di handler.
- **Telegram API error 429:** hormati `retry_after` dari respons, retry dengan backoff. Library `go-telegram/bot` menangani ini; verifikasi saat implementasi.
- **Input invalid:** selalu balas pesan spesifik yang menyebut aturannya (`Nominal harus angka antara 1 dan 1.000.000.000`). Persona boleh membungkus kalimatnya, tapi batas dan alasan tetap disebut verbatim (§4.12).
- **User baru:** `/start` mendaftarkan otomatis. Tidak ada approval manual dan tidak ada notifikasi ke user lain.
- **SQLite locked:** `busy_timeout=5000` di DSN, dan pool dibatasi `db.SetMaxOpenConns(1)` — SQLite hanya punya satu writer, dan membiarkan `database/sql` membuka beberapa koneksi tulis menghasilkan `SQLITE_BUSY` acak yang sulit direproduksi. Semua tulis lewat satu koneksi, jadi antrean terjadi di Go, bukan di file lock.

### 5.7 Migrasi

File `internal/storage/migrations/NNNN_nama.sql`, dijalankan berurutan saat start, versi terakhir dicatat di `PRAGMA user_version`. Idempoten: kalau versi sudah sama, skip. Tidak pakai library migrasi eksternal — cukup `embed.FS` + loop.

## 6. Testing

Prioritas test pada logika murni (tanpa I/O, tanpa Telegram):

1. **`money.Parse`** — tabel kasus: `17000`, `17.000`, `17k`, `17,5k`, `13,5k`, `1`, `0` (tolak), `-5` (tolak), `abc` (tolak), `1000000001` (tolak), `17,,5k` (tolak). Ini area bug paling mahal.
2. **`money.Format`** — `17000` → `Rp 17.000`, `1000000` → `Rp 1.000.000`, `0` → `Rp 0`.
3. **`period.Resolve`** — tanggal `2026-09-20` + periode (21,20) → rentang 21 Agu – 20 Sep. Tanggal `2026-09-21` → 21 Sep – 20 Okt. Kasus lintas tahun (`2026-12-25` → 21 Des 2026 – 20 Jan 2027). Kasus Februari.
4. **`period.ResolveActive`** — pilih periode berdasarkan `effective_from` terbesar ≤ tanggal.
5. **`report.Aggregate`** — total per kategori, urutan menurun, penanganan periode kosong, saldo = income − expense.
6. **`csv.Export`** — escaping koma/kutip/newline, prefix anti-injection, header-only untuk rentang kosong.
7. **Conversation state machine** — transisi tiap langkah, `/batal` dari setiap state, input invalid tidak mengubah state, state bertahan setelah proses di-restart.
8. **Isolasi multi-user** — dua user di DB yang sama: user B tidak melihat, mengubah, atau menghapus transaksi, kategori, periode, atau settings milik user A pada `/hari`, `/terakhir`, `/rekap`, `/export`, `/settings`. Termasuk: user B mengirim callback `edit`/`delete` dengan ID transaksi milik A → ditolak tanpa efek.
9. **Edit transaksi** — UPDATE mengubah kolom yang benar, `updated_at` berubah, `created_at` tetap, dan mengubah tipe tanpa mengganti kategori ditolak.
10. **Katalog persona** — setiap ID di `persona.All()` punya nilai **non-kosong** untuk seluruh 35 kunci string yang dipakai handler (§4.12); daftar ID di katalog sama dengan `CHECK (persona IN (...))` di skema DB; ID di luar katalog ditolak oleh `/persona set`. Tanpa DB, tanpa Telegram — katalog diuji sebagai data.
11. **Placeholder persona** — setiap token `{...}` yang muncul di string persona ada di tabel token §4.12 (tidak ada token tak terdeklarasi); render dengan nilai contoh mengganti semua token (tidak ada `{...}` tersisa di output).
12. **Netral sebagai fallback** — kalau kolom `persona` berisi nilai tak dikenal (mis. DB diedit manual), render jatuh ke `netral` tanpa panic, termasuk untuk pesan validasi.
13. **Parser quick input** — tabel kasus murni (tanpa I/O). Terima: `o, 47k, makan, makan malam`; `i, 2.850.000, gaji, gaji maganghub periode september`; `O, 17.000, Makan` (3 field, catatan kosong); `o, 47k, makan, telur, saos, es krim` (koma di catatan). Tolak: `oke, 47k, makan` (token pertama bukan `o`/`i`, bukan quick input sama sekali); `x, 47k, makan, x` (tipe salah); `o, abc, makan, x` (nominal invalid); `o, 47k, makan` tanpa catatan tetap sah, tetapi `o` saja tolak; `o, 47k, makan, <201 karakter>` tolak.
14. **Pencocokan kategori quick input** — tabel kasus: exact case-insensitive (`makan` vs `Makan`, `GAJI` vs `Gaji`); fuzzy jarak 1 (`makna` → `Makan`), jarak 2, prefix ≥ 4 (`transportasi` → `Transport`); jarak 3 → tolak; dua kandidat berjarak sama → tolak (ambigu); tipe tidak cocok (`makan` pada pesan `i, ...`) → tolak; kategori nonaktif tidak pernah jadi kandidat. Uji juga: hasil fuzzy **tidak pernah** INSERT sebelum `✅ Simpan` ditekan.
15. **Gate quick input** — pesan berawalan `o,`/`i,` saat ada baris `conversations` aktif diperlakukan sebagai input state biasa, bukan quick input; pesan bebas biasa saat `idle` dibalas `idle.hint`; pesan berawalan `o,` yang gagal parse dibalas error quick input, bukan `idle.hint`.

Test integrasi storage memakai file SQLite sementara (`t.TempDir()`), bukan `:memory:` (agar perilaku sama dengan produksi).

Tidak ada test yang memanggil Telegram API. Handler diuji dengan interface `Sender` yang bisa di-mock.

## 7. Kriteria Penerimaan

| # | Kriteria | Cara verifikasi |
|---|---|---|
| 1 | Bot start dengan env valid, siap menerima pesan | Jalankan, kirim `/start`, dapat balasan |
| 2 | Alur tambah pengeluaran 5 langkah selesai & tersimpan | Manual: tambah 1 transaksi, cek `/hari` |
| 3 | Nominal `17k`, `17.000`, `13,5k` terparse benar | Unit test + manual |
| 4 | `/rekap` menampilkan breakdown per kategori dengan total & persentase benar | Manual dengan data contoh dari pengguna (§8) |
| 5 | `/rekap 2026-09-21..2026-09-26` cocok dengan hitungan tangan | Manual |
| 6 | Periode (21,20) resolve benar untuk tanggal sebelum & sesudah tanggal 21 | Unit test |
| 7 | `/periode set 25` mengubah periode aktif mulai hari ini; rekap tanggal lama tidak berubah | Manual |
| 8 | `/export` mengirim CSV yang bisa dibuka di LibreOffice tanpa kolom bergeser | Manual |
| 9 | Reminder selalu terkirim pada jam yang diset, dengan isi bercabang benar (kosong vs ada transaksi) | Set jam reminder 2 menit ke depan; uji dua kondisi, cek isi pesan |
| 10 | Reminder terkirim sekali (bukan berulang) dalam satu hari | Set jam 2 menit ke depan, tunggu 3 menit, hanya 1 pesan |
| 11 | `/reminder off` menghentikan reminder | Manual |
| 12 | Kategori baru muncul di keyboard input | `/kategori` → tambah → cek alur input |
| 13 | Kategori yang sudah dipakai tidak bisa dihapus, hanya nonaktif | Manual |
| 14 | Bot restart di tengah percakapan input; percakapan bisa dilanjutkan atau `/batal` bekerja | Manual |
| 15 | Data contoh §8 direkap dan hasilnya cocok dengan total yang dihitung tangan | Manual + unit test agregasi |
| 16 | User B tidak bisa melihat, mengubah, atau menghapus data user A | Dua akun Telegram; `/hari`, `/terakhir`, `/rekap`, `/export` di akun B tidak menampilkan data A; callback edit/hapus dengan ID milik A ditolak |
| 17 | Semua varian `/rekap` (`hari`, `kemarin`, `minggu`, `bulan`, `periode`, rentang kustom) menghasilkan rentang tanggal yang benar | Unit test `period` + manual, dibandingkan dengan kalender |
| 18 | Edit tiap field transaksi (tanggal, kategori, nominal, catatan, tipe) tersimpan dan tercermin di `/hari` | Manual per field |
| 19 | Hapus dari `/hari` dan `/terakhir` menghilangkan baris dan total rekap ikut turun | Manual |
| 20 | `/persona` menampilkan 4 pilihan; `/persona set posesif` mengubah persona dan balasan berikutnya memakai gaya itu | Manual |
| 21 | Angka identik di semua persona: baris data reminder/rekap tidak berubah saat persona diganti | Unit test render: nominal & tanggal sama untuk 4 persona |
| 22 | `/persona set xxx` (ID tak dikenal) ditolak, persona aktif tidak berubah | Manual + unit test |
| 23 | Persona user A tidak memengaruhi persona user B | Dua akun Telegram |
| 24 | Quick input `o, 47k, makan, makan malam` tersimpan sebagai pengeluaran hari ini, kategori Makan, catatan "makan malam" | Manual: kirim pesan, cek kartu konfirmasi, `✅ Simpan`, cek `/hari` |
| 25 | Quick input `i, 2.850.000, gaji, gaji maganghub periode september` tersimpan sebagai pemasukan | Manual: sama seperti #24 |
| 26 | Kategori dengan typo (`makna`) diusulkan sebagai `Makan` di kartu konfirmasi, dan `❌ Batal` mencegah penyimpanan | Manual + unit test pencocokan |
| 27 | Pesan biasa saat `idle` (mis. `oke, 47k, makan`) tidak pernah jadi transaksi | Manual + unit test gate |

## 8. Data Contoh (untuk verifikasi)

Diambil dari input pengguna, tahun 2026 (21 Sep 2026 = Senin, 22 Sep = Selasa, 26 Sep = Sabtu).

| Tanggal | Item | Nominal | Kategori |
|---|---|---|---|
| 2026-09-21 (Senin) | maksi | 17.000 | Makan |
| 2026-09-21 | maklam | 15.000 | Makan |
| 2026-09-21 | catering | 135.000 | Makan |
| 2026-09-22 (Selasa) | roti sarapan | 5.000 | Makan |
| 2026-09-22 | roti tawar | 13.500 | Makan |
| 2026-09-23 (Rabu) | sunlight | 5.000 | Rumah Tangga |
| 2026-09-24 (Kamis) | bensin | 26.000 | Transport |
| 2026-09-26 (Sabtu) | telur, saos 5 sachet, 2 energen | 10.700 | Rumah Tangga |
| 2026-09-26 | catering | 165.000 | Makan |
| 2026-09-26 | maklam | 16.000 | Makan |
| 2026-09-26 | spons, es krim | 6.500 | Rumah Tangga |

**Hasil yang diharapkan untuk periode 21 Sep – 20 Okt 2026:**

| Kategori | Total | Transaksi | Porsi |
|---|---|---|---|
| Makan | 366.500 | 7 | 88,4% |
| Transport | 26.000 | 1 | 6,3% |
| Rumah Tangga | 22.200 | 3 | 5,4% |
| **Total pengeluaran** | **414.700** | **11** | **100%** |

Catatan keputusan untuk baris `telur 3, saos 5 sachet, 2 energen 10,7k`: input chat-only berarti pengguna memasukkan **satu nominal 10.700** untuk baris itu (bukan tiga transaksi). Kalau ingin memisah per item, pengguna mencatat tiga transaksi terpisah. Ini konsekuensi dari keputusan tanpa parser bebas di §4.3.

## 9. Deployment

- **Cara jalankan:** `make build` → `./moneybot` dengan env terisi. Atau `systemd` unit sederhana (`Restart=always`).
- **Backup:** file `moneybot.db` cukup di-copy (matikan bot dulu, atau pakai `sqlite3 moneybot.db ".backup"`). Tidak ada dependensi eksternal.
- **Tidak butuh:** domain, HTTPS, port terbuka, reverse proxy, container, cloud service.
- **Kebutuhan minimum:** satu proses yang hidup terus (VPS kecil, Raspberry Pi, atau PC yang menyala).

## 10. Risiko

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Long polling butuh proses hidup; bot mati = reminder tidak terkirim | Reminder hari itu hilang | Diterima (§4.8, tanpa catch-up). `/hari` tetap bisa dipakai untuk cek |
| Bot disalahgunakan user asing (spam) | Beban proses, sampah data | Diterima untuk tool pribadi; opsional v2: rate limit per `user_id` (mis. maks 60 update/menit) |
| Jam reminder diubah ke waktu yang sudah lewat hari ini | Reminder tidak terkirim hari itu | Perilaku eksplisit: reminder hanya untuk waktu ke depan. Bot balas kunci `reminder.past` (§4.12) — contoh `netral`: `Reminder diset 19:00. Hari ini sudah lewat, mulai berlaku besok.` |
| Kategori bertambah banyak → keyboard jadi panjang | Sulit dipakai di HP | Batas 20 kategori per tipe (§4.4) |
| SQLite file hilang tanpa backup | Kehilangan semua data | Dokumentasi backup (§9). Opsional v2: auto-backup harian ke chat Telegram |
| Nada persona "tegas" terasa menghakimi pengeluaran | Pengguna berhenti mencatat jujur → rekap tidak akurat | Aturan eksplisit §4.12 butir 5: tegur ketiadaan catatan, jangan nilai pengeluaran. Persona bisa diganti/dimatikan (`/persona set netral`) |
| Katalog string persona tidak lengkap → pesan kosong di produksi | Pengalaman rusak di jalur yang jarang dibuka | Test §6 #10: setiap ID wajib punya semua kunci; kunci kosong menggagalkan test |
| Fuzzy match kategori salah tebak (mis. `makna` → `Makan`) lalu user buru-buru menekan `✅ Simpan` | Data tercatat di kategori yang salah tanpa disadari | Fuzzy **tidak pernah** auto-save; hasil selalu lewat kartu konfirmasi yang menandai `(dari "makna")`. Ambigu (dua kandidat berjarak sama) → tolak, tidak menebak. Bisa dimatikan dengan mengetik nama kategori persis |
| Quick input salah parse (mis. catatan user sendiri mengandung koma di awal) | Pesan gagal parse → error, bukan transaksi salah | Format wajib 4 field; hanya jalan saat `idle`; pesan berawalan `o,`/`i,` yang gagal selalu dibalas error yang menyebut format (§4.13.4), tidak pernah diam-diam jadi transaksi |

## 11. Keputusan

1. **Format tanggal saat input manual:** `DD/MM` = tahun berjalan, kecuali tanggalnya di masa depan → tahun sebelumnya (diputuskan, lihat §4.2).
2. **`/rekap minggu`** — minggu mulai Senin (diputuskan).
3. **Edit transaksi** — diputuskan: CRUD penuh (§4.9). Edit per field dengan tombol `✏️`, bukan hapus + tambah ulang.
4. **Ringkasan di reminder** — diputuskan: reminder selalu dikirim tiap hari, dengan ringkasan kalau sudah ada transaksi (§4.8).
5. **Persona** — diputuskan: 3 persona + netral, hardcoded di kode, user hanya memilih (§4.12). Default `penjaga`. Tidak ada persona buatan user.
6. **Quick input** — diputuskan: format ketat `o/i, nominal, kategori, catatan` (§4.13), hanya saat `idle`, tanggal selalu hari ini, kategori lewat exact-then-fuzzy dengan konfirmasi wajib. Non-Tujuan §2.2 disesuaikan: yang ditolak adalah *parsing bahasa bebas/LLM*, bukan format bertoken ini.

Tidak ada pertanyaan yang masih menggantung. Semua keputusan di atas sudah final untuk v1; perubahan berikutnya masuk sebagai revisi PRD, bukan asumsi implementasi.
