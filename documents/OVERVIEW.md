# Overview

save-drive adalah CLI yang ditaruh di PATH untuk mengelola file release di Google Drive milik pribadi. Tujuannya: script build bisa upload hasil release ke Drive, dan file di Drive bisa diganti, diunduh, dilihat, atau dihapus hanya lewat command line. Spesifikasi awal dari pemilik project ada di [`PROJECT.md`](../PROJECT.md).

## Pengguna

Ada dua jenis pemanggil, keduanya memakai akun Google yang sama:

- **Pemilik project**, yang menjalankan command langsung di terminal, termasuk `authorize` untuk login pertama.
- **Script build/release** (PowerShell), yang memanggil `push` dan command lain tanpa interaksi. Script sering menangkap output command, jadi URL OAuth tidak terlihat. Karena itu `authorize` harus dijalankan manual lebih dulu.

Tidak ada peran atau izin di dalam aplikasi. Hak akses sepenuhnya mengikuti akun Google yang memberi izin OAuth, atau service account kalau itu yang dipakai.

## Alur utama

1. **Setup dan authorize.** User membuat OAuth client (Desktop app) di Google Cloud, mengisi `GOOGLE_OAUTH_CLIENT_CREDENTIALS` dan `DRIVE_ROOT_ID`, lalu menjalankan `save-drive authorize`. CLI mencetak URL consent, membuka browser, menerima callback di `127.0.0.1:8085`, lalu menyimpan token ke `.save-drive/oauth-token.json`.
2. **Push release.** `push --file <lokal> --to <folder/path> [--mkdir] [--replace]` mencari folder dari root, membuatnya kalau `--mkdir` diberikan, menolak kalau nama file sudah ada kecuali ada `--replace`, lalu upload.
3. **List.** `list --from <folder/path>` mencetak isi folder sebagai TSV: id, tipe (`file`/`folder`), nama, waktu modifikasi.
4. **Fetch.** `fetch --id <id>` atau `fetch --name <nama> --from <folder>` mengunduh ke `--to`, atau ke nama file di direktori sekarang.
5. **Delete.** `delete --id <id>` atau `delete --name <nama> --from <folder>` menghapus file permanen dari Drive (lewat API `Files.Delete`, tidak masuk trash).
6. **Audit.** Setiap command data menambah satu baris ke `logs/audit.jsonl` di samping exe; `logs [--limit N]` menampilkan entri terbaru lebih dulu.

## Area fitur

| Area | Direktori |
|------|-----------|
| Command dan flag | `internal/cli/` (satu file per command) |
| Auth dan akses Drive | `internal/gdrive/` |
| Audit log | `internal/audit/` |
| Env var dan path default | `internal/config/` |
| Versi build | `internal/buildinfo/`, di-stamp oleh `scripts/release.ps1` |
| Build dan release | `Taskfile.yml`, `scripts/` |

## Istilah

- **Root folder**: folder Drive tempat semua folder path dimulai. Diatur lewat `DRIVE_ROOT_ID` atau `--root-id`; default `root` (My Drive).
- **Folder path**: path berpemisah `/` di bawah root folder, misalnya `Releases/Prod`. Di-resolve per segmen berdasarkan nama oleh `ResolveFolderPath`.
- **Push / fetch**: upload / download satu file.
- **Replace**: `push --replace` menghapus file bernama sama di folder tujuan, lalu upload.
- **Creds**: file JSON credential, berupa OAuth client (`installed`/`web`) atau service account (`type: service_account`). Tipe dideteksi dari isinya.
- **Audit event**: satu record JSON di audit log dengan `action` (nama command) dan `result` (`success`/`failed`).

## Status

Berjalan: semua command di `README.md`, auth OAuth dan service account, build release dengan versi dari `git describe`.

Sedang berjalan: migrasi ke pola yang diadopsi di [`CODE-PATTERNS.md`](CODE-PATTERNS.md) (akses Drive di gdrive, audit sekali per run, context dari cobra, test).

Masalah yang diketahui:

- `push --replace` menghapus file lama **sebelum** upload. Kalau upload gagal, file lama sudah hilang (`internal/cli/push.go`).
- `list` tidak paginasi dan berhenti di 1000 item pertama (`internal/cli/list.go`).
- `delete --name` menghapus semua file dengan nama itu; `fetch --name` mengambil yang pertama ditemukan.
- Audit untuk kegagalan tidak lengkap: sebagian jalur error tidak tercatat.
- Buka browser otomatis memakai `rundll32`, jadi hanya jalan di Windows. Di OS lain URL tetap tercetak untuk dibuka manual.
- Belum ada test dan belum ada CI.
