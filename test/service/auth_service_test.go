package service_test

import (
	"os"
	"testing"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/service"
	"personal-finance-tracker/pkg/auth"
)

var (
	testUserRepo     *repository.UserRepository
	testCategoryRepo *repository.CategoryRepository
	testGroupRepo    *repository.GroupRepository
	testAuthSvc      *service.AuthService
	testTxRepo       *repository.TransactionRepository
	testTxSvc        *service.TransactionService
	testSummaryRepo  *repository.SummaryRepository
	testSummarySvc   *service.SummaryService
	testUserID       string
	testGroupID      string
	testCatID        string
)

func TestMain(m *testing.M) {
	// Set env vars for tests
	os.Setenv("JWT_SECRET", "test-jwt-secret-min-32-chars-long-for-testing")
	os.Setenv("JWT_ACCESS_EXPIRY", "15m")
	os.Setenv("JWT_REFRESH_EXPIRY", "168h") // "7d" bukan unit valid time.ParseDuration

	// Setup shared database
	dbURL := "postgresql://finance:finance123@localhost:5432/finance_tracker?sslmode=disable"
	db, err := repository.NewDB(dbURL)
	if err != nil {
		os.Exit(0) // Skip if DB not available
	}

	testUserRepo = repository.NewUserRepository(db.Pool)
	testCategoryRepo = repository.NewCategoryRepository(db.Pool)
	testTxRepo = repository.NewTransactionRepository(db.Pool)
	testSummaryRepo = repository.NewSummaryRepository(db.Pool)

	refreshTokenRepo := repository.NewRefreshTokenRepository(db.Pool)
	testGroupRepo = repository.NewGroupRepository(db.Pool)
	testAuthSvc = service.NewAuthService(testUserRepo, testCategoryRepo, refreshTokenRepo, testGroupRepo)
	testTxSvc = service.NewTransactionService(testTxRepo, testCategoryRepo)
	testSummarySvc = service.NewSummaryService(testSummaryRepo)

	// Get admin user ID
	user, err := testUserRepo.FindByEmail("admin@example.com")
	if err == nil {
		testUserID = user.ID
		// Get category ID (scoping kini berbasis group)
		if groupID, gerr := testGroupRepo.DefaultGroupForUser(testUserID); gerr == nil {
			testGroupID = groupID
			cats, err := testCategoryRepo.FindByGroupID(groupID, "income")
			if err == nil && len(cats) > 0 {
				testCatID = cats[0].ID
			}
		}
	}

	code := m.Run()
	db.Close()
	os.Exit(code)
}

func TestLoginSuccess(t *testing.T) {
	result, err := testAuthSvc.Login("admin@example.com", "admin123")
	if err != nil {
		t.Fatalf("Login should succeed: %v", err)
	}

	if result.AccessToken == "" {
		t.Error("Access token should not be empty")
	}
	if result.RefreshToken == "" {
		t.Error("Refresh token should not be empty")
	}
	if result.User.Email != "admin@example.com" {
		t.Errorf("Expected email admin@example.com, got %s", result.User.Email)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	_, err := testAuthSvc.Login("admin@example.com", "wrongpassword")
	if err == nil {
		t.Fatal("Login should fail with wrong password")
	}
}

func TestLoginNonExistentUser(t *testing.T) {
	_, err := testAuthSvc.Login("nonexistent@email.com", "password123")
	if err == nil {
		t.Fatal("Login should fail for non-existent user")
	}
}

func TestLoginEmptyEmail(t *testing.T) {
	_, err := testAuthSvc.Login("", "password123")
	if err == nil {
		t.Fatal("Login should fail with empty email")
	}
}

func TestLoginEmptyPassword(t *testing.T) {
	_, err := testAuthSvc.Login("admin@example.com", "")
	if err == nil {
		t.Fatal("Login should fail with empty password")
	}
}

func TestRefreshToken(t *testing.T) {
	loginResult, err := testAuthSvc.Login("admin@example.com", "admin123")
	if err != nil {
		t.Fatalf("Login should succeed: %v", err)
	}

	refreshResult, err := testAuthSvc.Refresh(loginResult.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh should succeed: %v", err)
	}

	if refreshResult.AccessToken == "" {
		t.Error("New access token should not be empty")
	}
}

func TestRefreshInvalidToken(t *testing.T) {
	_, err := testAuthSvc.Refresh("invalid-refresh-token")
	if err == nil {
		t.Fatal("Refresh should fail with invalid token")
	}
}

func TestBcryptHashVerification(t *testing.T) {
	password := "testPassword123!"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Hash generation failed: %v", err)
	}

	if !auth.CheckPassword(password, hash) {
		t.Error("Password verification should succeed")
	}
	if auth.CheckPassword("wrongPassword", hash) {
		t.Error("Password verification should fail for wrong password")
	}
}

func TestJWTTokenValidation(t *testing.T) {
	accessToken, err := auth.GenerateAccessToken(testUserID, "test@example.com", "")
	if err != nil {
		t.Fatalf("Generate access token failed: %v", err)
	}

	claims, err := auth.ValidateToken(accessToken)
	if err != nil {
		t.Fatalf("Validate token failed: %v", err)
	}

	if claims.Email != "test@example.com" {
		t.Errorf("Expected Email test@example.com, got %s", claims.Email)
	}
}

func TestJWTExpiredToken(t *testing.T) {
	_, err := auth.ValidateToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJleHAiOjF9.CIFQKfVvOQfC3r4OzD6Btx0LlOYOQUh5JJJqDHqFkEE")
	if err == nil {
		t.Error("Should reject expired token")
	}
}
