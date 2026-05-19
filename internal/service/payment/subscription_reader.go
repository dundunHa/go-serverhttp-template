package payment

import (
	"context"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// SubscriptionReader 给 GET /users/me 提供 provider-neutral 的订阅状态视图。
type SubscriptionReader struct {
	dao     dao.SubscriptionDAO
	catalog *Catalog
	now     func() time.Time
}

// NewSubscriptionReader 构造 reader。catalog 为 nil 时 reader 退化为“总是 NONE”。
func NewSubscriptionReader(d dao.SubscriptionDAO, catalog *Catalog) *SubscriptionReader {
	return &SubscriptionReader{
		dao:     d,
		catalog: catalog,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// LoadSubscriptionInfo 应用 plan "## Entitlement Model" -> "/users/me selection rule":
//  1. 仅看 catalog.AllowedEntitlementEnvironments;
//  2. 有有效权益就返回 highest level / latest effective end / latest last_event_at（DAO 已排序）;
//  3. 无有效权益但有终止行就返回 EXPIRED;
//  4. 完全没行就返回 NONE。
func (r *SubscriptionReader) LoadSubscriptionInfo(ctx context.Context, userID int64) (model.SubscriptionInfo, error) {
	if r == nil || r.dao == nil || r.catalog == nil {
		return model.SubscriptionInfo{Status: "NONE"}, nil
	}
	if userID <= 0 {
		return model.SubscriptionInfo{Status: "NONE"}, nil
	}

	envs := r.catalog.AllowedEntitlementEnvironments()
	if len(envs) == 0 {
		return model.SubscriptionInfo{Status: "NONE"}, nil
	}

	rows, err := r.dao.ListSubscriptionsForUserEntitlement(ctx, userID, envs)
	if err != nil {
		return model.SubscriptionInfo{}, err
	}
	if len(rows) == 0 {
		return model.SubscriptionInfo{Status: "NONE"}, nil
	}

	now := r.now()
	for _, sub := range rows {
		status := APIStatusForSubscription(sub, now)
		if status == "ACTIVE" || status == "CANCELED" {
			info := subscriptionInfoFromRow(sub, now)
			info.Status = status
			return info, nil
		}
	}

	terminal := rows[0]
	info := subscriptionInfoFromRow(terminal, now)
	info.Status = "EXPIRED"
	info.SubscribeLevel = 0
	return info, nil
}
