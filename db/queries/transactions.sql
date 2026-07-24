-- name: ListTransactions :many
SELECT t.id, t.user_id, t.category_id, t.type, t.amount,
       t.transaction_date, t.note, t.created_at, t.updated_at,
       COALESCE(c.name, '') AS category_name
FROM transactions t
LEFT JOIN categories c ON t.category_id = c.id
WHERE t.user_id = $1
  AND t.deleted_at IS NULL
  AND (sqlc.narg('from_date')::date IS NULL OR t.transaction_date >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR t.transaction_date <= sqlc.narg('to_date')::date)
  AND (sqlc.narg('category_id')::uuid IS NULL OR t.category_id = sqlc.narg('category_id')::uuid)
  AND (sqlc.narg('type')::text IS NULL OR t.type = sqlc.narg('type')::text)
ORDER BY t.transaction_date DESC, t.created_at DESC;

-- name: CountTransactions :one
SELECT COUNT(*)
FROM transactions t
WHERE t.user_id = $1
  AND t.deleted_at IS NULL
  AND (sqlc.narg('from_date')::date IS NULL OR t.transaction_date >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR t.transaction_date <= sqlc.narg('to_date')::date)
  AND (sqlc.narg('category_id')::uuid IS NULL OR t.category_id = sqlc.narg('category_id')::uuid)
  AND (sqlc.narg('type')::text IS NULL OR t.type = sqlc.narg('type')::text);

-- name: FindTransactionByID :one
SELECT t.id, t.user_id, t.category_id, t.type, t.amount,
       t.transaction_date, t.note, t.created_at, t.updated_at,
       COALESCE(c.name, '') AS category_name
FROM transactions t
LEFT JOIN categories c ON t.category_id = c.id
WHERE t.id = $1 AND t.user_id = $2 AND t.deleted_at IS NULL;

-- name: CreateTransaction :one
INSERT INTO transactions (user_id, category_id, type, amount, transaction_date, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, category_id, type, amount, transaction_date, note, created_at, updated_at;

-- name: UpdateTransaction :one
UPDATE transactions
SET category_id = $2, type = $3, amount = $4, transaction_date = $5, note = $6
WHERE id = $1 AND user_id = $7
RETURNING id, user_id, category_id, type, amount, transaction_date, note, created_at, updated_at;

-- name: SoftDeleteTransaction :exec
UPDATE transactions
SET deleted_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;
