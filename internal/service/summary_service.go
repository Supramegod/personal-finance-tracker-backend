package service

import (
	"personal-finance-tracker/internal/repository"
)

type SummaryService struct {
	repo *repository.SummaryRepository
}

func NewSummaryService(repo *repository.SummaryRepository) *SummaryService {
	return &SummaryService{repo: repo}
}

type BalanceResponse struct {
	Balance float64 `json:"balance"`
}

func (s *SummaryService) GetBalance(groupID string) (*BalanceResponse, error) {
	balance, err := s.repo.GetBalance(groupID)
	if err != nil {
		return nil, err
	}
	return &BalanceResponse{Balance: balance}, nil
}

type ReportResponse struct {
	Rows         []repository.ReportRow `json:"periods"`
	TotalIncome  float64                `json:"total_income"`
	TotalExpense float64                `json:"total_expense"`
	Net          float64                `json:"net"`
}

func (s *SummaryService) GetReport(groupID, period, from, to string) (*ReportResponse, error) {
	rows, totalIncome, totalExpense, err := s.repo.GetReport(groupID, period, from, to)
	if err != nil {
		return nil, err
	}

	return &ReportResponse{
		Rows:         rows,
		TotalIncome:  totalIncome,
		TotalExpense: totalExpense,
		Net:          totalIncome - totalExpense,
	}, nil
}
