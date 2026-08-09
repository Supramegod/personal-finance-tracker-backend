package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/service"
)

// Test tabungan yang menyentuh database. Fokusnya bukan validasi input (itu
// sudah dites tanpa DB di internal/service/savings_service_test.go), melainkan
// hal-hal yang HANYA bisa dibuktikan terhadap PostgreSQL sungguhan: atomisitas
// setoran, rollback saat penarikan ditolak, dan invariant saldo pot.

func requireSavingsEnv(t *testing.T) {
	t.Helper()
	if testGroupID == "" || testUserID == "" {
		t.Skip("butuh database dengan user admin ter-seed")
	}
}

// newTestGoal membuat pot bernama unik dan mendaftarkan pembersihannya:
// semua mutasi dibatalkan (yang sekaligus men-soft-delete transaksinya) lalu
// potnya dihapus, supaya test tidak meninggalkan sampah di database dev.
func newTestGoal(t *testing.T, target *float64) *repository.SavingsGoal {
	t.Helper()
	requireSavingsEnv(t)

	goal, err := testSavingsSvc.Create(service.CreateSavingsGoalInput{
		GroupID:      testGroupID,
		UserID:       testUserID,
		Name:         fmt.Sprintf("Test %s %d", t.Name(), time.Now().UnixNano()),
		TargetAmount: target,
	})
	if err != nil {
		t.Fatalf("buat pot tabungan: %v", err)
	}

	t.Cleanup(func() {
		entries, err := testSavingsSvc.ListEntries(goal.ID, testGroupID)
		if err != nil {
			t.Logf("cleanup: gagal ambil mutasi: %v", err)
			return
		}
		// Urutan terbalik: batalkan penarikan lebih dulu supaya saldo tidak
		// sempat jatuh di bawah nol di tengah pembersihan.
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].Direction == repository.SavingsDirectionWithdraw {
				if err := testSavingsSvc.DeleteEntry(entries[i].ID, goal.ID, testGroupID); err != nil {
					t.Logf("cleanup: hapus penarikan: %v", err)
				}
			}
		}
		for _, e := range entries {
			if e.Direction == repository.SavingsDirectionDeposit {
				if err := testSavingsSvc.DeleteEntry(e.ID, goal.ID, testGroupID); err != nil {
					t.Logf("cleanup: hapus setoran: %v", err)
				}
			}
		}
		if err := testSavingsSvc.Delete(goal.ID, testGroupID); err != nil {
			t.Logf("cleanup: hapus pot: %v", err)
		}
	})

	return goal
}

func deposit(t *testing.T, goalID string, amount float64) *repository.SavingsEntry {
	t.Helper()
	entry, err := testSavingsSvc.Deposit(service.SavingsEntryInput{
		GoalID:  goalID,
		GroupID: testGroupID,
		UserID:  testUserID,
		Amount:  amount,
	})
	if err != nil {
		t.Fatalf("setor %.0f: %v", amount, err)
	}
	return entry
}

// Setoran harus menghasilkan DUA hal sekaligus: baris mutasi dan transaksi
// bertanda transfer. Kalau salah satunya hilang, saldo kas dan saldo tabungan
// tidak akan pernah cocok lagi.
func TestSavingsDepositCreatesTransferTransaction(t *testing.T) {
	goal := newTestGoal(t, nil)

	before, err := testSummarySvc.GetBalance(testGroupID)
	if err != nil {
		t.Fatalf("ambil saldo awal: %v", err)
	}

	entry := deposit(t, goal.ID, 250_000)

	if entry.TransactionID == nil {
		t.Fatal("transaction_id: got nil; want id transaksi yang tercipta")
	}
	if entry.Direction != repository.SavingsDirectionDeposit {
		t.Errorf("direction: got %q; want %q", entry.Direction, repository.SavingsDirectionDeposit)
	}

	txn, err := testTxSvc.GetByID(*entry.TransactionID, testGroupID)
	if err != nil {
		t.Fatalf("ambil transaksi hasil setoran: %v", err)
	}
	if !txn.IsTransfer {
		t.Error("is_transfer: got false; want true — tanpa tanda ini setoran akan mencemari laporan")
	}
	if txn.Type != "expense" {
		t.Errorf("type: got %q; want \"expense\" — setoran memindahkan uang keluar dari kas", txn.Type)
	}
	if txn.Amount != 250_000 {
		t.Errorf("amount: got %.0f; want 250000", txn.Amount)
	}

	after, err := testSummarySvc.GetBalance(testGroupID)
	if err != nil {
		t.Fatalf("ambil saldo akhir: %v", err)
	}

	// Kas turun, tabungan naik, kekayaan bersih tidak bergerak — inilah arti
	// "menabung adalah transfer, bukan pengeluaran".
	if diff := before.Balance - after.Balance; !nearly(diff, 250_000) {
		t.Errorf("penurunan saldo kas: got %.2f; want 250000", diff)
	}
	if diff := after.SavingsTotal - before.SavingsTotal; !nearly(diff, 250_000) {
		t.Errorf("kenaikan total tabungan: got %.2f; want 250000", diff)
	}
	if !nearly(before.NetWorth, after.NetWorth) {
		t.Errorf("kekayaan bersih berubah: got %.2f; want %.2f", after.NetWorth, before.NetWorth)
	}
}

