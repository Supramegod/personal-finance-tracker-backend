package service_test

import (
	"testing"

	"personal-finance-tracker/internal/service"
)

func TestCreateIncomeTransaction(t *testing.T) {
	tx, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "income",
		Amount:          5000000,
		TransactionDate: "2026-06-19",
		Note:            "Gaji bulan Juni",
	})
	if err != nil {
		t.Fatalf("Create transaction failed: %v", err)
	}
	if tx.Amount != 5000000 {
		t.Errorf("Expected amount 5000000, got %f", tx.Amount)
	}
	if tx.Type != "income" {
		t.Errorf("Expected type income, got %s", tx.Type)
	}
	if tx.ID == "" {
		t.Error("Transaction ID should not be empty")
	}
}

func TestCreateExpenseTransaction(t *testing.T) {
	tx, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "expense",
		Amount:          25000,
		TransactionDate: "2026-06-19",
		Note:            "Makan siang",
	})
	if err != nil {
		t.Fatalf("Create expense failed: %v", err)
	}
	if tx.Type != "expense" {
		t.Errorf("Expected type expense, got %s", tx.Type)
	}
}

func TestCreateTransactionInvalidAmount(t *testing.T) {
	_, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "expense",
		Amount:          0,
		TransactionDate: "2026-06-19",
	})
	if err == nil {
		t.Fatal("Should reject zero amount")
	}
}

func TestCreateTransactionNegativeAmount(t *testing.T) {
	_, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "expense",
		Amount:          -1000,
		TransactionDate: "2026-06-19",
	})
	if err == nil {
		t.Fatal("Should reject negative amount")
	}
}

func TestCreateTransactionInvalidType(t *testing.T) {
	_, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "invalid",
		Amount:          1000,
		TransactionDate: "2026-06-19",
	})
	if err == nil {
		t.Fatal("Should reject invalid type")
	}
}

func TestCreateTransactionMissingDate(t *testing.T) {
	_, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:     testUserID,
		CategoryID: testCatID,
		Type:       "income",
		Amount:     1000,
	})
	if err == nil {
		t.Fatal("Should reject empty transaction date")
	}
}

func TestCreateTransactionInvalidDateFormat(t *testing.T) {
	_, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "income",
		Amount:          1000,
		TransactionDate: "19-06-2026",
	})
	if err == nil {
		t.Fatal("Should reject wrong date format")
	}
}

func TestListTransactions(t *testing.T) {
	transactions, total, err := testTxSvc.List(service.ListTransactionsInput{
		UserID: testUserID,
		Page:   1,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("List transactions failed: %v", err)
	}
	if total < 1 {
		t.Error("Should have at least 1 transaction")
	}
	if len(transactions) == 0 {
		t.Error("Should return transactions")
	}
}

func TestListTransactionsFilterByType(t *testing.T) {
	transactions, total, err := testTxSvc.List(service.ListTransactionsInput{
		UserID: testUserID,
		Type:   "income",
		Page:   1,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("List filtered transactions failed: %v", err)
	}
	if total < 1 {
		t.Error("Should have at least 1 income transaction")
	}
	for _, tx := range transactions {
		if tx.Type != "income" {
			t.Errorf("Expected income type, got %s", tx.Type)
		}
	}
}

func TestUpdateTransaction(t *testing.T) {
	tx, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "income",
		Amount:          1000000,
		TransactionDate: "2026-06-19",
		Note:            "Test update",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := testTxSvc.Update(service.UpdateTransactionInput{
		ID:              tx.ID,
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "income",
		Amount:          2000000,
		TransactionDate: "2026-06-20",
		Note:            "Updated",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Amount != 2000000 {
		t.Errorf("Expected amount 2000000, got %f", updated.Amount)
	}
}

func TestDeleteTransaction(t *testing.T) {
	tx, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "expense",
		Amount:          50000,
		TransactionDate: "2026-06-19",
		Note:            "To delete",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = testTxSvc.Delete(tx.ID, testUserID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = testTxSvc.GetByID(tx.ID, testUserID)
	if err == nil {
		t.Error("Should not find deleted transaction")
	}
}

func TestGetTransactionByID(t *testing.T) {
	tx, err := testTxSvc.Create(service.CreateTransactionInput{
		UserID:          testUserID,
		CategoryID:      testCatID,
		Type:            "income",
		Amount:          7500000,
		TransactionDate: "2026-06-19",
		Note:            "Get by ID test",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := testTxSvc.GetByID(tx.ID, testUserID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.ID != tx.ID {
		t.Errorf("Expected ID %s, got %s", tx.ID, found.ID)
	}
}
