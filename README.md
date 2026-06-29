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

Token disimpan lokal ke `.save-drive/oauth-token.json`.

Catatan penting untuk automation/script:

- kalau `save-drive` dipanggil dari PowerShell dengan output di-capture, misalnya `& save-drive list ... 2>&1`, maka URL OAuth juga ikut tertangkap dan tidak tampil ke terminal
- untuk kasus itu, jalankan `save-drive authorize` dulu sebelum step build/upload

## Setup (Opsional - Service Account)

Tetap didukung via `GOOGLE_APPLICATION_CREDENTIALS`, tapi untuk akun personal biasanya terbatas quota/ownership.

## Build

```powershell
go build -o save-drive.exe .
```

## Commands

```powershell
./save-drive.exe authorize

# Upload file, buat folder jika belum ada, replace jika nama sama
./save-drive.exe push --file .\README.md --to Test --mkdir --replace

# List isi folder
./save-drive.exe list --from Test

# Download by name
./save-drive.exe fetch --name README.md --from Test --to .\downloads\README.md

# Delete by name
./save-drive.exe delete --name README.md --from Test

# Lihat 10 audit log terbaru
./save-drive.exe logs

# Lihat 20 audit log terbaru
./save-drive.exe logs --limit 20
```

## Audit Log

Default path audit log mengikuti lokasi binary:

- jika binary ada di `C:\Tools\save-drive\save-drive.exe`
- maka log ada di `C:\Tools\save-drive\logs\audit.jsonl`

Bisa override dengan env var:

```powershell
$env:SAVE_DRIVE_LOG_FILE="D:\custom\audit.jsonl"
```
