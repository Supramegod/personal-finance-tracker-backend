package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AIConsent struct {
	Enabled   bool       `json:"enabled"`
	ConsentAt *time.Time `json:"consent_at,omitempty"`
	ConsentBy *string    `json:"consent_by,omitempty"`
	CanManage bool       `json:"can_manage"`
	Available bool       `json:"available"`
}

type InsightTransaction struct {
	Date     string  `json:"date"`
	Type     string  `json:"type"`
	Amount   float64 `json:"amount"`
	Category string  `json:"category"`
}

type AIInsight struct {
	ID            string          `json:"id"`
	GroupID       string          `json:"-"`
	PeriodMonth   time.Time       `json:"-"`
	Status        string          `json:"status"`
	Facts         json.RawMessage `json:"facts"`
	Analysis      json.RawMessage `json:"analysis,omitempty"`
	Model         string          `json:"model"`
	PromptVersion string          `json:"-"`
	SourceHash    string          `json:"-"`
	ErrorMessage  *string         `json:"error,omitempty"`
	GeneratedAt   *time.Time      `json:"generated_at,omitempty"`
	UpdatedAt     time.Time       `json:"-"`
}

type AIInsightRepository struct{ pool *pgxpool.Pool }

func NewAIInsightRepository(pool *pgxpool.Pool) *AIInsightRepository {
	return &AIInsightRepository{pool: pool}
}

func (r *AIInsightRepository) GetConsent(groupID, userID string) (*AIConsent, error) {
	var c AIConsent
	err := r.pool.QueryRow(context.Background(), `
		SELECT g.ai_insights_enabled, g.ai_consent_at, g.ai_consent_by,
		       EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.id AND gm.user_id=$2 AND gm.role='owner')
		FROM groups g WHERE g.id=$1
		  AND EXISTS(SELECT 1 FROM group_members member WHERE member.group_id=g.id AND member.user_id=$2)`, groupID, userID).
		Scan(&c.Enabled, &c.ConsentAt, &c.ConsentBy, &c.CanManage)
	return &c, err
}

func (r *AIInsightRepository) SetConsent(groupID, userID string, enabled bool) (*AIConsent, error) {
	role, err := r.roleOf(groupID, userID)
	if err != nil {
		return nil, err
	}
	if role != "owner" {
		return nil, fmt.Errorf("only owner can manage AI consent")
	}
	_, err = r.pool.Exec(context.Background(), `
		UPDATE groups SET ai_insights_enabled=$3,
		ai_consent_at=CASE WHEN $3 THEN NOW() ELSE NULL END,
		ai_consent_by=CASE WHEN $3 THEN $2::uuid ELSE NULL END
		WHERE id=$1`, groupID, userID, enabled)
	if err != nil {
		return nil, err
	}
	return r.GetConsent(groupID, userID)
}

func (r *AIInsightRepository) roleOf(groupID, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(context.Background(), `SELECT role FROM group_members WHERE group_id=$1 AND user_id=$2`, groupID, userID).Scan(&role)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return role, err
}

func (r *AIInsightRepository) EnabledGroups() ([]string, error) {
	rows, err := r.pool.Query(context.Background(), `SELECT id FROM groups WHERE ai_insights_enabled=true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *AIInsightRepository) Transactions(groupID string, from, to time.Time) ([]InsightTransaction, error) {
	rows, err := r.pool.Query(context.Background(), `
		SELECT t.transaction_date, t.type, t.amount, c.name
		FROM transactions t JOIN categories c ON c.id=t.category_id
		WHERE t.group_id=$1 AND t.deleted_at IS NULL AND t.transaction_date >= $2 AND t.transaction_date < $3
		ORDER BY t.transaction_date`, groupID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []InsightTransaction{}
	for rows.Next() {
		var item InsightTransaction
		var date time.Time
		if err := rows.Scan(&date, &item.Type, &item.Amount, &item.Category); err != nil {
			return nil, err
		}
		item.Date = date.Format("2006-01-02")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AIInsightRepository) Get(groupID string, month time.Time) (*AIInsight, error) {
	return r.scan(r.pool.QueryRow(context.Background(), `
		SELECT i.id, i.group_id, i.period_month, i.status, i.facts, i.analysis, i.model, i.prompt_version,
		       i.source_hash, i.error_message, i.generated_at, i.updated_at
		FROM financial_ai_insights i
		JOIN groups g ON g.id=i.group_id AND g.ai_insights_enabled=true
		WHERE i.group_id=$1 AND i.period_month=$2`, groupID, month))
}

func (r *AIInsightRepository) Latest(groupID string) (*AIInsight, error) {
	return r.scan(r.pool.QueryRow(context.Background(), `
		SELECT i.id, i.group_id, i.period_month, i.status, i.facts, i.analysis, i.model, i.prompt_version,
		       i.source_hash, i.error_message, i.generated_at, i.updated_at
		FROM financial_ai_insights i
		JOIN groups g ON g.id=i.group_id AND g.ai_insights_enabled=true
		WHERE i.group_id=$1 ORDER BY i.period_month DESC LIMIT 1`, groupID))
}

func (r *AIInsightRepository) Claim(groupID string, month time.Time, facts json.RawMessage, model, promptVersion, hash string) (bool, error) {
	result, err := r.pool.Exec(context.Background(), `
		INSERT INTO financial_ai_insights(group_id, period_month, status, facts, model, prompt_version, source_hash)
		VALUES($1,$2,'processing',$3,$4,$5,$6)
		ON CONFLICT(group_id, period_month) DO UPDATE SET status='processing', facts=EXCLUDED.facts,
		model=EXCLUDED.model, prompt_version=EXCLUDED.prompt_version, source_hash=EXCLUDED.source_hash,
		error_message=NULL, updated_at=NOW()
		WHERE financial_ai_insights.status IN ('failed','pending')
		   OR financial_ai_insights.source_hash <> EXCLUDED.source_hash
		   OR financial_ai_insights.model <> EXCLUDED.model
		   OR financial_ai_insights.prompt_version <> EXCLUDED.prompt_version`,
		groupID, month, facts, model, promptVersion, hash)
	return result.RowsAffected() == 1, err
}

func (r *AIInsightRepository) Complete(groupID string, month time.Time, analysis json.RawMessage) error {
	_, err := r.pool.Exec(context.Background(), `UPDATE financial_ai_insights SET status='completed', analysis=$3,
		error_message=NULL, generated_at=NOW(), updated_at=NOW() WHERE group_id=$1 AND period_month=$2`, groupID, month, analysis)
	return err
}

func (r *AIInsightRepository) Fail(groupID string, month time.Time, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := r.pool.Exec(context.Background(), `UPDATE financial_ai_insights SET status='failed', error_message=$3,
		updated_at=NOW() WHERE group_id=$1 AND period_month=$2`, groupID, month, message)
	return err
}

type rowScanner interface{ Scan(...any) error }

func (r *AIInsightRepository) scan(row rowScanner) (*AIInsight, error) {
	var i AIInsight
	err := row.Scan(&i.ID, &i.GroupID, &i.PeriodMonth, &i.Status, &i.Facts, &i.Analysis, &i.Model,
		&i.PromptVersion, &i.SourceHash, &i.ErrorMessage, &i.GeneratedAt, &i.UpdatedAt)
	return &i, err
}
