package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"
	"finarch/internal/interface/apiv1"
)

// main runs CLI demo for fund management system.
func main() {
	ctx := context.Background()

	dsn := os.Getenv("FINARCH_DB")
	if dsn == "" {
		dsn = "finarch.db"
	}

	database, err := db.OpenSQLite(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	switch command {
	case "init":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		fmt.Println("migration done")
	case "seed":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runSeed(ctx, database); err != nil {
			log.Fatal(err)
		}
		fmt.Println("seed done")
	case "addtx":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runAddTx(ctx, database, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "match":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runMatch(ctx, database, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "reimburse":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runReimburse(ctx, database, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "balance":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runBalance(ctx, database); err != nil {
			log.Fatal(err)
		}
	case "list":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runList(ctx, database); err != nil {
			log.Fatal(err)
		}
	case "serve":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		addr := "0.0.0.0:8080"
		if len(os.Args) >= 3 {
			addr = os.Args[2]
		}
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "finarch-dev-secret-change-in-prod"
		}
		jwtSvc := auth.NewJWTService(jwtSecret)
		// Auth brute-force protection
		authLimiter := auth.NewIPRateLimiter(10, 60*time.Second)
		loginTracker := auth.NewLoginAttemptTracker(5, 15*time.Minute)
		// Cloudflare Turnstile CAPTCHA (set TURNSTILE_SECRET to enable)
		captchaVerifier := auth.NewTurnstileVerifier(os.Getenv("TURNSTILE_SECRET"))
		turnstileSiteKey := os.Getenv("TURNSTILE_SITE_KEY")
		userRepo := sqliterepo.NewSQLiteUserRepository(database)
		tagRepo := sqliterepo.NewSQLiteTagRepository(database)
		txSvc, reimSvc, matchSvc, txRepo, acctSvc := buildServicesWithRepo(database)
		// Email service (no-op in dev unless RESEND_API_KEY is set)
		appBaseURL := os.Getenv("APP_BASE_URL")
		if appBaseURL == "" {
			appBaseURL = "http://localhost:8080"
		}
		emailSvc := email.NewResendSender(os.Getenv("RESEND_API_KEY"), os.Getenv("RESEND_FROM_EMAIL"), appBaseURL)
		authSvc := service.NewAuthService(userRepo, jwtSvc, loginTracker, emailSvc, email.IsConfigured(), appBaseURL)
		statsSvc := service.NewStatsService(database)
		srv := apiv1.NewServer(addr, database, dsn, txRepo, tagRepo, txSvc, reimSvc, matchSvc, authSvc, statsSvc, jwtSvc, authLimiter, captchaVerifier, turnstileSiteKey, acctSvc, emailSvc)
		log.Printf("FinArch API v1: http://%s", addr)
		log.Fatal(srv.Run())
	default:
		printUsage()
	}
}

// runSeed inserts one project and demo transactions.
func runSeed(ctx context.Context, database *sql.DB) error {
	txSvc, _, _, projectRepo := buildServices(database)
	return seedData(ctx, projectRepo, txSvc)
}

// runAddTx creates one transaction from CLI args.
func runAddTx(ctx context.Context, database *sql.DB, args []string) error {
	if len(args) < 5 {
		return fmt.Errorf("addtx args: occurredAtUnix direction source category amountYuan [projectId] [note]")
	}
	txSvc, _, _, _ := buildServices(database)

	occurredAt := time.Unix(int64(mustParseFloat64(args[0])), 0)
	direction := model.Direction(args[1])
	source := model.Source(args[2])
	category := args[3]
	amount := model.Money(mustParseFloat64(args[4]))

	var projectID *string
	if len(args) >= 6 && args[5] != "" {
		projectID = &args[5]
	}
	note := ""
	if len(args) >= 7 {
		note = strings.Join(args[6:], " ")
	}

	t, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		OccurredAt: occurredAt,
		Direction:  direction,
		Source:     source,
		Category:   category,
		AmountYuan: amount,
		Currency:   "CNY",
		ProjectID:  projectID,
		Note:       note,
	})
	if err != nil {
		return err
	}
	fmt.Printf("created tx: id=%s amount=%.2f\n", t.ID, t.AmountYuan)
	return nil
}

// runMatch executes reimbursement matching.
func runMatch(ctx context.Context, database *sql.DB, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("match args: targetYuan toleranceYuan maxDepth limit [projectId]")
	}
	_, _, matchingSvc, _ := buildServices(database)

	target := model.Money(mustParseFloat64(args[0]))
	tolerance := model.Money(mustParseFloat64(args[1]))
	maxDepth, err := strconv.Atoi(args[2])
	if err != nil {
		return fmt.Errorf("invalid maxDepth: %w", err)
	}
	limit, err := strconv.Atoi(args[3])
	if err != nil {
		return fmt.Errorf("invalid limit: %w", err)
	}

	var projectID *string
	if len(args) >= 5 && args[4] != "" {
		projectID = &args[4]
	}

	// CLI is single-user; use empty string as the user scope.
	results, err := matchingSvc.Match(ctx, "", target, tolerance, maxDepth, projectID, limit)
	if err != nil {
		return err
	}
	for i, r := range results {
		fmt.Printf("%d) total=%.2f error=%.2f projectCount=%d itemCount=%d ids=%v\n", i+1, r.TotalYuan, r.AbsErrorYuan, r.ProjectCount, r.ItemCount, r.TransactionIDs)
	}
	if len(results) == 0 {
		fmt.Println("no match")
	}
	return nil
}

