# Panduan Deploy — dari nol

Panduan ini untuk server yang **hanya punya Docker**. Setelah selesai, setiap
`git push` ke `main` otomatis: test → build image → kirim ke server → restart.

Yang dibutuhkan:

- Server Linux dengan Docker + Docker Compose plugin, dan akses SSH
- Satu domain/subdomain yang bisa diarahkan ke IP server (mis. `api.domain-anda.com`)

Cek dulu plugin compose-nya ada:

```bash
docker compose version    # harus keluar "Docker Compose version v2.x"
```

Kalau perintah itu error, yang terpasang Compose v1 yang sudah usang. Pasang
plugin-nya dulu (`apt install docker-compose-plugin` di Debian/Ubuntu).

---

## Bagian 1 — Arahkan domain

Di panel DNS domain Anda, buat satu record:

| Type | Name | Value |
|------|------|-------|
| A    | api  | IP server Anda |

Tunggu sampai propagasi selesai, lalu pastikan sudah benar:

```bash
dig +short api.domain-anda.com     # harus mengembalikan IP server
```

**Jangan lanjut sebelum langkah ini benar.** Caddy meminta sertifikat HTTPS ke
Let's Encrypt dengan cara membuktikan bahwa domain itu menunjuk ke server ini.
Kalau DNS belum jadi, penerbitan sertifikat gagal berulang kali.

---

## Bagian 2 — Siapkan server

Semua perintah di bagian ini dijalankan **di server**, lewat SSH.

### 2.1 Buat user khusus deploy

Jangan memakai `root` untuk deploy.

```bash
sudo adduser --disabled-password --gecos "" deploy
sudo usermod -aG docker deploy
sudo su - deploy
```

> Menambahkan user ke grup `docker` setara dengan memberi akses root ke server.
> Itu wajar untuk user deploy khusus, tapi jangan lakukan ke akun harian Anda.

### 2.2 Siapkan folder aplikasi

Masih sebagai user `deploy`:

```bash
mkdir -p ~/finance-backend
cd ~/finance-backend
```

Ambil dua berkas yang dibutuhkan dari repo:

```bash
BASE=https://raw.githubusercontent.com/Supramegod/personal-finance-tracker-backend/main
curl -fsSL -o docker-compose.prod.yml $BASE/docker-compose.prod.yml
curl -fsSL -o Caddyfile               $BASE/Caddyfile
```

### 2.3 Buat berkas `.env`

Ini satu-satunya tempat rahasia disimpan. Berkas ini **tidak pernah** masuk
Git dan tidak dikirim GitHub Actions — Anda membuatnya manual, sekali saja.

Generate nilainya dulu:

```bash
echo "DB pass : $(openssl rand -base64 24)"
echo "JWT     : $(openssl rand -base64 48)"
echo "Admin   : $(openssl rand -base64 18)"
```

Lalu buat berkasnya (`nano .env`), isi dengan nilai hasil generate di atas:

```dotenv
# ---- Domain ----
API_DOMAIN=api.domain-anda.com

# ---- Database ----
POSTGRES_USER=finance
POSTGRES_PASSWORD=<DB pass hasil generate>
POSTGRES_DB=finance_tracker

# ---- Auth (WAJIB, aplikasi menolak start kalau kosong) ----
JWT_SECRET=<JWT hasil generate, minimal 32 karakter>
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h

# ---- Admin pertama ----
ADMIN_EMAIL=email-anda@domain.com
ADMIN_PASSWORD=<Admin hasil generate>

# ---- Server ----
APP_ENV=production
LOG_LEVEL=info
PORT=8080
SWAGGER_HOST=api.domain-anda.com

# ---- CORS: origin frontend, BUKAN domain API ----
CORS_ORIGINS=https://app.domain-anda.com

RATE_LIMIT_PER_MINUTE=60
```

Kunci izin aksesnya:

```bash
chmod 600 .env
```

> `DATABASE_URL` sengaja tidak ditulis di sini — `docker-compose.prod.yml` yang
> menyusunnya dari `POSTGRES_*` di atas, supaya password tidak ditulis dua kali
> dan tidak mungkin beda.

### 2.4 Buka firewall

Hanya port 80 dan 443. Port database dan API tidak dipublikasikan sama sekali
oleh compose produksi, jadi tidak perlu dibuka.

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

> Peringatan: UFW **tidak** memblokir port yang dipublikasikan Docker — Docker
> menulis aturan iptables sendiri yang dilewati UFW. Karena itu keamanan di sini
> bertumpu pada tidak adanya bagian `ports:` di service `db` dan `api`. Jangan
> menambahkannya "sebentar buat ngetes".

---

## Bagian 3 — Kunci SSH untuk GitHub Actions

Buat kunci **di laptop Anda** (bukan di server), tanpa passphrase karena akan
dipakai otomatis:

```bash
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/finance_deploy -N ""
```

Kirim kunci publiknya ke server:

```bash
ssh-copy-id -i ~/.ssh/finance_deploy.pub deploy@IP-SERVER
```

Uji dulu bahwa kuncinya berfungsi:

```bash
ssh -i ~/.ssh/finance_deploy deploy@IP-SERVER "docker ps"
```

Kalau perintah itu berhasil tanpa minta password, kunci sudah benar.

---

## Bagian 4 — Setelan di GitHub

### 4.1 Daftarkan secret

Buka repo → **Settings** → **Secrets and variables** → **Actions** →
**New repository secret**. Buat tiga secret:

| Nama | Isi |
|------|-----|
| `SSH_HOST` | IP server |
| `SSH_USER` | `deploy` |
| `SSH_KEY` | seluruh isi `~/.ssh/finance_deploy` (kunci **privat**) |

