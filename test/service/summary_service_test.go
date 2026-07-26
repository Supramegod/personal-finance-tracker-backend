package service_test

import (
	"testing"

	"personal-finance-tracker/internal/service"
)

func TestGetBalance(t *testing.T) {
	balance, err := testSummarySvc.GetBalance(testGroupID)
	if err != nil {
		t.Fatalf("Get balance failed: %v", err)
	}
	if balance.Balance < 0 {
		t.Errorf("Balance should not be negative, got %f", balance.Balance)
	}
}

func TestGetReportDaily(t *testing.T) {
	report, err := testSummarySvc.GetReport(testGroupID, "daily", "2026-06-01", "2026-06-30")
	if err != nil {
		t.Fatalf("Get daily report failed: %v", err)
	}
	if len(report.Rows) == 0 {
		t.Error("Report should have at least 1 period")
	}
	if report.TotalIncome < 0 {
		t.Errorf("Total income should not be negative, got %f", report.TotalIncome)
	}
}

func TestGetReportWeekly(t *testing.T) {
	report, err := testSummarySvc.GetReport(testGroupID, "weekly", "2026-06-01", "2026-06-30")
	if err != nil {
		t.Fatalf("Get weekly report failed: %v", err)
	}
	if report.Net != report.TotalIncome-report.TotalExpense {
		t.Errorf("Net should equal income - expense: net=%f, income=%f, expense=%f",
			report.Net, report.TotalIncome, report.TotalExpense)
	}
}

func TestGetReportMonthly(t *testing.T) {
	report, err := testSummarySvc.GetReport(testGroupID, "monthly", "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Get monthly report failed: %v", err)
	}
	if report.Net != report.TotalIncome-report.TotalExpense {
		t.Errorf("Net mismatch: net=%f, income=%f, expense=%f",
			report.Net, report.TotalIncome, report.TotalExpense)
	}
}

func TestGetReportNoData(t *testing.T) {
	report, err := testSummarySvc.GetReport(testGroupID, "daily", "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("Get report for empty period failed: %v", err)
	}
	if report.TotalIncome != 0 {
		t.Errorf("Expected 0 income for empty period, got %f", report.TotalIncome)
	}
	if report.Net != 0 {
		t.Errorf("Expected 0 net for empty period, got %f", report.Net)
	}
}

func TestBalanceAfterNewTransaction(t *testing.T) {
	balanceBefore, err := testSummarySvc.GetBalance(testGroupID)
	if err != nil {
		t.Fatalf("Get balance before: %v", err)
	}

	tx, err := testTxSvc.Create(service.CreateTransactionInput{
		GroupID:         testGroupID,
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "income",
		Amount:          1000000,
		TransactionDate: "2026-06-19",
		Note:            "Balance test",
	})
	if err != nil {
		t.Fatalf("Create transaction: %v", err)
	}

	balanceAfter, err := testSummarySvc.GetBalance(testGroupID)
	if err != nil {
		t.Fatalf("Get balance after: %v", err)
	}

	expected := balanceBefore.Balance + 1000000
	if balanceAfter.Balance != expected {
		t.Errorf("Expected balance %f, got %f", expected, balanceAfter.Balance)
	}

	testTxSvc.Delete(tx.ID, testGroupID)
}
