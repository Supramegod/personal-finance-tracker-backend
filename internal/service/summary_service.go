package service

import (
	"personal-finance-tracker/internal/repository"
)

type SummaryService struct {
	repo        *repository.SummaryRepository
	savingsRepo *repository.SavingsRepository
}

func NewSummaryService(
	repo *repository.SummaryRepository,
	savingsRepo *repository.SavingsRepository,
) *SummaryService {
	return &SummaryService{repo: repo, savingsRepo: savingsRepo}
}

// BalanceResponse memisahkan uang yang bebas dipakai dari uang yang sudah
// dialokasikan ke tabungan.
//
//	Balance      — saldo kas: sudah dikurangi setoran tabungan.
//	SavingsTotal — total di semua pot tabungan kelompok.
//	NetWorth     — kekayaan bersih: kas + tabungan.
//
// Field Balance sengaja tidak berganti nama maupun arti struktural, supaya
// klien lama yang belum tahu soal tabungan tetap jalan.
type BalanceResponse struct {
	Balance      float64 `json:"balance"`
	SavingsTotal float64 `json:"savings_total"`
	NetWorth     float64 `json:"net_worth"`
}

func (s *SummaryService) GetBalance(groupID string) (*BalanceResponse, error) {
	balance, err := s.repo.GetBalance(groupID)
	if err != nil {
		return nil, err
	}

	savings, err := s.savingsRepo.TotalSaved(groupID)
	if err != nil {
		return nil, err
	}

	return &BalanceResponse{
		Balance:      balance,
		SavingsTotal: savings,
		NetWorth:     balance + savings,
	}, nil
}

// ReportResponse memisahkan arus kas operasional dari tabungan, sehingga
// berlaku identitas: Net = TotalIncome - TotalExpense - TotalSavings.
type ReportResponse struct {
	Rows         []repository.ReportRow `json:"periods"`
	TotalIncome  float64                `json:"total_income"`
	TotalExpense float64                `json:"total_expense"`
	TotalSavings float64                `json:"total_savings"`
	Net          float64                `json:"net"`
}

func (s *SummaryService) GetReport(groupID, period, from, to string) (*ReportResponse, error) {
	rows, totalIncome, totalExpense, totalSavings, err := s.repo.GetReport(groupID, period, from, to)
	if err != nil {
		return nil, err
	}

	return &ReportResponse{
		Rows:         rows,
		TotalIncome:  totalIncome,
		TotalExpense: totalExpense,
		TotalSavings: totalSavings,
		Net:          totalIncome - totalExpense - totalSavings,
	}, nil
}
