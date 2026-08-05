ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS ai_insights_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS ai_consent_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS ai_consent_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS financial_ai_insights (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  period_month DATE NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
  facts JSONB NOT NULL DEFAULT '{}'::jsonb,
  analysis JSONB,
  model VARCHAR(100) NOT NULL,
  prompt_version VARCHAR(32) NOT NULL,
  source_hash VARCHAR(64) NOT NULL DEFAULT '',
  error_message TEXT,
  generated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (group_id, period_month)
);

CREATE INDEX IF NOT EXISTS idx_ai_insights_group_period
  ON financial_ai_insights(group_id, period_month DESC);
