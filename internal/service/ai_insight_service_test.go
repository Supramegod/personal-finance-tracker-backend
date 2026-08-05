package service

import (
	"encoding/json"
	"testing"

	"personal-finance-tracker/internal/repository"
)

func TestBuildInsightFacts(t *testing.T) {
	previous := []repository.InsightTransaction{{Type: "expense", Amount: 100}}
	items := []repository.InsightTransaction{
		{Type: "income", Amount: 1000, Category: "Gaji"},
		{Type: "expense", Amount: 300, Category: "Makan"},
		{Type: "expense", Amount: 100, Category: "Transportasi"},
	}
	facts := buildFacts(items, previous)
	if facts.TotalIncome != 1000 || facts.TotalExpense != 400 || facts.Net != 600 {
		t.Fatalf("unexpected totals: %+v", facts)
	}
	if facts.SavingsRate != 60 || facts.ExpenseChange == nil || *facts.ExpenseChange != 300 {
		t.Fatalf("unexpected derived facts: %+v", facts)
	}
	if len(facts.TopCategories) != 2 || facts.TopCategories[0].Name != "Makan" {
		t.Fatalf("categories are not sorted: %+v", facts.TopCategories)
	}
}

func TestInsightPayloadContainsNoIdentityOrNote(t *testing.T) {
	data, err := json.Marshal(repository.InsightTransaction{Date: "2026-07-01", Type: "expense", Amount: 10, Category: "Makan"})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"id", "user_id", "email", "note", "full_name"} {
		if _, exists := value[forbidden]; exists {
			t.Fatalf("payload leaks %s", forbidden)
		}
	}
}

func TestParseGeminiStructuredOutput(t *testing.T) {
	analysis := `{"headline":"Arus kas sehat","summary":"Pengeluaran masih terkendali.","health_status":"good","key_findings":["Saldo positif"],"recommendations":[{"title":"Pertahankan","action":"Tinjau mingguan","priority":"low"}],"cautions":[]}`
	response, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": analysis}}}}}})
	result, err := parseGemini(response)
	if err != nil || result.HealthStatus != "good" {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
}

func TestValidateAnalysisRejectsUnknownStatus(t *testing.T) {
	err := validateAnalysis(&InsightAnalysis{Headline: "x", Summary: "y", HealthStatus: "unknown"})
	if err == nil {
		t.Fatal("expected invalid health status")
	}
}
