# Save Drive CLI

CLI sederhana untuk upload/fetch/list/delete file di Google Drive. Default auth untuk akun personal: OAuth user login.

## Setup (Personal Account - Recommended)

1. Enable Google Drive API di Google Cloud project.
2. Buka **APIs & Services > Credentials**.
3. Klik **Create Credentials > OAuth client ID**.
4. Pilih **Desktop app** lalu download JSON credential.
5. Set environment variable:

```powershell
$env:GOOGLE_OAUTH_CLIENT_CREDENTIALS="D:\keys\oauth-client.json"
$env:DRIVE_ROOT_ID="<FOLDER_ID_ROOT_PROJECT>"
```

Jalankan authorize sekali untuk membuat token lokal:

```powershell
./save-drive.exe authorize
```

Saat authorize dimulai, CLI akan:

- print Google OAuth URL ke terminal
- coba buka browser otomatis
- tetap bisa dipakai manual dengan copy/paste URL kalau browser tidak kebuka atau callback gagal

Token disimpan lokal ke `.save-drive/oauth-token.json` (relatif ke working directory). Token yang di-refresh otomatis ditulis ulang ke file yang sama.

`authorize` selalu menjalankan flow baru dan menimpa token lama. Kalau command gagal dengan `oauth token is invalid or expired`, cukup jalankan `save-drive authorize` lagi. Ini sering terjadi kalau OAuth consent screen masih berstatus *Testing*, karena Google membatalkan refresh token setelah 7 hari. Publish app ke *In production* supaya token tidak kedaluwarsa.

Catatan penting untuk automation/script:

- URL OAuth dan pesan token dicetak ke **stderr**, jadi tidak tercampur dengan output yang di-parse script dari stdout. Kalau stderr juga di-capture (`2>&1`), URL tidak tampil di terminal
- karena itu jalankan `save-drive authorize` dulu sebelum step build/upload
- callback OAuth memakai port acak di `127.0.0.1` untuk client *Desktop app*; client tipe *Web* tetap memakai `127.0.0.1:8085`

## Setup (Opsional - Service Account)

Tetap didukung via `GOOGLE_APPLICATION_CREDENTIALS`. Service account tidak punya storage quota sendiri, jadi `push` ke My Drive milik service account pasti gagal. Pakai Shared Drive: tambahkan service account sebagai member, lalu set `DRIVE_ROOT_ID` ke ID Shared Drive (atau folder di dalamnya).

## Build

Plain `go build`:

```powershell
go build -o save-drive.exe ./cmd/save-drive
```

### With Task (recommended)

Uses [Task](https://taskfile.dev) (`winget install Task.Task`). Run `task` to list everything.

```powershell
task test       # unit tests (tanpa credential dan jaringan)
task dev        # fast dev build  -> bin\save-drive.exe
task release    # optimized exe   -> dist\save-drive.exe  (stripped + version-stamped)
task clean      # remove bin\ and dist\

# Pass args after `--`:
task dev -- list --from Test          # build then run
task release -- -Version 1.2.0        # stamp an explicit version
```

`release` strips debug info (`-s -w`), uses `-trimpath`, and stamps version/commit/date.
The default version comes from `git describe`. Check it with:

```powershell
.\dist\save-drive.exe version
```

## Project Layout

```
cmd/save-drive/      entrypoint (main)
internal/cli/        cobra commands, one file per command
internal/gdrive/     Google Drive client + OAuth flow
internal/audit/      JSONL audit logging
internal/config/     env vars and default file paths
internal/buildinfo/  version metadata stamped via -ldflags
scripts/             PowerShell build scripts used by Taskfile.yml
```

Dokumen untuk kontributor ada di `AGENTS.md` dan `documents/`.

## Commands

```powershell
./save-drive.exe authorize
./save-drive.exe whoami             # akun yang dipakai
./save-drive.exe logout             # hapus token lokal

# Upload file, buat folder jika belum ada, replace jika nama sama
./save-drive.exe push --file .\README.md --to Test --mkdir --replace

# Upload banyak file sekaligus (--file bisa diulang dan menerima glob)
./save-drive.exe push --file .\dist\*.zip --file .\dist\SHA256SUMS --to Releases/1.2.0 --mkdir

# Lihat apa yang akan terjadi tanpa mengubah Drive
./save-drive.exe push --file .\dist\app.zip --to Releases/Prod --replace --dry-run

# List isi folder (TSV: id, type, name, modified, size, path)
./save-drive.exe list --from Test
./save-drive.exe list --from Releases --recursive --json

# Download by name; --to boleh folder
./save-drive.exe fetch --name README.md --from Test --to .\downloads\
./save-drive.exe fetch --name app.zip --from Releases/Prod --latest

# Pindahkan ke trash (bisa di-restore 30 hari); --permanent untuk hapus permanen
./save-drive.exe delete --name README.md --from Test

# Sisakan 5 release terbaru, sisanya ke trash
./save-drive.exe prune --from Releases/Prod --keep 5 --match "*.zip" --dry-run

# Buat link "anyone with the link" dan cetak URL-nya
./save-drive.exe share --name app.zip --from Releases/Prod
./save-drive.exe share --name app.zip --from Releases/Prod --revoke

# Lihat 10 audit log terbaru
./save-drive.exe logs

# Lihat 20 audit log terbaru
./save-drive.exe logs --limit 20
```

Perilaku yang penting untuk script:

- `push` memvalidasi file lokal sebelum menyentuh Drive. Dengan `--replace`, file baru di-upload dan checksum MD5-nya dicek dulu, baru file lama dipindah ke trash. Upload yang gagal tidak menghilangkan file lama.
- `--name` hanya mencocokkan file, tidak pernah folder.
- `fetch --name` dan `share --name` gagal kalau ada lebih dari satu file dengan nama itu; pakai `--latest` atau `--id`.
- `fetch` menulis ke `<file>.part` lalu rename setelah checksum cocok. Google Docs/Sheets/Slides di-export ke `.docx`/`.xlsx`/`.pptx`.
- `delete` dan `prune` mencetak `trashed: ...`, atau `deleted: ...` dengan `--permanent`.
- Error 429/5xx dan rate limit dari Drive di-retry otomatis dengan backoff. Ctrl+C membatalkan request yang sedang jalan.
- `list`, `push`, `fetch`, `share`, dan `whoami` punya `--json`.

## Audit Log

Default path audit log mengikuti lokasi binary:

- jika binary ada di `C:\Tools\save-drive\save-drive.exe`
- maka log ada di `C:\Tools\save-drive\logs\audit.jsonl`

Bisa override dengan env var:

```powershell
$env:SAVE_DRIVE_LOG_FILE="D:\custom\audit.jsonl"
```

Kalau log tidak bisa ditulis (misalnya exe ada di `C:\Program Files`), command tetap jalan dan mencetak `warning: write audit log ...` ke stderr. Set `SAVE_DRIVE_LOG_FILE` ke lokasi yang bisa ditulis.
