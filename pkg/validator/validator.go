package validator

import (
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Error vars
var (
	ErrRequired     = errors.New("field is required")
	ErrInvalidEmail = errors.New("invalid email format")
	ErrInvalidType  = errors.New("type must be 'income' or 'expense'")
	ErrAmountZero   = errors.New("amount must be greater than 0")
	ErrInvalidDate  = errors.New("invalid date format, use YYYY-MM-DD")
	ErrInvalidUUID  = errors.New("invalid UUID format")
	ErrMaxLength    = errors.New("exceeds maximum length")
	ErrMinLength    = errors.New("below minimum length")
)

// Required checks if a string is non-empty
func Required(value string, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(fieldName + " is required")
	}
	return nil
}

// Email validates email format
func Email(email string) error {
	if strings.TrimSpace(email) == "" {
		return ErrRequired
	}
	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}
	return nil
}

// TransactionType validates income/expense
func TransactionType(t string) error {
	t = strings.ToLower(t)
	if t != "income" && t != "expense" {
		return ErrInvalidType
	}
	return nil
}

// AmountPositive validates amount > 0
func AmountPositive(amount float64) error {
	if amount <= 0 {
		return ErrAmountZero
	}
	return nil
}

// DateFormat validates YYYY-MM-DD format
func DateFormat(date string) error {
	if strings.TrimSpace(date) == "" {
		return ErrRequired
	}
	_, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ErrInvalidDate
	}
	return nil
}

// UUID validates UUID v4 format
func UUID(id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrRequired
	}
	if !uuidRegex.MatchString(strings.ToLower(id)) {
		return ErrInvalidUUID
	}
	return nil
}

// MaxLength validates string max length
func MaxLength(value string, max int, fieldName string) error {
	if len(value) > max {
		return errors.New(fieldName + " " + ErrMaxLength.Error() + " " + string(rune(max)))
	}
	return nil
}

// MinLength validates string min length
func MinLength(value string, min int, fieldName string) error {
	if len(value) < min {
		return errors.New(fieldName + " " + ErrMinLength.Error() + " " + string(rune(min)))
	}
	return nil
}

// Password validates password strength (min 6 chars)
func Password(password string) error {
	if strings.TrimSpace(password) == "" {
		return ErrRequired
	}
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	return nil
}
