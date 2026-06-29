# Project

Project ini simple aja, berbentuk CLI yang akan masuk ke-path.

Inti dari project ini adalah me-manage project gw di google drive agar release bisa diupdate dengan mudah hanya dengan cli saja.

Fitur:

- OAuth authorize command untuk login awal dan simpan token lokal
- Upload file ke google drive
- Bikin folder di google drive
- fetch file ke google drive
- Replace file di google drive
- Delete file di google drive
- Harus bisa kasih tahu list folder dan isinya

Kebutuhan auth:

- Google OAuth client JSON untuk akun personal (utama)
- optional service account JSON kalau memang dipakai
- token OAuth lokal supaya flow build/release tidak berhenti minta login terus

Catatan automation:

- kalau CLI dipanggil dari script yang menangkap output command, URL OAuth bisa tidak kelihatan di terminal
- karena itu perlu command `authorize` yang bisa dijalankan manual lebih dulu

menggunakan bahasa golang.
