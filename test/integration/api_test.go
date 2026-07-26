// Package integration berisi integration test untuk endpoint-endpoint kritis.
// Test ini membutuhkan koneksi database PostgreSQL yang aktif.
// Untuk menjalankan: go test ./test/integration/... -v -count=1
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"testing"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/pkg/auth"
)

const (
	baseURL = "http://localhost:8080/api/v1"
)

var (
	accessToken  string
	refreshToken string
	userID       string
	categoryID   string
	transactionID string
)

// TestMain handles test setup
func TestMain(m *testing.M) {
	// Skip if integration test not requested
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		log.Println("Skipping integration tests. Set RUN_INTEGRATION_TESTS=1 to run.")
		os.Exit(0)
	}

	// Check if server is running
	resp, err := http.Get(baseURL + "/health")
	if err != nil || resp.StatusCode != 200 {
		log.Printf("Server is not running at %s. Start with 'make run' first.", baseURL)
		os.Exit(1)
	}
	resp.Body.Close()

	// Ensure database is set up by pinging it
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://finance:finance123@localhost:5432/finance_tracker?sslmode=disable"
	}
	db, err := repository.NewDB(dbURL)
	if err != nil {
		log.Printf("Database not available: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Seed admin + categories
	if err := db.SeedAdmin(); err != nil {
		log.Printf("Seed warning: %v", err)
	}

	// Get admin user info for tests
	userRepo := repository.NewUserRepository(db.Pool)
	admin, err := userRepo.FindByEmail(os.Getenv("ADMIN_EMAIL"))
	if err != nil {
		admin, err = userRepo.FindByEmail("admin@example.com")
		if err != nil {
			log.Fatalf("Admin user not found: %v", err)
		}
	}
	userID = admin.ID

	// Get a category ID (scoping kini berbasis group)
	catRepo := repository.NewCategoryRepository(db.Pool)
	groupRepo := repository.NewGroupRepository(db.Pool)
	groupID, err := groupRepo.DefaultGroupForUser(userID)
	if err != nil {
		log.Fatalf("Admin group not found: %v", err)
	}
	cats, err := catRepo.FindByGroupID(groupID, "income")
	if err != nil || len(cats) == 0 {
		log.Fatalf("No categories found: %v", err)
	}
	categoryID = cats[0].ID

	code := m.Run()
	os.Exit(code)
}

// Helper: HTTP request with JSON body
func apiRequest(method, path string, body interface{}, token string) (*http.Response, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	return resp, respBytes, nil
}

// Helper: parse JSON response
func parseJSON(t *testing.T, data []byte, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nBody: %s", err, string(data))
	}
}

// ============================================================
// AUTH INTEGRATION TESTS
// ============================================================

func TestIntegrationLogin(t *testing.T) {
	resp, body, err := apiRequest("POST", "/auth/login", map[string]string{
		"email":    "admin@example.com",
		"password": "admin123",
	}, "")
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	accessToken = result["access_token"].(string)
	refreshToken = result["refresh_token"].(string)

	if accessToken == "" {
		t.Fatal("Access token should not be empty")
	}
	if refreshToken == "" {
		t.Fatal("Refresh token should not be empty")
	}

	t.Logf("Login successful. Access token: %s...", accessToken[:20])
}

func TestIntegrationLoginInvalid(t *testing.T) {
	// Wrong password
	resp, _, _ := apiRequest("POST", "/auth/login", map[string]string{
		"email":    "admin@example.com",
		"password": "wrongpassword",
	}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for wrong password, got %d", resp.StatusCode)
	}

	// Non-existent user
	resp, _, _ = apiRequest("POST", "/auth/login", map[string]string{
		"email":    "nonexistent@test.com",
		"password": "password123",
	}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for non-existent user, got %d", resp.StatusCode)
	}
}

