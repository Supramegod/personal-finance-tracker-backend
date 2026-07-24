-- name: FindCategoriesByUserID :many
SELECT id, user_id, name, type, icon, is_default, created_at
FROM categories
WHERE user_id = $1
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type')::text)
ORDER BY type, name;

-- name: FindCategoryByID :one
SELECT id, user_id, name, type, icon, is_default, created_at
FROM categories
WHERE id = $1;

-- name: CreateCategory :one
INSERT INTO categories (user_id, name, type, icon, is_default)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, name, type, icon, is_default, created_at;

-- name: UpdateCategory :one
UPDATE categories
SET name = $2, icon = $3
WHERE id = $1
RETURNING id, user_id, name, type, icon, is_default, created_at;

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = $1 AND user_id = $2;
