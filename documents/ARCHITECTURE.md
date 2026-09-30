# Arsitektur

## Stack

- Go, `go 1.25.0` di `go.mod` (versi tertinggi yang didukung toolchain 1.25; `google.golang.org/api` di atas v0.298 butuh Go 1.26). Module name: `save-drive`.
- `github.com/spf13/cobra` v1.10.2 untuk command tree.
- `google.golang.org/api` v0.298.0 (`drive/v3`) dan `golang.org/x/oauth2` v0.36.0 untuk Drive dan OAuth.
- Build lewat [Task](https://taskfile.dev) + script PowerShell di `scripts/`. Target platform: Windows (`.exe`, `rundll32`, `.ps1`).

## Komponen

```mermaid
flowchart LR
    script[Script build / terminal] --> cli[save-drive.exe]
    cli --> drive[(Google Drive API)]
    cli --> oauth[Google OAuth]
    oauth -. callback 127.0.0.1:port acak .-> cli
    cli --> token[.save-drive/oauth-token.json<br/>relatif ke cwd]
    cli --> log[logs/audit.jsonl<br/>di samping exe]
    creds[creds JSON<br/>GOOGLE_OAUTH_CLIENT_CREDENTIALS] --> cli
```

Satu proses per pemanggilan. Tidak ada daemon, job, atau antrean. Server HTTP hanya hidup selama OAuth flow (maksimal 3 menit) di `internal/gdrive/auth.go`.

## Satu request: `push`

`save-drive push --file .\release.zip --to Releases/Prod --mkdir --replace`

1. `cmd/save-drive/main.go` memanggil `cli.Execute()`, yang membuat context yang dibatalkan Ctrl+C. Error apa pun dicetak sebagai `error: ...` ke stderr dengan exit code 1.
2. `internal/cli/root.go` merutekan ke `newPushCmd()`. Default flag `--creds` dan `--root-id` diambil dari `internal/config` saat command dibuat.
3. `runPush` di `internal/cli/push.go` mengekspansi `--file` dan memastikan setiap file lokal ada, **sebelum** menyentuh Drive.
4. `newApp` (`internal/cli/app.go`) membuat `gdrive.Client` dan `audit.Logger`, lalu `defer a.record(&e, &err)` memastikan satu audit event tercatat apa pun hasilnya.
5. `gdrive.New` (`internal/gdrive/client.go`) membaca creds JSON, mendeteksi tipenya, lalu untuk OAuth memuat token dari file atau menjalankan `tokenFromWeb` (`internal/gdrive/auth.go`). HTTP client dibungkus `retryTransport` (`internal/gdrive/retry.go`).
6. `ResolveFolderPath` (`internal/gdrive/folders.go`) menelusuri `Releases`, lalu `Prod`, dengan satu query `Files.List` per segmen (folder tertua menang), dan membuat folder yang belum ada.
7. `runPush` mengecek semua nama dengan `FindFiles`; tanpa `--replace`, nama yang bentrok menghentikan run sebelum ada upload.
8. `Client.Upload` (`internal/gdrive/files.go`) meng-upload lewat `Files.Create(...).Media(...)`, membandingkan `md5Checksum` dengan MD5 lokal, lalu memindah file lama ke trash.
9. stdout mencetak `uploaded: release.zip (id=...)`; `record` menulis event ke audit log.

## Layer dan import

| Package | Boleh mengimpor | Isi |
|---------|-----------------|-----|
| `cmd/save-drive` | `internal/cli` | `main` saja |
| `internal/cli` | `gdrive`, `audit`, `config`, `buildinfo` | flag, validasi input, pemanggilan gdrive, audit, output |
| `internal/gdrive` | `config` | auth, semua panggilan Drive API |
| `internal/audit` | stdlib | tulis dan baca JSONL |
| `internal/config` | stdlib | nama env var, path default |
| `internal/buildinfo` | tidak ada | variabel versi untuk `-ldflags` |

`gdrive` mengimpor `config` hanya untuk `DefaultTokenFile()` di `gdrive.New`. Package di bawah `internal/` tidak mengimpor `cli`.

## Data

Tidak ada database. State ada di tiga tempat:

- **Google Drive**: file dan folder. Folder dicari berdasarkan nama, bukan ID, kecuali root.
- **Token OAuth**: JSON `oauth2.Token` di `GOOGLE_OAUTH_TOKEN_FILE`, default `.save-drive/oauth-token.json` relatif ke working directory, permission `0600`. Ditulis atomik (file `.tmp` lalu rename), dan ditulis ulang setiap kali access token di-refresh.
- **Audit log**: JSON Lines, satu `audit.Event` per baris (`internal/audit/audit.go`). Lokasi: `SAVE_DRIVE_LOG_FILE`, atau `logs/audit.jsonl` di samping exe. Baris yang tidak bisa di-parse dilewati saat dibaca. Kegagalan menulis log hanya menghasilkan warning di stderr supaya audit tidak pernah menggagalkan operasi.

## Auth

`gdrive.New` membaca creds JSON:

- `"type": "service_account"`: service account dengan scope `drive.DriveScope`. Service account tidak punya storage quota, jadi root harus berupa Shared Drive.
- Ada key `installed` atau `web`: OAuth client. Token diambil dari file; kalau file tidak ada atau JSON-nya rusak, flow web dijalankan dan token disimpan. `authorize` (`gdrive.Authorize`) selalu menjalankan flow web. Token yang ditolak Google menghasilkan error yang menyarankan `save-drive authorize`.
- Selain itu: error `unsupported credentials json`.

`--creds` default ke `GOOGLE_OAUTH_CLIENT_CREDENTIALS`, lalu `GOOGLE_APPLICATION_CREDENTIALS`.

Flow web: callback server di `127.0.0.1` dengan port acak untuk client `installed`, atau `127.0.0.1:8085` untuk client `web` (redirect URI-nya harus terdaftar persis). URL consent memakai `state` acak, PKCE (S256), `access_type=offline`, dan `prompt=consent` supaya Google selalu mengirim refresh token. Request callback dengan `state` yang salah ditolak tanpa membatalkan flow. Batas waktu 3 menit, dan Ctrl+C membatalkannya.

## Build dan deploy

- `task dev` membangun `bin/save-drive.exe` dan bisa langsung menjalankannya dengan argumen setelah `--`.
- `task release` membangun `dist/save-drive.exe` dengan `-trimpath -ldflags "-s -w -X save-drive/internal/buildinfo.Version=..."`. Versi default dari `git describe --tags --always --dirty`.
- Deploy manual: salin exe ke folder di PATH. Audit log akan ikut berada di folder itu (`logs/`).
- CI: `.github/workflows/ci.yml` di `windows-latest` menjalankan gofmt, vet, test, dan `scripts/release.ps1`. `.gitattributes` memaksa LF untuk file Go supaya gofmt tidak gagal karena CRLF di checkout Windows.

## ADR

Belum ada ADR. Aturan dan template ada di [`ADR/README.md`](ADR/README.md).