func TestIntegrationRefreshToken(t *testing.T) {
	if refreshToken == "" {
		t.Skip("No refresh token available, login first")
	}

	resp, body, err := apiRequest("POST", "/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	}, "")
	if err != nil {
		t.Fatalf("Refresh request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	newAccess := result["access_token"].(string)
	newRefresh := result["refresh_token"].(string)

	if newAccess == "" || newRefresh == "" {
		t.Fatal("New tokens should not be empty")
	}

	// Old refresh token should be revoked (rotation)
	resp, _, _ = apiRequest("POST", "/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for revoked token, got %d", resp.StatusCode)
	}

	// Update tokens for subsequent tests
	accessToken = newAccess
	refreshToken = newRefresh
}

func TestIntegrationLogout(t *testing.T) {
	if refreshToken == "" {
		t.Skip("No refresh token available")
	}

	resp, body, err := apiRequest("POST", "/auth/logout", map[string]string{
		"refresh_token": refreshToken,
	}, "")
	if err != nil {
		t.Fatalf("Logout request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// Token should be revoked now
	resp, _, _ = apiRequest("POST", "/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for revoked token after logout, got %d", resp.StatusCode)
	}
}

func TestIntegrationAuthRequired(t *testing.T) {
	// Access protected endpoint without token
	resp, _, _ := apiRequest("GET", "/transactions?page=1&limit=10", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 without token, got %d", resp.StatusCode)
	}
}

// ============================================================
// TRANSACTION INTEGRATION TESTS
// ============================================================

func TestIntegrationCreateTransaction(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token, login first")
	}

	resp, body, err := apiRequest("POST", "/transactions", map[string]interface{}{
		"type":             "income",
		"amount":           5000000,
		"category_id":      categoryID,
		"transaction_date": "2026-06-19",
		"note":             "Integration test: Gaji Juni",
	}, accessToken)
	if err != nil {
		t.Fatalf("Create transaction failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	transactionID = result["id"].(string)
	if transactionID == "" {
		t.Fatal("Transaction ID should not be empty")
	}

	amount := result["amount"].(float64)
	if amount != 5000000 {
		t.Errorf("Expected amount 5000000, got %f", amount)
	}

	t.Logf("Transaction created: %s", transactionID)
}

func TestIntegrationCreateTransactionInvalid(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	// Zero amount
	resp, body, _ := apiRequest("POST", "/transactions", map[string]interface{}{
		"type":             "expense",
		"amount":           0,
		"category_id":      categoryID,
		"transaction_date": "2026-06-19",
	}, accessToken)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for zero amount, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// Invalid type
	resp, _, _ = apiRequest("POST", "/transactions", map[string]interface{}{
		"type":             "invalid",
		"amount":           1000,
		"category_id":      categoryID,
		"transaction_date": "2026-06-19",
	}, accessToken)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid type, got %d", resp.StatusCode)
	}

	// Missing required fields
	resp, _, _ = apiRequest("POST", "/transactions", map[string]interface{}{
		"type": "income",
	}, accessToken)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing fields, got %d", resp.StatusCode)
	}
}

func TestIntegrationGetTransactionByID(t *testing.T) {
	if transactionID == "" || accessToken == "" {
		t.Skip("No transaction ID or access token")
	}

	resp, body, err := apiRequest("GET", "/transactions/"+transactionID, nil, accessToken)
	if err != nil {
		t.Fatalf("Get transaction failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	if result["id"] != transactionID {
		t.Errorf("Expected ID %s, got %s", transactionID, result["id"])
	}
}

func TestIntegrationUpdateTransaction(t *testing.T) {
	if transactionID == "" || accessToken == "" {
		t.Skip("No transaction ID or access token")
	}

	resp, body, err := apiRequest("PUT", "/transactions/"+transactionID, map[string]interface{}{
		"type":             "income",
		"amount":           7500000,
		"category_id":      categoryID,
		"transaction_date": "2026-06-20",
		"note":             "Updated: Gaji Juni (bonus)",
	}, accessToken)
	if err != nil {
		t.Fatalf("Update transaction failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	amount := result["amount"].(float64)
	if amount != 7500000 {
		t.Errorf("Expected amount 7500000, got %f", amount)
	}
}

func TestIntegrationListTransactions(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	resp, body, err := apiRequest("GET", "/transactions?page=1&limit=10", nil, accessToken)
	if err != nil {
		t.Fatalf("List transactions failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	data := result["data"].([]interface{})
	total := result["total"].(float64)

	if len(data) == 0 {
		t.Error("Should have at least 1 transaction")
	}
	if total < 1 {
		t.Errorf("Expected total >= 1, got %f", total)
	}
}

func TestIntegrationListTransactionsFilterByType(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	resp, body, err := apiRequest("GET", "/transactions?type=income&page=1&limit=10", nil, accessToken)
	if err != nil {
		t.Fatalf("List filtered transactions failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	data := result["data"].([]interface{})
	for i, item := range data {
		tx := item.(map[string]interface{})
		if tx["type"] != "income" {
			t.Errorf("Item %d: expected type 'income', got %s", i, tx["type"])
		}
	}
}

// ============================================================
// CATEGORY INTEGRATION TESTS
// ============================================================

func TestIntegrationListCategories(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	resp, body, err := apiRequest("GET", "/categories", nil, accessToken)
	if err != nil {
		t.Fatalf("List categories failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	data := result["data"].([]interface{})
	if len(data) == 0 {
		t.Error("Should have at least 1 category")
	}
}

func TestIntegrationListCategoriesByType(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	// Filter income
	resp, body, err := apiRequest("GET", "/categories?type=income", nil, accessToken)
	if err != nil {
		t.Fatalf("List income categories failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)
	data := result["data"].([]interface{})
	for i, item := range data {
		cat := item.(map[string]interface{})
		if cat["type"] != "income" {
			t.Errorf("Item %d: expected type 'income', got %s", i, cat["type"])
		}
	}
}

func TestIntegrationCreateCategory(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	resp, body, err := apiRequest("POST", "/categories", map[string]string{
		"name": "Test Category INTEGRATION",
		"type": "expense",
		"icon": "test",
	}, accessToken)
	if err != nil {
		t.Fatalf("Create category failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	if result["name"] != "Test Category INTEGRATION" {
		t.Errorf("Expected name 'Test Category INTEGRATION', got %s", result["name"])
	}

	// Duplicate name should fail
	resp, _, _ = apiRequest("POST", "/categories", map[string]string{
		"name": "Test Category INTEGRATION",
		"type": "expense",
	}, accessToken)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for duplicate category, got %d", resp.StatusCode)
	}
}

// ============================================================
// SUMMARY INTEGRATION TESTS
// ============================================================

func TestIntegrationGetBalance(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	resp, body, err := apiRequest("GET", "/summary/balance", nil, accessToken)
	if err != nil {
		t.Fatalf("Get balance failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	balance := result["balance"].(float64)
	t.Logf("Current balance: Rp%.2f", balance)
}

func TestIntegrationGetReport(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	// Monthly report
	resp, body, err := apiRequest("GET", "/summary/report?period=monthly&from=2026-01-01&to=2026-12-31", nil, accessToken)
	if err != nil {
		t.Fatalf("Get report failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	totalIncome := result["total_income"].(float64)
	totalExpense := result["total_expense"].(float64)
	net := result["net"].(float64)

	if net != totalIncome-totalExpense {
		t.Errorf("Net should equal income - expense: %f != %f - %f", net, totalIncome, totalExpense)
	}
}

func TestIntegrationReportMissingParams(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	// Missing 'from' param
	resp, _, _ := apiRequest("GET", "/summary/report?period=monthly", nil, accessToken)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing params, got %d", resp.StatusCode)
	}
}

func TestIntegrationBalanceAfterTransaction(t *testing.T) {
	if accessToken == "" || categoryID == "" {
		t.Skip("No access token or category ID")
	}

	// Get balance before
	resp, body, _ := apiRequest("GET", "/summary/balance", nil, accessToken)
	var before map[string]interface{}
	parseJSON(t, body, &before)
	balanceBefore := before["balance"].(float64)

	// Create an income transaction
	resp, body, err := apiRequest("POST", "/transactions", map[string]interface{}{
		"type":             "income",
		"amount":           1000000,
		"category_id":      categoryID,
		"transaction_date": "2026-06-19",
		"note":             "Balance integration test",
	}, accessToken)
	if err != nil {
		t.Fatalf("Create transaction failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201, got %d", resp.StatusCode)
	}

	var txResult map[string]interface{}
	parseJSON(t, body, &txResult)
	newTxID := txResult["id"].(string)

	// Get balance after
	resp, body, _ = apiRequest("GET", "/summary/balance", nil, accessToken)
	var after map[string]interface{}
	parseJSON(t, body, &after)
	balanceAfter := after["balance"].(float64)

	expected := balanceBefore + 1000000
	if balanceAfter != expected {
		t.Errorf("Expected balance %f, got %f (diff: %f)", expected, balanceAfter, balanceAfter-balanceBefore)
	}

	// Cleanup: delete the test transaction
	apiRequest("DELETE", "/transactions/"+newTxID, nil, accessToken)
}

// ============================================================
// SOFT DELETE INTEGRATION TEST
// ============================================================

func TestIntegrationSoftDeleteTransaction(t *testing.T) {
	if accessToken == "" || categoryID == "" {
		t.Skip("No access token or category ID")
	}

	// Create transaction to delete
	resp, body, _ := apiRequest("POST", "/transactions", map[string]interface{}{
		"type":             "expense",
		"amount":           50000,
		"category_id":      categoryID,
		"transaction_date": "2026-06-19",
		"note":             "To be deleted",
	}, accessToken)
	var txResult map[string]interface{}
	parseJSON(t, body, &txResult)
	deleteID := txResult["id"].(string)

	// Delete it
	resp, _, _ = apiRequest("DELETE", "/transactions/"+deleteID, nil, accessToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 for delete, got %d", resp.StatusCode)
	}

	// Should not be found anymore
	resp, _, _ = apiRequest("GET", "/transactions/"+deleteID, nil, accessToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for deleted transaction, got %d", resp.StatusCode)
	}

	// Should NOT appear in list
	resp, body, _ = apiRequest("GET", "/transactions?page=1&limit=100", nil, accessToken)
	var listResult map[string]interface{}
	parseJSON(t, body, &listResult)
	data := listResult["data"].([]interface{})
	for _, item := range data {
		tx := item.(map[string]interface{})
		if tx["id"] == deleteID {
			t.Error("Deleted transaction should not appear in list")
		}
	}
}

// ============================================================
// JWT & BCRYPT UNIT-LEVEL VERIFICATION
// ============================================================

func TestIntegrationJWTTokenLifecycle(t *testing.T) {
	// Set env for test
	os.Setenv("JWT_SECRET", "test-jwt-secret-min-32-chars-long-for-testing")
	defer os.Setenv("JWT_SECRET", "")

	// Generate access token
	token, err := auth.GenerateAccessToken("test-user-id", "test@example.com", "")
	if err != nil {
		t.Fatalf("Generate access token failed: %v", err)
	}

	// Validate it
	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("Validate token failed: %v", err)
	}

	if claims.UserID != "test-user-id" {
		t.Errorf("Expected UserID 'test-user-id', got '%s'", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Expected Email 'test@example.com', got '%s'", claims.Email)
	}
}

func TestIntegrationBcryptPassword(t *testing.T) {
	password := "securePassword123!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !auth.CheckPassword(password, hash) {
		t.Error("CheckPassword should return true for correct password")
	}

	if auth.CheckPassword("wrongPassword", hash) {
		t.Error("CheckPassword should return false for wrong password")
	}
}

// ============================================================
// RATE LIMITING TEST
// ============================================================

func TestIntegrationHealthEndpoint(t *testing.T) {
	resp, body, err := apiRequest("GET", "/health", nil, "")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)
	if result["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %s", result["status"])
	}
}

// ============================================================
// CLEANUP — Delete all test transactions
// ============================================================

func TestIntegrationCleanup(t *testing.T) {
	if accessToken == "" {
		t.Skip("No access token")
	}

	// Get all transactions and delete test ones
	resp, body, _ := apiRequest("GET", "/transactions?page=1&limit=100", nil, accessToken)
	if resp.StatusCode != http.StatusOK {
		return
	}

	var result map[string]interface{}
	parseJSON(t, body, &result)

	data := result["data"].([]interface{})
	deleted := 0
	for _, item := range data {
		tx := item.(map[string]interface{})
		note := ""
		if n, ok := tx["note"]; ok && n != nil {
			note = n.(string)
		}
		if note == "To be deleted" || note == "Balance integration test" || note == "Integration test: Gaji Juni" {
			id := tx["id"].(string)
			apiRequest("DELETE", "/transactions/"+id, nil, accessToken)
			deleted++
		}
	}
	t.Logf("Cleaned up %d test transactions", deleted)

	// Also clean up test category
	resp, body, _ = apiRequest("GET", "/categories", nil, accessToken)
	if resp.StatusCode == http.StatusOK {
		var catResult map[string]interface{}
		parseJSON(t, body, &catResult)
		cats := catResult["data"].([]interface{})
		for _, item := range cats {
			cat := item.(map[string]interface{})
			if cat["name"] == "Test Category INTEGRATION" {
				id := cat["id"].(string)
				apiRequest("DELETE", "/categories/"+id, nil, accessToken)
				t.Log("Cleaned up test category")
			}
		}
	}
}
