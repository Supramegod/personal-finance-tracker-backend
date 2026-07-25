# Panduan Deploy

Arsitektur: GitHub membangun image → dorong ke GitHub Container Registry (GHCR).
Server (PC rumah) menarik image itu dan menjalankan Docker sendiri. Akses publik
lewat Cloudflare Tunnel, jadi tidak butuh IP publik maupun buka port.

```
push ke main
    │
    ▼
GitHub Actions:  test → build image → push ke ghcr.io   (otomatis)
    │
    ▼
PC server:  docker compose pull && up -d                 (manual, oleh Anda/teman)
    │
    ▼
Cloudflare Tunnel  →  publik: https://api.domain-anda.com
```

Kenapa begini: PC rumah di IndiHome memakai CGNAT — koneksi masuk dari internet
(SSH, HTTP) tidak bisa menembus. Maka GitHub tidak "mendorong" ke server;
sebaliknya server yang menarik, dan Cloudflare Tunnel yang membuka jalan publik
lewat koneksi keluar.

---

## Bagian 1 — Setelan GitHub (sekali)

Supaya build bisa mendorong image ke GHCR:

Repo → **Settings** → **Actions** → **General** → **Workflow permissions** →
pilih **Read and write permissions** → **Save**.

Tanpa ini, langkah push image gagal dengan `denied: permission_denied`.

Setelah itu setiap `git push` ke `main` otomatis membuat image baru. Tidak ada
secret yang perlu dibuat.

---

## Bagian 2 — Agar server bisa menarik image

Image di GHCR privat secara default. Dua pilihan:

**Cara A — jadikan image publik (paling mudah).**
Setelah build pertama selesai: profil GitHub → tab **Packages** →
`personal-finance-tracker-backend` → **Package settings** → **Change
visibility** → **Public**. Server tidak perlu login. Kode tetap privat kalau
repo privat — yang publik hanya image-nya.

**Cara B — tetap privat, server login pakai token.**
Buat Personal Access Token (classic) scope `read:packages` di
<https://github.com/settings/tokens>, lalu di PC server:

```bash
echo "TOKEN" | docker login ghcr.io -u Supramegod --password-stdin
```

---

## Bagian 3 — Siapkan Cloudflare Tunnel

Bagian ini biasanya dikerjakan orang yang memegang domain (teman Anda). Hasil
akhir yang dibutuhkan cuma satu: **TUNNEL_TOKEN**.

1. Punya domain, dan tambahkan domainnya ke Cloudflare (gratis) —
   <https://dash.cloudflare.com> → Add a site → ikuti pengarahan ganti
   nameserver.
2. Buka **Zero Trust** → **Networks** → **Tunnels** → **Create a tunnel** →
   pilih **Cloudflared** → beri nama (mis. `finance`).
3. Cloudflare menampilkan token panjang (`eyJ...`). **Salin token ini** — inilah
   `TUNNEL_TOKEN`. Abaikan perintah instalasi yang ditawarkan; token saja cukup
   karena cloudflared kita jalankan lewat Docker.
4. Di tab **Public Hostname** tunnel itu → **Add a public hostname**:
   - Subdomain: `api` (jadi `api.domain-anda.com`)
   - Service: **HTTP** → `api:8080`
     (nama `api` = nama container di compose, port 8080 = port aplikasi)
5. Simpan.

> Service-nya `http://api:8080`, bukan `localhost` — cloudflared berjalan di
> dalam network Docker yang sama dengan API, jadi menjangkaunya lewat nama
> container. TLS/HTTPS ditangani Cloudflare di edge; aplikasi tetap HTTP di
> belakang tunnel.

---

## Bagian 4 — Jalankan di PC server

Semua perintah di PC server. Pastikan Docker + plugin compose ada:

```bash
docker compose version    # harus "Docker Compose version v2.x"
```

Siapkan direktori kerja:

```bash
mkdir -p ~/finance-backend && cd ~/finance-backend
curl -fsSL -o docker-compose.prod.yml \
  https://raw.githubusercontent.com/Supramegod/personal-finance-tracker-backend/main/docker-compose.prod.yml
```

Buat `.env`. Generate rahasianya dulu:

```bash
echo "DB   : $(openssl rand -base64 24)"
echo "JWT  : $(openssl rand -base64 48)"
echo "ADMIN: $(openssl rand -base64 18)"
```

`nano .env`, isi bagian `<...>`:

