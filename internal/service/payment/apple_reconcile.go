package payment

import (
	"context"
	"fmt"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
)

// AppleReconcileService 把 Apple Notification History 复盘到本地 reducer 上，
// 用于补齐 webhook 漏投或 JWS 验证失败留下的状态漂移。
//
// 每条 history 记录都通过同一个 webhook reducer 路径，notification_uuid 的 UNIQUE
// 约束保证多次 replay 不重复执行业务变更。
type AppleReconcileService struct {
	reconciler AppleReconciler
	webhook    *AppleWebhookService
}

// NewAppleReconcileService 构造 reconcile service。任一依赖为 nil 时调用都返回 ErrNotConfigured。
func NewAppleReconcileService(reconciler AppleReconciler, webhook *AppleWebhookService) *AppleReconcileService {
	return &AppleReconcileService{
		reconciler: reconciler,
		webhook:    webhook,
	}
}

// ReplayResult 是单次 Replay 调用的统计输出，便于 runbook / CLI 决定是否继续翻页。
type ReplayResult struct {
	Replayed      int
	Failed        int
	NextPageToken string
	LastErrors    []string
}

// Replay 拉取一段时间窗 / pagination token 的 notification history，并依次走 webhook reducer。
//
// 调用方负责按 NextPageToken 自行翻页；本方法每次只处理 reconciler 单页结果。
func (s *AppleReconcileService) Replay(ctx context.Context, req NotificationHistoryRequest) (*ReplayResult, error) {
	if s == nil || s.reconciler == nil || s.webhook == nil {
		return nil, ErrNotConfigured
	}

	events, nextPage, err := s.reconciler.GetNotificationHistory(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("apple reconcile: fetch notification history: %w", err)
	}

	out := &ReplayResult{NextPageToken: nextPage}
	for i := range events {
		ev := events[i]
		if err := s.webhook.dao.InTx(ctx, func(qtx dao.SubscriptionTx) error {
			return s.webhook.processInTx(ctx, qtx, &ev)
		}); err != nil {
			out.Failed++
			out.LastErrors = append(out.LastErrors, err.Error())
			continue
		}
		out.Replayed++
	}
	return out, nil
}
