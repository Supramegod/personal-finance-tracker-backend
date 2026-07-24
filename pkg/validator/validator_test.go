package validator

import "testing"

func TestRequired(t *testing.T) {
	if err := Required("", "field"); err == nil {
		t.Error("Required should reject empty string")
	}
	if err := Required("  ", "field"); err == nil {
		t.Error("Required should reject whitespace-only string")
	}
	if err := Required("value", "field"); err != nil {
		t.Errorf("Required should accept non-empty string: %v", err)
	}
}

func TestEmail(t *testing.T) {
	if err := Email(""); err == nil {
		t.Error("Email should reject empty")
	}
	if err := Email("invalid"); err == nil {
		t.Error("Email should reject invalid format")
	}
	if err := Email("user@example.com"); err != nil {
		t.Errorf("Email should accept valid: %v", err)
	}
	if err := Email("user+tag@example.co.id"); err != nil {
		t.Errorf("Email should accept valid with tag: %v", err)
	}
}

func TestTransactionType(t *testing.T) {
	if err := TransactionType("income"); err != nil {
		t.Errorf("Should accept income: %v", err)
	}
	if err := TransactionType("expense"); err != nil {
		t.Errorf("Should accept expense: %v", err)
	}
	if err := TransactionType("INCOME"); err != nil {
		t.Errorf("Should accept case-insensitive: %v", err)
	}
	if err := TransactionType("invalid"); err == nil {
		t.Error("Should reject invalid type")
	}
}

func TestAmountPositive(t *testing.T) {
	if err := AmountPositive(0); err == nil {
		t.Error("Should reject zero amount")
	}
	if err := AmountPositive(-100); err == nil {
		t.Error("Should reject negative amount")
	}
	if err := AmountPositive(1); err != nil {
		t.Errorf("Should accept positive amount: %v", err)
	}
	if err := AmountPositive(999999999); err != nil {
		t.Errorf("Should accept large amount: %v", err)
	}
}

func TestDateFormat(t *testing.T) {
	if err := DateFormat(""); err == nil {
		t.Error("Should reject empty date")
	}
	if err := DateFormat("19-06-2026"); err == nil {
		t.Error("Should reject DD-MM-YYYY format")
	}
	if err := DateFormat("2026/06/19"); err == nil {
		t.Error("Should reject slash format")
	}
	if err := DateFormat("2026-06-19"); err != nil {
		t.Errorf("Should accept YYYY-MM-DD: %v", err)
	}
	if err := DateFormat("2026-02-28"); err != nil {
		t.Errorf("Should accept valid date: %v", err)
	}
}

func TestUUID(t *testing.T) {
	if err := UUID(""); err == nil {
		t.Error("Should reject empty UUID")
	}
	if err := UUID("not-a-uuid"); err == nil {
		t.Error("Should reject invalid UUID")
	}
	if err := UUID("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Errorf("Should accept valid UUID: %v", err)
	}
}

func TestMaxLength(t *testing.T) {
	if err := MaxLength("hello", 5, "field"); err != nil {
		t.Errorf("Should accept equal to max: %v", err)
	}
	if err := MaxLength("hello!", 5, "field"); err == nil {
		t.Error("Should reject exceeding max")
	}
}

func TestMinLength(t *testing.T) {
	if err := MinLength("ab", 2, "field"); err != nil {
		t.Errorf("Should accept equal to min: %v", err)
	}
	if err := MinLength("a", 2, "field"); err == nil {
		t.Error("Should reject below min")
	}
}

func TestPassword(t *testing.T) {
	if err := Password(""); err == nil {
		t.Error("Should reject empty password")
	}
	if err := Password("12345"); err == nil {
		t.Error("Should reject password < 6 chars")
	}
	if err := Password("123456"); err != nil {
		t.Errorf("Should accept password >= 6 chars: %v", err)
	}
}
