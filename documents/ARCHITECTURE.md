# Arsitektur

## Stack

- Go, `go 1.22.7` di `go.mod`. Module name: `save-drive`.
- `github.com/spf13/cobra` v1.10.2 untuk command tree.
- `google.golang.org/api` v0.221.0 (`drive/v3`) dan `golang.org/x/oauth2` v0.26.0 untuk Drive dan OAuth.
- Build lewat [Task](https://taskfile.dev) + script PowerShell di `scripts/`. Target platform: Windows (`.exe`, `rundll32`, `.ps1`).

## Komponen

```mermaid
flowchart LR
    script[Script build / terminal] --> cli[save-drive.exe]
    cli --> drive[(Google Drive API)]
    cli --> oauth[Google OAuth]
    oauth -. callback 127.0.0.1:8085 .-> cli
    cli --> token[.save-drive/oauth-token.json<br/>relatif ke cwd]
    cli --> log[logs/audit.jsonl<br/>di samping exe]
    creds[creds JSON<br/>GOOGLE_OAUTH_CLIENT_CREDENTIALS] --> cli
```

Satu proses per pemanggilan. Tidak ada daemon, job, atau antrean. Server HTTP hanya hidup selama OAuth flow (maksimal 3 menit) di `internal/gdrive/auth.go`.

## Satu request: `push`

`save-drive push --file .\release.zip --to Releases/Prod --mkdir --replace`

1. `cmd/save-drive/main.go` memanggil `cli.Execute()`; error apa pun dicetak sebagai `error: ...` ke stderr dengan exit code 1.
2. `internal/cli/root.go` merutekan ke `newPushCmd()`. Default flag `--creds` dan `--root-id` diambil dari `internal/config` saat command dibuat.
3. `runPush` di `internal/cli/push.go` memanggil `newApp` (`internal/cli/app.go`), yang membuat `gdrive.Client` dan `audit.Logger`.
4. `gdrive.New` (`internal/gdrive/client.go`) membaca creds JSON, mendeteksi tipenya, lalu untuk OAuth memuat token dari file atau menjalankan `tokenFromWeb` (`internal/gdrive/auth.go`).
5. `ResolveFolderPath` (`internal/gdrive/folders.go`) menelusuri `Releases`, lalu `Prod`, dengan satu query `Files.List` per segmen, dan membuat folder yang belum ada.
6. `ListFilesByName` mencari file bernama sama; dengan `--replace`, `runPush` menghapusnya lewat `Service().Files.Delete`.
7. `runPush` upload lewat `Service().Files.Create(...).Media(fd)`.
8. `a.log(audit.Event{...})` menambah satu baris ke audit log; stdout mencetak `uploaded: release.zip (id=...)`.

Langkah 6 dan 7 memanggil Drive API langsung dari cli. Di pola yang diadopsi, keduanya pindah ke `gdrive.Client.Upload` (lihat [`CODE-PATTERNS.md`](CODE-PATTERNS.md#akses-drive)).

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
- **Token OAuth**: JSON `oauth2.Token` di `GOOGLE_OAUTH_TOKEN_FILE`, default `.save-drive/oauth-token.json` relatif ke working directory, permission `0600`.
- **Audit log**: JSON Lines, satu `audit.Event` per baris (`internal/audit/audit.go`). Lokasi: `SAVE_DRIVE_LOG_FILE`, atau `logs/audit.jsonl` di samping exe. Baris yang tidak bisa di-parse dilewati saat dibaca. Kegagalan menulis log diabaikan supaya audit tidak pernah menggagalkan operasi.

## Auth

`gdrive.New` membaca creds JSON:

- `"type": "service_account"`: service account dengan scope `drive.DriveScope`.
- Ada key `installed` atau `web`: OAuth client. Token diambil dari file; kalau file tidak ada atau tidak valid, flow web dijalankan dan token disimpan.
- Selain itu: error `unsupported credentials json`.

`--creds` default ke `GOOGLE_OAUTH_CLIENT_CREDENTIALS`, lalu `GOOGLE_APPLICATION_CREDENTIALS`. OAuth callback terikat ke `127.0.0.1:8085`, jadi port itu harus kosong saat authorize.

## Build dan deploy

- `task dev` membangun `bin/save-drive.exe` dan bisa langsung menjalankannya dengan argumen setelah `--`.
- `task release` membangun `dist/save-drive.exe` dengan `-trimpath -ldflags "-s -w -X save-drive/internal/buildinfo.Version=..."`. Versi default dari `git describe --tags --always --dirty`.
- Deploy manual: salin exe ke folder di PATH. Audit log akan ikut berada di folder itu (`logs/`).
- Tidak ada CI.

## ADR

Belum ada ADR. Aturan dan template ada di [`ADR/README.md`](ADR/README.md).
