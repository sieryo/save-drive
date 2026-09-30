# Overview

save-drive adalah CLI yang ditaruh di PATH untuk mengelola file release di Google Drive milik pribadi. Tujuannya: script build bisa upload hasil release ke Drive, dan file di Drive bisa diganti, diunduh, dilihat, atau dihapus hanya lewat command line. Spesifikasi awal dari pemilik project ada di [`PROJECT.md`](../PROJECT.md).

## Pengguna

Ada dua jenis pemanggil, keduanya memakai akun Google yang sama:

- **Pemilik project**, yang menjalankan command langsung di terminal, termasuk `authorize` untuk login pertama.
- **Script build/release** (PowerShell), yang memanggil `push` dan command lain tanpa interaksi. Script sering menangkap output command, jadi URL OAuth tidak terlihat. Karena itu `authorize` harus dijalankan manual lebih dulu.

Tidak ada peran atau izin di dalam aplikasi. Hak akses sepenuhnya mengikuti akun Google yang memberi izin OAuth, atau service account kalau itu yang dipakai.

## Alur utama

1. **Setup dan authorize.** User membuat OAuth client (Desktop app) di Google Cloud, mengisi `GOOGLE_OAUTH_CLIENT_CREDENTIALS` dan `DRIVE_ROOT_ID`, lalu menjalankan `save-drive authorize`. CLI mencetak URL consent ke stderr, membuka browser, menerima callback di port acak `127.0.0.1` (client tipe web: `127.0.0.1:8085`), lalu menyimpan token ke `.save-drive/oauth-token.json`. `authorize` selalu menjalankan flow baru, jadi juga dipakai untuk memulihkan token yang dicabut atau kedaluwarsa. `whoami` menampilkan akun aktif, `logout` menghapus token lokal.
2. **Push release.** `push --file <lokal>... --to <folder/path> [--mkdir] [--replace] [--dry-run]` memvalidasi semua file lokal (glob diekspansi), mencari folder dari root, membuatnya kalau `--mkdir` diberikan, dan menolak kalau nama file sudah ada kecuali ada `--replace`. Semua nama dicek sebelum upload pertama. Setiap file di-upload, checksum MD5 dicek, lalu file lama bernama sama dipindah ke trash.
3. **List.** `list --from <folder/path> [--recursive] [--json]` mencetak isi folder sebagai TSV: id, tipe (`file`/`folder`), nama, waktu modifikasi, ukuran, path relatif. Paginasi diikuti sampai habis.
4. **Fetch.** `fetch --id <id>` atau `fetch --name <nama> --from <folder> [--latest]` mengunduh ke `--to` (file atau folder), atau ke nama file di Drive di direktori sekarang. Download ditulis ke `.part` lalu di-rename setelah checksum cocok. File Google Workspace di-export ke format Office.
5. **Delete.** `delete --id <id>` atau `delete --name <nama> --from <folder>` memindahkan file ke trash Drive. `--permanent` menghapus permanen (lewat API `Files.Delete`), dan ditolak untuk folder.
6. **Prune.** `prune --from <folder> --keep N [--match glob]` memindahkan semua kecuali N file terbaru ke trash.
7. **Share.** `share --id/--name` memberi izin "anyone with the link" (reader) dan mencetak link; `--revoke` mencabutnya.
8. **Audit.** Setiap command data (push, list, fetch, delete, prune, share) menambah tepat satu baris ke `logs/audit.jsonl` di samping exe, termasuk saat gagal; `logs [--limit N]` menampilkan entri terbaru lebih dulu.

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
- **Folder path**: path berpemisah `/` di bawah root folder, misalnya `Releases/Prod`. Di-resolve per segmen berdasarkan nama oleh `ResolveFolderPath`. Kalau ada beberapa folder dengan nama sama, yang paling lama dibuat yang dipakai.
- **Push / fetch**: upload / download satu file.
- **Replace**: `push --replace` meng-upload file baru, lalu memindahkan file lama bernama sama di folder tujuan ke trash.
- **Trash**: default untuk `delete`, `prune`, dan `--replace`. File bisa di-restore dari trash Drive selama 30 hari.
- **Creds**: file JSON credential, berupa OAuth client (`installed`/`web`) atau service account (`type: service_account`). Tipe dideteksi dari isinya.
- **Audit event**: satu record JSON di audit log dengan `action` (nama command) dan `result` (`success`/`failed`).

## Status

Berjalan: semua command di `README.md`, auth OAuth dan service account, build release dengan versi dari `git describe`, test dengan Drive palsu (`task test`), CI di GitHub Actions (`windows-latest`).

Semua akses Drive sudah lewat `internal/gdrive`, audit dicatat sekali per run, dan context dari cobra (Ctrl+C membatalkan request).

Masalah yang diketahui:

- Token OAuth disimpan relatif ke working directory, jadi script yang jalan dari folder lain tidak menemukan token dan membuka OAuth flow. Memindah default ke `%AppData%` butuh ADR.
- `push --replace` membuat file baru (ID dan link share berubah). Alternatifnya `Files.Update` pada ID yang sama; butuh ADR.
- Semua error keluar dengan exit code 1. Exit code berbeda per jenis error adalah kontrak baru untuk script; butuh ADR.
- `delete --name` memindahkan **semua** file dengan nama itu ke trash.
- Dua `push --mkdir` paralel bisa sempat membuat folder kembar; yang kalah dipindah ke trash setelah dibuat.
