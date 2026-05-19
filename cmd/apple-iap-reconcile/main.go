package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dundunHa/go-serverhttp-template/internal/config"
	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/service/payment"
	logpkg "github.com/dundunHa/go-serverhttp-template/pkg/log"
)

func main() {
	from := flag.String("from", "", "history start time RFC3339 (e.g. 2026-05-01T00:00:00Z)")
	to := flag.String("to", "", "history end time RFC3339")
	originalTx := flag.String("original-transaction-id", "", "filter by original transaction id (optional)")
	pageToken := flag.String("page-token", "", "pagination token from previous run (optional)")
	dsn := flag.String("dsn", os.Getenv("DB_DSN"), "Postgres DSN (default: $DB_DSN)")
	flag.Parse()

	if *dsn == "" {
		fail("--dsn or $DB_DSN required")
	}

	startedAt := time.Now()

	conf, err := config.LoadConfig()
	if err != nil {
		fail(fmt.Sprintf("load config: %v", err))
	}
	logpkg.InitLogger(conf.AppEnv, conf.Log)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fail(fmt.Sprintf("connect db: %v", err))
	}
	defer pool.Close()

	subscriptionDAO := dao.NewSubscriptionDAO(pool)
	tokens := payment.NewTokenService(subscriptionDAO)

	catalog, err := payment.NewCatalog(conf.AppleIAP, conf.AppEnv)
	if err != nil {
		fail(fmt.Sprintf("apple iap not configured: %v", err))
	}

	webhookVerifier, err := payment.NewAppleWebhookVerifier(catalog)
	if err != nil {
		fail(fmt.Sprintf("webhook verifier: %v", err))
	}
	webhook := payment.NewAppleWebhookService(catalog, webhookVerifier, tokens, subscriptionDAO)

	reconciler, err := payment.NewAppleReconciler(catalog)
	if err != nil {
		fail(fmt.Sprintf("reconciler: %v", err))
	}

	svc := payment.NewAppleReconcileService(reconciler, webhook)

	req := payment.NotificationHistoryRequest{
		OriginalTransactionID: *originalTx,
		PaginationToken:       *pageToken,
	}
	if *from != "" {
		t, err := time.Parse(time.RFC3339, *from)
		if err != nil {
			fail(fmt.Sprintf("--from invalid: %v", err))
		}
		req.StartDate = t
	}
	if *to != "" {
		t, err := time.Parse(time.RFC3339, *to)
		if err != nil {
			fail(fmt.Sprintf("--to invalid: %v", err))
		}
		req.EndDate = t
	}

	res, err := svc.Replay(ctx, req)
	if err != nil {
		fail(fmt.Sprintf("replay: %v", err))
	}

	slog.Info("apple iap reconcile complete",
		"replayed", res.Replayed,
		"failed", res.Failed,
		"next_page_token", res.NextPageToken,
		"errors_sample", res.LastErrors,
		"elapsed", time.Since(startedAt),
	)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
