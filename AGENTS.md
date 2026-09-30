# AGENTS.md

save-drive adalah CLI Go (cobra) untuk push, fetch, list, dan delete file di Google Drive, dipanggil dari script build/release di Windows. Satu binary: tanpa server, tanpa database.

## Perintah

Semua perintah ada di `Taskfile.yml` (jalankan `task` untuk daftarnya). Yang punya jebakan:

- `task fmt` dan `task check` menjalankan `gofmt -w .`, jadi file di working tree ditulis ulang.
- `task clean` menghapus `bin/` dan `dist/`.
- Argumen `task release` memakai gaya parameter PowerShell: `task release -- -Version 1.2.0`.
- Verifikasi perubahan dengan `task test` (Drive palsu, tanpa credential dan jaringan) dan `go vet ./...`.
- Command selain `logs`, `logout`, dan `version` bicara ke Drive milik user. Kalau token belum ada, command membuka OAuth flow di browser dan menunggu 3 menit. Minta izin user sebelum menjalankan command Drive terhadap Drive asli.
- Token asli user ada di `.save-drive/oauth-token.json` di root repo. Sebelum smoke test binary, arahkan `GOOGLE_OAUTH_TOKEN_FILE` dan `SAVE_DRIVE_LOG_FILE` ke path sementara; `logout` dan `authorize` menghapus atau menimpa token.

## Aturan

- Satu file per command di `internal/cli/`, dengan bentuk `xOptions` + `newXCmd()` + `runX(opts)`. Daftarkan command baru di `newRootCmd()` (`internal/cli/root.go`).
- Env var baru: konstanta `EnvX` + fungsi `DefaultX()` di `internal/config/config.go`, lalu tambahkan ke teks `Long` di `root.go`.
- Stdout dibaca oleh script. `list` dan `logs` mencetak TSV, status berupa satu baris huruf kecil (`uploaded: ...`), `--json` mencetak JSON. Pesan untuk manusia ke stderr. Pertahankan format ini kecuali user minta diubah; kolom TSV baru ditambah di akhir.
- Field JSON `audit.Event` bersifat tetap: file log lama harus tetap terbaca oleh `logs`. Field baru boleh ditambah dengan `omitempty`.
- Error dikembalikan ke atas, di-wrap dengan `fmt.Errorf("konteks: %w", err)`, pesan huruf kecil dalam bahasa Inggris. Hanya `cmd/save-drive/main.go` yang mencetak error ke stderr.
- Komentar kode dan pesan commit dalam bahasa Inggris. Commit: satu baris imperatif (`Add Task runner and PowerShell build scripts`).
- **JANGAN** pakai co-author / trailer apa pun. Commit HANYA saat user minta.
- Token OAuth disimpan relatif ke working directory (`.save-drive/oauth-token.json`), sedangkan audit log ada di samping exe (`logs/audit.jsonl`).
- File credential disimpan di luar repo (README memakai `D:\keys\`). `.gitignore` hanya menutup `service-account*.json`, OAuth client JSON tidak.

Pola yang diadopsi (detail di `documents/CODE-PATTERNS.md`):

- Semua panggilan Drive API ada di `internal/gdrive`; `internal/cli` tidak mengimpor `google.golang.org/api/drive/v3`.
- Satu audit event per run, dicatat lewat `defer a.record(&e, &err)`.
- Context diambil dari `cmd.Context()`.
- Test gdrive memakai Drive palsu di `internal/gdrive/fake_test.go`.
- Penghapusan default ke trash; hapus permanen hanya lewat `--permanent`. Pencarian by name tidak pernah mengembalikan folder.

## Dokumen

- Sebelum mengubah perilaku command atau menambah fitur, baca `documents/OVERVIEW.md`.
- Sebelum memindah kode antar package atau menyentuh auth, baca `documents/ARCHITECTURE.md`.
- Sebelum menulis atau me-review kode, baca `documents/CODE-PATTERNS.md`.
- Saat mengambil keputusan yang sulit dibalik, baca `documents/ADR/README.md`.
- `PROJECT.md` adalah spesifikasi awal dari pemilik project; `README.md` adalah panduan setup untuk pengguna.
