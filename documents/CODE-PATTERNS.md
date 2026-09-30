# Pola kode

Aturan menulis kode di repo ini. Setiap bagian bertanda **tetap** (pola yang sudah dipakai kode sejak awal) atau **diadopsi** (dipilih di pass dokumentasi 2026-09-30). Migrasi ke pola yang diadopsi selesai di hari yang sama; semua kode sekarang mengikuti keduanya.

Project ini tidak punya frontend. Area frontend (routing, komponen, styling) tidak berlaku.

## Layout package (tetap)

Entry point di `cmd/save-drive/`, semua kode lain di `internal/<nama>/`, satu tanggung jawab per package. Batas import ada di [`ARCHITECTURE.md`](ARCHITECTURE.md#layer-dan-import).

Contoh: `internal/audit/audit.go`, `internal/config/config.go`.

## Command (tetap)

Satu command, satu file di `internal/cli/`. Setiap file berisi:

- struct `xOptions` untuk semua flag,
- `newXCmd() *cobra.Command` yang mendaftarkan flag ke struct itu,
- `runX(ctx context.Context, opts xOptions) (err error)` yang berisi logika. Return value diberi nama supaya `defer a.record(&e, &err)` bisa membacanya.

Daftarkan di `newRootCmd()` (`internal/cli/root.go`). Flag `--creds` dan `--root-id` memakai default dari `config`. Flag wajib memakai `cmd.MarkFlagRequired`; aturan "salah satu dari `--id` atau `--name`" divalidasi di awal `runX` dengan `errors.New`.

Contoh untuk ditiru: `internal/cli/fetch.go`, dan `internal/cli/push.go` untuk command dengan `--dry-run`. Command tanpa flag boleh ditulis inline seperti `internal/cli/version.go`.

## Config dan env var (tetap)

Nama env var berupa konstanta `EnvX` di `internal/config/config.go`, dibaca lewat fungsi `DefaultX()` yang mengembalikan default kalau kosong (`GetenvOr`). Hanya `config` yang memanggil `os.Getenv`. Setiap env var baru juga ditulis di teks `Long` di `internal/cli/root.go` dan di `README.md`.

## Akses Drive (diadopsi)

Semua panggilan Drive API ada di `internal/gdrive`. Package `internal/cli` tidak mengimpor `google.golang.org/api/drive/v3` dan tidak memanggil `Service()`.

- Aturan Drive (cek nama bentrok, replace, `SupportsAllDrives(true)`, paginasi) tinggal di method gdrive.
- cli hanya membaca flag, memanggil gdrive, mencatat audit, dan mencetak output.
- Method gdrive menerima `ctx` sebagai parameter pertama dan mengembalikan error yang di-wrap.

Method utama, di `internal/gdrive/files.go`, `folders.go`, dan `client.go`:

```go
func (c *Client) Upload(ctx context.Context, folderID, localPath string, opts UploadOptions) (*drive.File, error)
func (c *Client) Download(ctx context.Context, fileID, dst string) (*drive.File, string, error)
func (c *Client) Trash(ctx context.Context, ids ...string) error
func (c *Client) Delete(ctx context.Context, ids ...string) error // permanen
func (c *Client) List(ctx context.Context, folderID string) ([]*drive.File, error)
func (c *Client) ListTree(ctx context.Context, folderID string) ([]Entry, error)
func (c *Client) FindFiles(ctx context.Context, parentID, name string) ([]*drive.File, error)
func (c *Client) ResolveFolderPath(ctx context.Context, rootID, folderPath string, create bool) (string, error)
func (c *Client) WhoAmI(ctx context.Context) (*drive.User, error)
```

cli memakai field `*drive.File` hasil method tanpa perlu mengimpor `drive/v3`. Aturan yang dijaga gdrive:

- `Upload` dengan `Replace` meng-upload file baru dan mengecek MD5 dulu, baru memindah file lama ke trash. Upload yang gagal tidak menghilangkan file lama.
- Pencarian berdasarkan nama (`FindFiles`) tidak pernah mengembalikan folder. `Files.Delete` pada folder ikut menghapus seluruh isinya.
- Penghapusan default lewat `Trash`. `Delete` hanya dipanggil kalau user memberi `--permanent`.
- Semua list mengikuti `NextPageToken` sampai habis (`c.list`).
- Error dibungkus `ErrNotFound` / `ErrExists` supaya cli bisa memakai `errors.Is`.
- Retry untuk 429/5xx/rate limit ada di transport HTTP (`retry.go`), bukan di tiap method.

## Audit (diadopsi)

Setiap command data mencatat tepat satu `audit.Event` per run, termasuk saat gagal. `runX` mengisi event selangkah demi selangkah, dan satu `defer` setelah `newApp` mengisi `Result` dan mencatatnya:

```go
func runPush(ctx context.Context, opts pushOptions) (err error) {
	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "push", FileName: filepath.Base(opts.filePath), FolderPath: opts.to}
	defer a.record(&e, &err)
	// ...
	e.RemoteID = created.Id
	e.Detail = fmt.Sprintf("size=%d replace=%t", created.Size, opts.replace)
	return nil
}

// internal/cli/app.go
func (a *app) record(e *audit.Event, err *error) {
	e.Result = "success"
	if *err != nil {
		e.Result = "failed"
		e.Detail = (*err).Error()
	}
	a.logger.Append(*e)
}
```

- `Action` = nama command (`push`, `fetch`, `delete`, `list`). `Result` = `success` atau `failed`.
- Error validasi flag dan error saat membuat client (sebelum `defer`) tidak dicatat.
- Nama dan tag JSON field `audit.Event` tidak diubah. Field baru memakai `omitempty`.

`audit.Logger.Append` mengembalikan error; `record` mencetaknya sebagai `warning: write audit log ...` ke stderr dan tidak menggagalkan command.

## Context (diadopsi)

Context dibuat sekali di `Execute` dan dibatalkan oleh Ctrl+C. Command mengambilnya dari cobra dan meneruskannya ke `runX`, `newApp`, dan gdrive:

```go
// internal/cli/root.go
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return newRootCmd().ExecuteContext(ctx)
}

// internal/cli/push.go
RunE: func(cmd *cobra.Command, _ []string) error {
	return runPush(cmd.Context(), opts)
},
```

Pengecualian: shutdown server callback OAuth di `internal/gdrive/auth.go` memakai `context.Background()` dengan timeout, karena shutdown harus jalan meski ctx sudah batal.

## Error (tetap)

- Fungsi mengembalikan `error`; tidak ada `panic`, `log.Fatal`, atau `os.Exit` di luar `cmd/save-drive/main.go`.
- Wrap dengan konteks saat melewati batas package: `fmt.Errorf("read credentials file: %w", err)`.
- Pesan huruf kecil, bahasa Inggris, tanpa titik di akhir. Pesan untuk user menyebut flag yang harus dipakai: `"delete --name requires --from"`.
- `cmd/save-drive/main.go` satu-satunya tempat yang mencetak error (`error: ...` ke stderr, exit 1). Root command memakai `SilenceUsage` dan `SilenceErrors`.

Contoh: `internal/gdrive/client.go`.

## Output (tetap)

- Hasil ke stdout lewat `fmt.Printf`. Output list berupa TSV satu baris per item (`internal/cli/list.go`, `internal/cli/logs.go`). Di `logs`, field kosong ditulis `-`.
- Status sukses satu baris huruf kecil: `uploaded: <nama> (id=<id>)`, `downloaded: <path>`, `trashed: <id>` (atau `deleted: <id>` dengan `--permanent`), `shared: <link>`. Dry-run memakai `would <verb>: ...`.
- `--json` mencetak JSON berindentasi (struct `fileJSON` di `internal/cli/app.go`) sebagai ganti status line atau TSV.
- Pesan untuk manusia (URL OAuth, progress upload, warning) ke stderr. Progress hanya tampil kalau stderr adalah terminal.
- Nilai TSV lewat `tsvField` supaya tab atau newline di nama file tidak merusak kolom.
- Script membaca output ini, jadi kolom dan urutannya tidak diubah tanpa permintaan user. Kolom baru ditambah di akhir.

## Test (diadopsi)

- Test ditulis di `_test.go` di samping file yang dites, satu package dengan kodenya, hanya stdlib `testing`.
- Table test untuk fungsi murni: `escapeQuery`, `config.GetenvOr`/`DefaultX`, round-trip `audit.Append` + `audit.ReadAll` di `t.TempDir()`.
- Method gdrive dites lewat Drive palsu di `httptest.Server`, lewat constructor tak-ekspor yang menerima HTTP client dan endpoint. Test tidak pernah bicara ke Drive asli.
- `go test ./...` harus lulus tanpa credential dan tanpa jaringan.

```go
// internal/gdrive/client.go
func newWithHTTP(ctx context.Context, hc *http.Client, endpoint string) (*Client, error) {
	svc, err := drive.NewService(ctx, option.WithHTTPClient(hc), option.WithEndpoint(endpoint))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
}

// internal/gdrive/files_test.go
srv := httptest.NewServer(fakeDrive(t))
defer srv.Close()
c, err := newWithHTTP(ctx, srv.Client(), srv.URL+"/")
```

Drive palsu ada di `internal/gdrive/fake_test.go`. Ia mencatat setiap operasi yang mengubah data (`ops`: `create:<nama>`, `trash:<id>`, ...), sehingga test bisa mengecek urutan, misalnya upload sebelum trash. Query hanya mendukung klausa yang dibangun gdrive; klausa baru harus ditambahkan ke `matches`. Opsi `maxPage`, `badMD5`, dan `failStatus`/`failCount` dipakai untuk menguji paginasi, checksum, dan retry.

`task test` menjalankan `go test ./...` dan termasuk di `task check` serta CI.

## Penamaan (tetap)

- Package: satu kata huruf kecil (`gdrive`, `buildinfo`).
- File di `internal/cli/`: nama command (`push.go`, `logs.go`); `app.go` dan `root.go` untuk wiring.
- Tipe dan fungsi di `cli` tidak diekspor, kecuali `Execute`.
- Flag: kebab-case (`--root-id`); flag path lokal `--file`/`--to`, folder Drive `--to` (push) atau `--from` (lainnya).

## Komentar (tetap)

Komentar dalam bahasa Inggris. Setiap package punya komentar `// Package x ...` di satu file, dan setiap identifier yang diekspor punya doc comment. Komentar di dalam fungsi menjelaskan alasan, contohnya `openBrowser` di `internal/gdrive/auth.go` yang menjelaskan kenapa memakai `rundll32`.

## Pertanyaan terbuka

Tidak ada area yang ditunda di pass ini.
