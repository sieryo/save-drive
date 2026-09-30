# ADR

ADR (Architecture Decision Record) mencatat satu keputusan yang memenuhi ketiga syarat ini:

- sulit dibalik,
- akan membingungkan pembaca di masa depan kalau tanpa konteks,
- hasil dari pertimbangan nyata antara beberapa alternatif.

Contoh di project ini: mengganti cobra, pindah dari folder berbasis nama ke folder berbasis ID, atau mengubah format audit log. Aturan kode sehari-hari masuk ke [`../CODE-PATTERNS.md`](../CODE-PATTERNS.md), bukan ADR.

## Aturan

- Nama file: `NNN-kebab-slug.md`, tiga digit, satu di atas nomor tertinggi yang ada. Nomor tidak pernah dipakai ulang, termasuk nomor ADR yang ditolak.
- Setiap ADR dimulai dari [`000-template.md`](000-template.md).
- Status: Diusulkan, Diterima, Ditolak, atau Digantikan oleh ADR-NNN.
- Teks keputusan ADR yang sudah Diterima tidak diubah. Keputusan baru ditulis di ADR baru, dan status ADR lama diganti menjadi "Digantikan oleh ADR-NNN". Koreksi kecil ditulis sebagai catatan revisi di bawah baris status.

## Daftar

Belum ada ADR.