Untuk `SSH_KEY`, salin apa adanya termasuk baris pertama dan terakhir:

```bash
cat ~/.ssh/finance_deploy
```

Harus terlihat seperti `-----BEGIN OPENSSH PRIVATE KEY-----` sampai
`-----END OPENSSH PRIVATE KEY-----`. Jangan ada baris yang terpotong.

Kalau port SSH bukan 22, tambahkan juga secret `SSH_PORT`.

### 4.2 Izinkan Actions menulis package

Repo → **Settings** → **Actions** → **General** → bagian
**Workflow permissions** → pilih **Read and write permissions** → Save.

Tanpa ini, langkah push image ke GHCR gagal dengan error `denied: permission_denied`.

### 4.3 Login registry di server

Image di GHCR bersifat privat secara default, jadi server perlu izin untuk
menariknya. Pilih salah satu:

**Cara A — jadikan package publik (paling mudah).**
Setelah workflow pertama selesai, buka profil GitHub Anda → tab **Packages** →
pilih `personal-finance-tracker-backend` → **Package settings** → **Change
visibility** → Public. Server tidak perlu login sama sekali.
Kode Anda tetap privat kalau repo-nya privat — yang publik hanya image-nya.

**Cara B — tetap privat, server login pakai token.**
Buat Personal Access Token (classic) dengan scope `read:packages` di
<https://github.com/settings/tokens>, lalu di server:

```bash
echo "TOKEN-ANDA" | docker login ghcr.io -u Supramegod --password-stdin
```

---

## Bagian 5 — Jalankan pertama kali

Deploy pertama dilakukan manual supaya kalau ada yang salah, error-nya terlihat
langsung. Di server:

```bash
cd ~/finance-backend
export API_IMAGE=ghcr.io/supramegod/personal-finance-tracker-backend:latest
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps
```

Pantau log sampai muncul banner ASCII dan `Server starting on port 8080`:

```bash
docker compose -f docker-compose.prod.yml logs -f api
```

Uji dari luar:

```bash
curl https://api.domain-anda.com/health    # {"status":"ok"}
curl https://api.domain-anda.com/ready     # {"status":"ready"} — ikut cek DB
```

Sertifikat HTTPS butuh 10–30 detik saat pertama. Kalau `curl` gagal, lihat
log Caddy: `docker compose -f docker-compose.prod.yml logs caddy`.

Uji login dengan kredensial dari `.env`:

```bash
curl -X POST https://api.domain-anda.com/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"email-anda@domain.com","password":"ADMIN_PASSWORD-anda"}'
```

Setelah ini berhasil, deploy berikutnya cukup `git push` — GitHub Actions yang
mengerjakan sisanya.

---

## Operasional harian

```bash
cd ~/finance-backend

# Lihat status & log
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f api

# Restart
docker compose -f docker-compose.prod.yml restart api

# Tugas admin (list user, cleanup token)
docker exec finance-api ./setup -list-users
docker exec finance-api ./setup -cleanup-tokens
```

### Backup database

**Lakukan ini sebelum menganggap deploy selesai.** Tidak ada backup otomatis.

```bash
docker exec finance-db pg_dump -U finance finance_tracker \
  > ~/backup-$(date +%F).sql
```

Otomatiskan harian lewat cron (`crontab -e`):

```cron
0 2 * * * docker exec finance-db pg_dump -U finance finance_tracker > ~/backups/finance-$(date +\%F).sql
```

Salin hasil backup ke luar server secara berkala — backup yang hanya ada di
server yang sama tidak menolong kalau servernya hilang.

Restore:

```bash
cat backup-2026-07-24.sql | docker exec -i finance-db psql -U finance -d finance_tracker
```

### Rollback ke versi sebelumnya

Setiap build diberi tag SHA commit, jadi versi lama tetap tersedia:

```bash
cd ~/finance-backend
export API_IMAGE=ghcr.io/supramegod/personal-finance-tracker-backend:sha-<SHA-LAMA>
docker compose -f docker-compose.prod.yml up -d api
```

SHA bisa dilihat di riwayat commit GitHub atau di tab Packages.

---

## Kalau bermasalah

| Gejala | Penyebab yang paling sering |
|--------|------------------------------|
| Actions gagal di `docker push`, `denied` | Workflow permissions belum diset ke Read and write (bagian 4.2) |
| Server gagal `pull`, `unauthorized` | Package masih privat dan server belum login (bagian 4.3) |
| `repository name must be lowercase` | Nama image ditulis dengan huruf kapital — workflow sudah menanganinya, jangan hardcode manual |
| API restart terus | `.env` kurang variabel wajib. Cek log: pesannya menyebut nama variabel yang kosong |
| HTTPS tidak jadi | DNS belum mengarah ke server, atau port 80 tertutup. Let's Encrypt perlu port 80 |
| Frontend kena CORS | `CORS_ORIGINS` harus berisi domain **frontend** (`https://app...`), bukan domain API |
| `/health` OK tapi `/ready` gagal | API hidup tapi database tidak terjangkau. Cek `docker compose logs db` |

Melihat variabel yang terbaca container (nilai rahasia akan ikut tampil,
jangan dilakukan saat berbagi layar):

```bash
docker exec finance-api env | sort
```

---

## Catatan tentang Kubernetes

Folder `k8s/` di repo ini untuk deployment ke cluster Kubernetes, dan **tidak
dipakai** dalam panduan ini. Server dengan Docker saja sudah cukup untuk
aplikasi single-user seperti ini. Manifest itu berguna nanti kalau memang
pindah ke cluster — abaikan saja untuk sekarang.
