-- name: FindUserByEmail :one
SELECT id, email, password_hash, full_name, created_at, updated_at
FROM users
WHERE email = $1;

-- name: FindUserByID :one
SELECT id, email, password_hash, full_name, created_at, updated_at
FROM users
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, full_name)
VALUES ($1, $2, $3)
RETURNING id, email, password_hash, full_name, created_at, updated_at;

-- name: UpdateUser :one
UPDATE users
SET email = $2, full_name = $3, updated_at = NOW()
WHERE id = $1
RETURNING id, email, password_hash, full_name, created_at, updated_at;
