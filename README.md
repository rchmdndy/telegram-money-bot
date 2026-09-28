# telegram-money-bot

Bot Telegram untuk mencatat pengeluaran & pemasukan harian, lalu merekapnya per **periode penggajian** yang bisa diatur sendiri (contoh: 21 → 20 bulan berikutnya).

Chat-only: tidak ada Mini App, tidak ada web server, tidak ada port yang dibuka. Satu binary Go + satu file SQLite.

- Spesifikasi lengkap: [`docs/PRD.md`](docs/PRD.md)
- Go 1.26, `modernc.org/sqlite` (pure Go, tanpa cgo)
- Multi-user: setiap `user_id` Telegram terisolasi

## Fitur

| | |
|---|---|
| Catat transaksi | Alur 5 langkah lewat tombol, atau **quick input** `o, 47k, makan, makan malam` |
| Nominal | `17k`, `17.000`, `13,5k`, `17000` — semuanya terparse |
| Rekap | Per kategori dengan total & persentase, untuk rentang apa pun |
| Periode gaji | Tanggal awal 2–28, sisa bulan mengikuti (mis. 25 → 24) |
| Edit & hapus | Per field (`✏️`), dari `/hari` dan `/terakhir` |
| Kategori | Seed bawaan + bisa ditambah/ganti nama/nonaktifkan (maks 20 aktif per tipe) |
| Reminder harian | Jam bisa diatur (default 19:00), berisi ringkasan kalau hari itu sudah ada catatan |
| Export | CSV siap dibuka di LibreOffice/Excel |
| Persona | 4 gaya balasan: `netral`, `penjaga` (default), `posesif`, `softboy` |

## Perintah

```
/start                 daftar + pesan sambutan
/help                  daftar perintah
/rekap [rentang]       rekap per kategori (hari, kemarin, minggu, bulan, periode, 2026-09-21..2026-09-26)
/hari                  transaksi hari ini + tombol edit/hapus
/terakhir              10 transaksi terakhir
/kategori              kelola kategori
/periode [set|list]    kelola periode rekap
/reminder [on|off|HH:MM]
/export [rentang]      kirim CSV
/persona [set <id>]    lihat/ganti persona
/settings              ringkasan pengaturan
/batal                 batalkan percakapan aktif
```

Pesan bebas saat tidak ada percakapan aktif diperlakukan sebagai quick input:

```
o, 47k, makan, makan malam        → pengeluaran Rp 47.000, kategori Makan, catatan "makan malam"
i, 2.850.000, gaji, september     → pemasukan
```

Formatnya ketat 4 field (`tipe, nominal, kategori, catatan`). Tidak ada parsing bahasa bebas/LLM — pesan yang tidak diawali `o,`/`i,` tidak pernah jadi transaksi. Kategori dengan typo (`makna`) diusulkan sebagai `Makan` di kartu konfirmasi, dan **selalu** lewat konfirmasi sebelum disimpan.

## Menjalankan

### Docker (disarankan)

```bash
docker run -d --name moneybot \
  -e TELEGRAM_BOT_TOKEN=123456:ABC... \
  -e TZ=Asia/Jakarta \
  -v moneybot-data:/data \
  --restart unless-stopped \
  ghcr.io/rchmdndy/telegram-money-bot:latest
```

Image `distroless/static`, jalan sebagai `nonroot`, ukuran ~23 MB. Database ada di volume `/data`.

### Binary

```bash
make build          # menghasilkan ./moneybot
TELEGRAM_BOT_TOKEN=123456:ABC... ./moneybot
```

### Environment

| Variabel | Wajib | Default | Keterangan |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | ya | — | dari [@BotFather](https://t.me/BotFather) |
| `DB_PATH` | tidak | `./moneybot.db` | lokasi file SQLite |
| `TZ` | tidak | `Asia/Jakarta` | zona waktu reminder & tanggal |
| `LOG_LEVEL` | tidak | `info` | `debug`, `info`, `warn`, `error` |

### systemd

```ini
[Unit]
Description=Telegram money bot
After=network-online.target

[Service]
Environment=TELEGRAM_BOT_TOKEN=123456:ABC...
Environment=TZ=Asia/Jakarta
Environment=DB_PATH=/var/lib/moneybot/moneybot.db
ExecStart=/usr/local/bin/moneybot
Restart=always
User=moneybot

[Install]
WantedBy=multi-user.target
```

## Development

```bash
make test     # go test ./...
make vet      # go vet ./...
make fmt      # gofmt -l -w .
make build
```

Semua handler diuji lewat interface `Sender` yang di-mock — tidak ada test yang menyentuh API Telegram. Test storage memakai file SQLite di `t.TempDir()`.

## CI/CD

- **CI** (`.github/workflows/ci.yml`) — setiap push & PR: `gofmt`, `go vet`, `go test -race` + coverage, dan `docker build` dengan cache GitHub Actions. Image hasil build di-smoke-test di CI.
- **CD** (`.github/workflows/cd.yml`) — push ke `master`/`main` atau tag `v*`: build & push image ke GHCR dengan tag branch, tag git, short SHA, dan `latest`.

## Backup

File SQLite cukup di-copy (matikan bot dulu, atau `sqlite3 moneybot.db ".backup 'backup.db'"`). Tidak ada dependensi eksternal.