// runReimburse creates reimbursement for transaction IDs.
func runReimburse(ctx context.Context, database *sql.DB, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("reimburse args: applicant transactionIdsCsv [requestNo]")
	}
	_, reimbursementSvc, _, _ := buildServices(database)

	applicant := args[0]
	txIDs := splitCSV(args[1])
	requestNo := ""
	if len(args) >= 3 {
		requestNo = args[2]
	}

	reim, err := reimbursementSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		Applicant:      applicant,
		TransactionIDs: txIDs,
		RequestNo:      requestNo,
	})
	if err != nil {
		return err
	}
	fmt.Printf("reimbursement created: id=%s requestNo=%s total=%.2f\n", reim.ID, reim.RequestNo, reim.TotalYuan)
	return nil
}

// runBalance prints computed balances.
func runBalance(ctx context.Context, database *sql.DB) error {
	txSvc, _, _, _ := buildServices(database)
	// CLI is single-user; use empty string as the user scope.
	company, personal, err := txSvc.GetBalances(ctx, "")
	if err != nil {
		return err
	}
	fmt.Printf("company_balance_yuan=%.2f personal_outstanding_yuan=%.2f\n", company, personal)
	return nil
}

// runList prints all transactions to stdout.
func runList(ctx context.Context, database *sql.DB) error {
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	txs, err := txRepo.ListByUser(ctx, "", model.ModeWork)
	if err != nil {
		return err
	}
	if len(txs) == 0 {
		fmt.Println("no transactions")
		return nil
	}
	fmt.Printf("%-36s  %-10s  %-8s  %-8s  %-12s  %10s  %s\n",
		"ID", "日期", "方向", "来源", "分类", "金额(元)", "备注")
	for _, t := range txs {
		reimStatus := ""
		if t.Reimbursed {
			reimStatus = " [已报销]"
		}
		fmt.Printf("%-36s  %-10s  %-8s  %-8s  %-12s  %10.2f  %s%s\n",
			t.ID, t.OccurredAt.Format("2006-01-02"),
			string(t.Direction), string(t.Source),
			t.Category, t.AmountYuan.Float64(),
			t.Note, reimStatus)
	}
	return nil
}

func buildServicesWithRepo(database *sql.DB) (*service.TransactionService, *service.ReimbursementService, *service.MatchingService, *sqliterepo.SQLiteTransactionRepository, *service.AccountService) {
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)
	matchSvc := service.NewMatchingService(txRepo)
	acctSvc := service.NewAccountService(acctRepo)
	return txSvc, reimSvc, matchSvc, txRepo, acctSvc
}

func printUsage() {
	fmt.Println("usage:")
	fmt.Println("  cli init")
	fmt.Println("  cli seed")
	fmt.Println("  cli list")
	fmt.Println("  cli addtx occurredAtUnix direction source category amountYuan [projectId] [note]")
	fmt.Println("  cli match targetYuan toleranceYuan maxDepth limit [projectId]")
	fmt.Println("  cli reimburse applicant transactionIdsCsv [requestNo]")
	fmt.Println("  cli balance")
	fmt.Println("  cli serve [addr]   (默认 0.0.0.0:8080，启动 Web UI)")
}

func mustParseFloat64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Fatalf("invalid float64 %q: %v", s, err)
	}
	return v
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func buildServices(database *sql.DB) (*service.TransactionService, *service.ReimbursementService, *service.MatchingService, *sqliterepo.SQLiteProjectRepository) {
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	projectRepo := sqliterepo.NewSQLiteProjectRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)

	txSvc := service.NewTransactionService(txRepo, acctRepo)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)
	matchSvc := service.NewMatchingService(txRepo)
	return txSvc, reimSvc, matchSvc, projectRepo
}

func seedData(ctx context.Context, projectRepo *sqliterepo.SQLiteProjectRepository, txSvc *service.TransactionService) error {
	projectID := "PJT-001"
	err := projectRepo.Create(ctx, model.Project{
		ID:        projectID,
		Name:      "科研项目A",
		Code:      "PJT-001",
		CreatedAt: time.Now(),
	})
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		return err
	}

	seed := []service.CreateTransactionRequest{
		{OccurredAt: time.Now().AddDate(0, 0, -10), Direction: model.DirectionIncome, Source: model.SourceCompany, Category: "资金注入", AmountYuan: 5000},
		{OccurredAt: time.Now().AddDate(0, 0, -9), Direction: model.DirectionExpense, Source: model.SourceCompany, Category: "CNC", AmountYuan: 1200, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -8), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "钣金", AmountYuan: 800, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -7), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "3D打印", AmountYuan: 400, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -6), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "差旅", AmountYuan: 205, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -5), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "材料", AmountYuan: 595, ProjectID: &projectID},
	}

	for _, req := range seed {
		if _, err := txSvc.CreateTransaction(ctx, req); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
			return err
		}
	}
	return nil
}