// Penarikan melebihi saldo harus ditolak DAN tidak meninggalkan jejak apa pun.
// Tanpa case ini, bug pada defer rollback tidak akan pernah ketahuan.
func TestSavingsWithdrawBeyondBalanceIsRejectedAndRollsBack(t *testing.T) {
	goal := newTestGoal(t, nil)
	deposit(t, goal.ID, 100_000)

	entriesBefore, err := testSavingsSvc.ListEntries(goal.ID, testGroupID)
	if err != nil {
		t.Fatalf("ambil mutasi awal: %v", err)
	}

	_, err = testSavingsSvc.Withdraw(service.SavingsEntryInput{
		GoalID:  goal.ID,
		GroupID: testGroupID,
		UserID:  testUserID,
		Amount:  500_000,
	})
	if !errors.Is(err, service.ErrSavingsInsufficient) {
		t.Fatalf("got %v; want ErrSavingsInsufficient", err)
	}

	entriesAfter, err := testSavingsSvc.ListEntries(goal.ID, testGroupID)
	if err != nil {
		t.Fatalf("ambil mutasi akhir: %v", err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Errorf("jumlah mutasi: got %d; want %d — transaksi DB tidak ter-rollback",
			len(entriesAfter), len(entriesBefore))
	}

	detail, err := testSavingsSvc.GetByID(goal.ID, testGroupID)
	if err != nil {
		t.Fatalf("ambil detail pot: %v", err)
	}
	if !nearly(detail.SavedAmount, 100_000) {
		t.Errorf("saldo pot: got %.2f; want 100000", detail.SavedAmount)
	}
}

// Menghapus pot yang masih bersaldo membuat uangnya lenyap dari pembukuan
// tanpa jejak penarikan — harus ditolak.
func TestSavingsDeleteGoalWithBalanceIsRejected(t *testing.T) {
	goal := newTestGoal(t, nil)
	deposit(t, goal.ID, 75_000)

	err := testSavingsSvc.Delete(goal.ID, testGroupID)
	if !errors.Is(err, service.ErrSavingsNotEmpty) {
		t.Fatalf("got %v; want ErrSavingsNotEmpty", err)
	}

	if _, err := testSavingsSvc.GetByID(goal.ID, testGroupID); err != nil {
		t.Errorf("pot seharusnya masih ada setelah penghapusan ditolak: %v", err)
	}
}

// Transaksi hasil setoran tidak boleh dihapus lewat endpoint transaksi biasa;
// kalau bisa, saldo pot langsung melenceng dari ledger.
func TestSavingsTransferTransactionCannotBeDeletedDirectly(t *testing.T) {
	goal := newTestGoal(t, nil)
	entry := deposit(t, goal.ID, 50_000)

	err := testTxSvc.Delete(*entry.TransactionID, testGroupID)
	if !errors.Is(err, repository.ErrTransferTransaction) {
		t.Fatalf("got %v; want ErrTransferTransaction", err)
	}

	if _, err := testTxSvc.GetByID(*entry.TransactionID, testGroupID); err != nil {
		t.Errorf("transaksi seharusnya masih hidup setelah penghapusan ditolak: %v", err)
	}
}

// Membatalkan mutasi lewat jalur yang benar harus mengembalikan saldo pot DAN
// men-soft-delete transaksinya.
func TestSavingsDeleteEntryRestoresBalanceAndRemovesTransaction(t *testing.T) {
	goal := newTestGoal(t, nil)
	entry := deposit(t, goal.ID, 120_000)
	txnID := *entry.TransactionID

	if err := testSavingsSvc.DeleteEntry(entry.ID, goal.ID, testGroupID); err != nil {
		t.Fatalf("batalkan setoran: %v", err)
	}

	detail, err := testSavingsSvc.GetByID(goal.ID, testGroupID)
	if err != nil {
		t.Fatalf("ambil detail pot: %v", err)
	}
	if !nearly(detail.SavedAmount, 0) {
		t.Errorf("saldo pot: got %.2f; want 0", detail.SavedAmount)
	}

	if _, err := testTxSvc.GetByID(txnID, testGroupID); err == nil {
		t.Error("transaksi masih terbaca; seharusnya sudah ter-soft-delete bersama mutasinya")
	}
}

// Status pot mengikuti saldo: 'completed' saat target tercapai, kembali
// 'active' saat turun lagi di bawah target.
func TestSavingsStatusFollowsBalance(t *testing.T) {
	target := 200_000.0
	goal := newTestGoal(t, &target)

	if goal.Status != repository.SavingsStatusActive {
		t.Fatalf("status awal: got %q; want %q", goal.Status, repository.SavingsStatusActive)
	}

	deposit(t, goal.ID, 200_000)

	detail, err := testSavingsSvc.GetByID(goal.ID, testGroupID)
	if err != nil {
		t.Fatalf("ambil detail pot: %v", err)
	}
	if detail.Status != repository.SavingsStatusCompleted {
		t.Errorf("status setelah target tercapai: got %q; want %q",
			detail.Status, repository.SavingsStatusCompleted)
	}
	if detail.Progress == nil || !nearly(*detail.Progress, 1) {
		t.Errorf("progress: got %v; want 1", detail.Progress)
	}

	if _, err := testSavingsSvc.Withdraw(service.SavingsEntryInput{
		GoalID:  goal.ID,
		GroupID: testGroupID,
		UserID:  testUserID,
		Amount:  50_000,
	}); err != nil {
		t.Fatalf("tarik tabungan: %v", err)
	}

	detail, err = testSavingsSvc.GetByID(goal.ID, testGroupID)
	if err != nil {
		t.Fatalf("ambil detail pot: %v", err)
	}
	if detail.Status != repository.SavingsStatusActive {
		t.Errorf("status setelah turun di bawah target: got %q; want %q",
			detail.Status, repository.SavingsStatusActive)
	}
}

// Inti dari keputusan desain Opsi C: setoran menggerakkan saldo kas, tapi TIDAK
// boleh muncul sebagai pengeluaran di laporan — ia masuk baris total_savings.
func TestSavingsExcludedFromIncomeExpenseReport(t *testing.T) {
	goal := newTestGoal(t, nil)

	today := time.Now().Format("2006-01-02")
	before, err := testSummarySvc.GetReport(testGroupID, "monthly", today, today)
	if err != nil {
		t.Fatalf("ambil laporan awal: %v", err)
	}

	deposit(t, goal.ID, 300_000)

	after, err := testSummarySvc.GetReport(testGroupID, "monthly", today, today)
	if err != nil {
		t.Fatalf("ambil laporan akhir: %v", err)
	}

	if !nearly(before.TotalExpense, after.TotalExpense) {
		t.Errorf("total_expense berubah dari %.2f jadi %.2f — setoran tabungan bocor ke laporan pengeluaran",
			before.TotalExpense, after.TotalExpense)
	}
	if !nearly(before.TotalIncome, after.TotalIncome) {
		t.Errorf("total_income berubah dari %.2f jadi %.2f", before.TotalIncome, after.TotalIncome)
	}
	if diff := after.TotalSavings - before.TotalSavings; !nearly(diff, 300_000) {
		t.Errorf("kenaikan total_savings: got %.2f; want 300000", diff)
	}
}

func TestSavingsGoalNotFoundForOtherGroup(t *testing.T) {
	requireSavingsEnv(t)

	// UUID valid yang dijamin bukan milik siapa pun.
	_, err := testSavingsSvc.GetByID("00000000-0000-0000-0000-000000000000", testGroupID)
	if !errors.Is(err, service.ErrSavingsNotFound) {
		t.Fatalf("got %v; want ErrSavingsNotFound", err)
	}
}

// nearly membandingkan nominal dengan toleransi satu sen — kolomnya
// DECIMAL(15,2) tapi discan ke float64.
func nearly(a, b float64) bool {
	d := a - b
	return d > -0.01 && d < 0.01
}
