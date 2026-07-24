# Deploy Backend — Docker & Kubernetes

Manifest untuk menjalankan Personal Finance Tracker API di Kubernetes, lengkap
dengan PostgreSQL untuk lingkungan self-hosted.

```
backend/k8s/
├── namespace.yaml              Namespace finance-tracker
├── backend-config.yaml         ConfigMap — konfigurasi non-rahasia
├── backend-secret.example.yaml TEMPLATE Secret — jangan commit versi terisinya
├── postgres.yaml               Service headless + StatefulSet + PVC 5Gi
├── backend-deployment.yaml     Service + Deployment API
├── ingress.yaml                Ingress + TLS (ingress-nginx + cert-manager)
├── networkpolicy.yaml          Opsional — batasi akses ke DB dan API
└── kustomization.yaml          Entry point `kubectl apply -k`
```

## 1. Build image

Dijalankan dari folder `backend/`:

```bash
docker build -t personal-finance-tracker-api:0.1.0 .
```

Image memuat dua binary:

- `./server` — API (entrypoint default)
- `./setup` — CLI operasional: `-migrate`, `-seed`, `-cleanup-tokens`, `-list-users`

Push ke registry, lalu samakan tag-nya di `kustomization.yaml` bagian `images`.

## 2. Buat Secret

Jangan mengisi `backend-secret.example.yaml` lalu apply — file itu template.
Buat Secret langsung dari command line supaya nilainya tidak pernah tersimpan
di disk:

```bash
kubectl create namespace finance-tracker

DB_PASSWORD="$(openssl rand -base64 24)"

kubectl -n finance-tracker create secret generic postgres-secret \
  --from-literal=POSTGRES_USER=finance \
  --from-literal=POSTGRES_PASSWORD="$DB_PASSWORD" \
  --from-literal=POSTGRES_DB=finance_tracker

kubectl -n finance-tracker create secret generic backend-secret \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=ADMIN_EMAIL="admin@example.com" \
  --from-literal=ADMIN_PASSWORD="$(openssl rand -base64 24)" \
  --from-literal=DATABASE_URL="postgresql://finance:$DB_PASSWORD@postgres:5432/finance_tracker?sslmode=disable"
```

`JWT_SECRET` wajib ada — tanpa itu aplikasi `log.Fatal` saat start.
Password DB di `DATABASE_URL` harus sama persis dengan `POSTGRES_PASSWORD`.

Secret bawaan Kubernetes hanya base64, **bukan** terenkripsi. Untuk produksi
pakai Sealed Secrets, External Secrets Operator, atau SOPS.

## 3. Sesuaikan konfigurasi

Di `backend-config.yaml`, ganti nilai berikut ke domain sebenarnya:

- `CORS_ORIGINS` — origin frontend. Salah isi = browser memblokir semua request.
- `SWAGGER_HOST` — host yang dipakai Swagger UI.

Di `ingress.yaml`, ganti `api.finance.example.com` dan pastikan
`ingressClassName` serta `cluster-issuer` cocok dengan cluster Anda.

## 4. Apply

```bash
cd backend/k8s
kubectl apply -k .
kubectl -n finance-tracker rollout status deploy/finance-api
```

## Verifikasi

```bash
kubectl -n finance-tracker get pods
kubectl -n finance-tracker logs deploy/finance-api

# Uji tanpa lewat ingress
kubectl -n finance-tracker port-forward svc/finance-api 8080:80
curl localhost:8080/health   # {"status":"ok"}
curl localhost:8080/ready    # {"status":"ready"} — ikut ping database
```

## Tugas operasional

```bash
POD=$(kubectl -n finance-tracker get pod -l app.kubernetes.io/name=finance-api -o name)

kubectl -n finance-tracker exec $POD -- ./setup -list-users
kubectl -n finance-tracker exec $POD -- ./setup -migrate
kubectl -n finance-tracker exec $POD -- ./setup -cleanup-tokens
```

Backup database (StatefulSet ini tidak punya backup otomatis):

```bash
kubectl -n finance-tracker exec postgres-0 -- \
  pg_dump -U finance finance_tracker > backup-$(date +%F).sql
```

## Hal yang perlu diketahui sebelum produksi

**Jangan naikkan `replicas` di atas 1 tanpa mengubah kode lebih dulu.**
`cmd/server/main.go` memanggil `RunMigrations()` dan `SeedAdmin()` setiap kali
proses start, sedangkan `internal/repository/db.go` tidak punya tabel versi
migrasi maupun advisory lock — dua pod yang start bersamaan akan mengeksekusi
DDL yang sama secara paralel. Prasyarat untuk scale out: pindahkan migrasi ke
Job atau initContainer, dan bungkus dengan `pg_advisory_lock`. Karena itu
strategi deploy diset `Recreate`, bukan `RollingUpdate` — akan ada downtime
singkat saat rollout.

**Migrasi memakai path relatif.** `RunMigrations()` membaca `db/migrations`
relatif terhadap working directory, jadi image menyalinnya ke `/app/db/migrations`
dan `WORKDIR` diset `/app`. Kalau `WORKDIR` diubah, migrasi diam-diam tidak
menemukan file apa pun.

**PostgreSQL di sini untuk staging/self-hosted.** Tidak ada failover, tidak ada
point-in-time recovery. Untuk data finansial produksi, pertimbangkan managed
database (Neon, Supabase, RDS) — cukup ganti `DATABASE_URL` di Secret dan hapus
`postgres.yaml` dari `kustomization.yaml`. Pakai `sslmode=require`.

**Deploy ke produksi butuh persetujuan Owner** sesuai `PLAN.md`. Manifest ini
disiapkan, bukan diterapkan.