```dotenv
# Image yang dijalankan. :latest = build terakhir. Untuk versi spesifik,
# ganti dengan tag sha-<commit> dari tab Packages.
API_IMAGE=ghcr.io/supramegod/personal-finance-tracker-backend:latest

# Token dari Bagian 3
TUNNEL_TOKEN=<token cloudflare eyJ...>

# Database
POSTGRES_USER=finance
POSTGRES_PASSWORD=<DB>
POSTGRES_DB=finance_tracker

# Auth (WAJIB — aplikasi menolak start kalau kosong)
JWT_SECRET=<JWT, minimal 32 karakter>
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h

# Admin pertama
ADMIN_EMAIL=email-anda@gmail.com
ADMIN_PASSWORD=<ADMIN>

# Server
APP_ENV=production
LOG_LEVEL=info
PORT=8080
SWAGGER_HOST=api.domain-anda.com

# CORS: origin FRONTEND, bukan domain API
CORS_ORIGINS=https://app.domain-anda.com

RATE_LIMIT_PER_MINUTE=60
```

Simpan (`Ctrl+O`, Enter, `Ctrl+X`), kunci izinnya:

```bash
chmod 600 .env
```

Jalankan:

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps
```

Pantau sampai API siap:

```bash
docker compose -f docker-compose.prod.yml logs -f api      # tunggu "Server starting on port 8080"
docker compose -f docker-compose.prod.yml logs -f cloudflared   # tunggu "Registered tunnel connection"
```

Uji dari mana saja:

```bash
curl https://api.domain-anda.com/health    # {"status":"ok"}
curl https://api.domain-anda.com/ready     # {"status":"ready"}
```

---

## Update ke versi baru

Setiap `git push` ke `main` menghasilkan image baru otomatis. Untuk menerapkannya
di server:

```bash
cd ~/finance-backend
docker compose -f docker-compose.prod.yml pull api
docker compose -f docker-compose.prod.yml up -d
docker image prune -f
```

Kalau mau otomatis tanpa mengetik, jalankan itu lewat cron (`crontab -e`), mis.
tiap 10 menit, atau pasang [Watchtower]. Tapi untuk single-user, manual saat mau
update sudah cukup.

## Rollback

Setiap build punya tag `sha-<commit>`. Untuk balik ke versi lama, ubah `API_IMAGE`
di `.env` ke tag SHA yang diinginkan (lihat tab Packages), lalu:

```bash
docker compose -f docker-compose.prod.yml up -d api
```

## Backup database

Tidak ada backup otomatis. Lakukan sebelum menganggap ini beres:

```bash
docker exec finance-db pg_dump -U finance finance_tracker > ~/backup-$(date +%F).sql
```

Otomatiskan harian (`crontab -e`):

```cron
0 2 * * * docker exec finance-db pg_dump -U finance finance_tracker > ~/backups/finance-$(date +\%F).sql
```

Salin hasil backup ke luar PC secara berkala — backup yang hanya ada di PC yang
sama tidak menolong kalau PC-nya rusak.

Restore:

```bash
cat backup-2026-07-25.sql | docker exec -i finance-db psql -U finance -d finance_tracker
```

---

## Operasional

```bash
cd ~/finance-backend
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f api
docker compose -f docker-compose.prod.yml restart api

# Tugas admin
docker exec finance-api ./setup -list-users
docker exec finance-api ./setup -cleanup-tokens
```

## Kalau bermasalah

| Gejala | Penyebab tersering |
|--------|--------------------|
| Actions gagal di push image, `denied` | Workflow permissions belum Read and write (Bagian 1) |
| Server gagal `pull`, `unauthorized` | Image masih privat, server belum login (Bagian 2) |
| API restart terus | `.env` kurang variabel wajib — cek log, pesannya menyebut variabel yang kosong |
| `curl` domain gagal, tapi container jalan | cloudflared belum konek / public hostname salah. Cek `logs -f cloudflared` dan pemetaan `api:8080` di dashboard |
| Frontend kena CORS | `CORS_ORIGINS` harus domain **frontend**, bukan domain API |
| `/health` OK tapi `/ready` gagal | API hidup, database tidak terjangkau. Cek `logs db` |

---

## Catatan

- **Folder `k8s/`** untuk deploy ke cluster Kubernetes — tidak dipakai di sini,
  abaikan.
- **`docker-compose.yml`** (tanpa `.prod`) untuk development di laptop:
  membangun image lokal dan tidak memakai tunnel. Yang untuk server adalah
  `docker-compose.prod.yml`.
