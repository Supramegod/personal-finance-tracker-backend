-- name: GetBalance :one
SELECT COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE -amount END), 0)::float8 AS balance
FROM transactions
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetSummaryReport :many
SELECT DATE_TRUNC(sqlc.arg('date_trunc')::text, transaction_date)::date AS period,
       COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0)::float8 AS total_income,
       COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0)::float8 AS total_expense
FROM transactions
WHERE user_id = $1
  AND transaction_date >= $2::date
  AND transaction_date <= $3::date
  AND deleted_at IS NULL
GROUP BY period
ORDER BY period;

-- name: GetGrandTotals :one
SELECT COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0)::float8 AS total_income,
       COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0)::float8 AS total_expense
FROM transactions
WHERE user_id = $1
  AND transaction_date >= $2::date
  AND transaction_date <= $3::date
  AND deleted_at IS NULL;
