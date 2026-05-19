package payment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/config"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// AppEnvProd 与 config.AppEnv 中表示生产部署的字面值保持一致；
// 生产环境会触发额外的安全校验（禁用 P8Path、禁用 raw payload 落库等）。
const AppEnvProd = "prod"

// Environment 是 model.AppleEnvironment 的 payment 包内别名。两个名字指向同一类型，
// 跨 dao / payment / model 包传递时无需显式转换。
type Environment = model.AppleEnvironment

const (
	EnvProduction = model.AppleEnvProduction
	EnvSandbox    = model.AppleEnvSandbox
)

type Product struct {
	PlanID              string      `json:"plan_id"`
	ProductID           string      `json:"product_id"`
	Level               int         `json:"level"`
	Environment         Environment `json:"environment"`
	SubscriptionGroupID string      `json:"subscription_group_id,omitempty"`
}

type catalogKey struct {
	ProductID   string
	Environment Environment
}

// Catalog 是 Apple IAP 配置的不可变运行期视图。
// 构造完成后所有字段只读；通过 NewCatalog 校验配置并填充。
//
// 不要在 catalog 之外读取环境变量；catalog 是唯一的事实源。
type Catalog struct {
	products              []Product
	byProductIDEnv        map[catalogKey]Product
	allowedEntitlement    map[Environment]struct{}
	entitlementOrder      []Environment
	enableSandboxFallback bool
	storeRawPayloads      bool
	webhookMaxBodyBytes   int
	appleAPITimeout       time.Duration
	bundleID              string
	issuerID              string
	keyID                 string
	privateKeyPEM         string
}

// NewCatalog 验证 Apple IAP 配置并构造只读 Catalog。
//
// 行为：
//   - 完全未配置（核心字段为空 + 无 products + 无 key 材料）→ ErrNotConfigured。
//     这一情况映射到 verify/account-token 路由的 503。
//   - 部分配置但语法/语义错误（products JSON 解析失败、(product_id,environment) 重复、
//     生产开启 raw payload 等）→ wrap ErrInvalidConfig。
//
// 生产安全规则（appEnv == "prod"）：
//   - StoreRawPayloads=true 直接拒绝。
//   - 仅 P8Path 而无 PrivateKey 直接拒绝（强制 secret store 注入）。
//   - 授权环境列表只含 Sandbox 而无 Production 直接拒绝。
//
// PrivateKey 优先于 P8Path；当 PrivateKey 非空时不会读取磁盘。
// 调用方不得日志输出返回的 catalog 字段（私钥、产品 JSON 等）。
func NewCatalog(cfg config.AppleIAPConfig, appEnv string) (*Catalog, error) {
	hasCoreIdentity := cfg.BundleID != "" || cfg.IssuerID != "" || cfg.KeyID != ""
	hasKeyMaterial := cfg.PrivateKey != "" || cfg.P8Path != ""
	hasProducts := strings.TrimSpace(cfg.Products) != ""
	hasEnv := strings.TrimSpace(cfg.EntitlementEnvironments) != ""

	if !hasCoreIdentity && !hasKeyMaterial && !hasProducts && !hasEnv {
		return nil, ErrNotConfigured
	}

	if cfg.BundleID == "" || cfg.IssuerID == "" || cfg.KeyID == "" {
		return nil, ErrNotConfigured
	}
	if !hasKeyMaterial {
		return nil, ErrNotConfigured
	}
	if !hasProducts {
		return nil, ErrNotConfigured
	}
	if !hasEnv {
		return nil, ErrNotConfigured
	}

	if appEnv == AppEnvProd && cfg.StoreRawPayloads {
		return nil, fmt.Errorf("storing raw payloads in production is forbidden: %w", ErrInvalidConfig)
	}

	allowed, order, err := parseEntitlementEnvironments(cfg.EntitlementEnvironments)
	if err != nil {
		return nil, err
	}
	if appEnv == AppEnvProd {
		_, hasProd := allowed[EnvProduction]
		_, hasSandbox := allowed[EnvSandbox]
		if hasSandbox && !hasProd {
			return nil, fmt.Errorf("production entitlement environments must include Production: %w", ErrInvalidConfig)
		}
	}

	products, byKey, err := parseProducts(cfg.Products)
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return nil, ErrNotConfigured
	}

	pem, err := resolvePrivateKey(cfg, appEnv)
	if err != nil {
		return nil, err
	}

	return &Catalog{
		products:              products,
		byProductIDEnv:        byKey,
		allowedEntitlement:    allowed,
		entitlementOrder:      order,
		enableSandboxFallback: cfg.EnableSandboxFallback,
		storeRawPayloads:      cfg.StoreRawPayloads,
		webhookMaxBodyBytes:   cfg.WebhookMaxBodyBytes,
		appleAPITimeout:       cfg.AppleAPITimeout,
		bundleID:              cfg.BundleID,
		issuerID:              cfg.IssuerID,
		keyID:                 cfg.KeyID,
		privateKeyPEM:         pem,
	}, nil
}

func parseEntitlementEnvironments(raw string) (map[Environment]struct{}, []Environment, error) {
	parts := strings.Split(raw, ",")
	allowed := make(map[Environment]struct{}, len(parts))
	order := make([]Environment, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		env := Environment(name)
		if env != EnvProduction && env != EnvSandbox {
			return nil, nil, fmt.Errorf("unknown entitlement environment %q: %w", name, ErrInvalidConfig)
		}
		if _, dup := allowed[env]; dup {
			continue
		}
		allowed[env] = struct{}{}
		order = append(order, env)
	}
	if len(allowed) == 0 {
		return nil, nil, ErrNotConfigured
	}
	return allowed, order, nil
}

func parseProducts(raw string) ([]Product, map[catalogKey]Product, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil, ErrNotConfigured
	}
	var products []Product
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&products); err != nil {
		return nil, nil, fmt.Errorf("parse APPLE_IAP_PRODUCTS json: %w", joinInvalidConfig(err))
	}

	byKey := make(map[catalogKey]Product, len(products))
	for i, p := range products {
		if p.PlanID == "" || p.ProductID == "" || p.Environment == "" {
			return nil, nil, fmt.Errorf("APPLE_IAP_PRODUCTS[%d]: plan_id, product_id, environment required: %w", i, ErrInvalidConfig)
		}
		if p.Environment != EnvProduction && p.Environment != EnvSandbox {
			return nil, nil, fmt.Errorf("APPLE_IAP_PRODUCTS[%d]: unknown environment %q: %w", i, p.Environment, ErrInvalidConfig)
		}
		key := catalogKey{ProductID: p.ProductID, Environment: p.Environment}
		if _, dup := byKey[key]; dup {
			return nil, nil, fmt.Errorf("APPLE_IAP_PRODUCTS[%d]: duplicate %s/%s: %w", i, p.ProductID, p.Environment, errors.Join(ErrDuplicateProduct, ErrInvalidConfig))
		}
		byKey[key] = p
	}
	return products, byKey, nil
}

func resolvePrivateKey(cfg config.AppleIAPConfig, appEnv string) (string, error) {
	if cfg.PrivateKey != "" {
		return cfg.PrivateKey, nil
	}
	if cfg.P8Path == "" {
		return "", ErrNotConfigured
	}
	if appEnv == AppEnvProd {
		return "", fmt.Errorf("APPLE_IAP_P8_PATH is dev-only; provide APPLE_IAP_PRIVATE_KEY via secret store in prod: %w", ErrInvalidConfig)
	}
	data, err := os.ReadFile(cfg.P8Path)
	if err != nil {
		return "", fmt.Errorf("read APPLE_IAP_P8_PATH: %w", joinInvalidConfig(err))
	}
	return string(data), nil
}

// joinInvalidConfig 让被包装的错误同时满足 errors.Is(err, ErrInvalidConfig)，
// 同时保留原始错误（json/io 错误）便于调试。
func joinInvalidConfig(err error) error {
	if err == nil {
		return ErrInvalidConfig
	}
	return errors.Join(err, ErrInvalidConfig)
}

// Lookup 按 (productID, env) 在目录中查找产品；未命中返回 ErrUnknownProduct。
func (c *Catalog) Lookup(productID string, env Environment) (Product, error) {
	if c == nil {
		return Product{}, ErrNotConfigured
	}
	p, ok := c.byProductIDEnv[catalogKey{ProductID: productID, Environment: env}]
	if !ok {
		return Product{}, ErrUnknownProduct
	}
	return p, nil
}

// IsEntitlementEnvironment 检查 env 是否在允许授权的白名单内。
func (c *Catalog) IsEntitlementEnvironment(env Environment) bool {
	if c == nil {
		return false
	}
	_, ok := c.allowedEntitlement[env]
	return ok
}

func (c *Catalog) BundleID() string               { return c.bundleID }
func (c *Catalog) IssuerID() string               { return c.issuerID }
func (c *Catalog) KeyID() string                  { return c.keyID }
func (c *Catalog) PrivateKeyPEM() string          { return c.privateKeyPEM }
func (c *Catalog) AppleAPITimeout() time.Duration { return c.appleAPITimeout }
func (c *Catalog) WebhookMaxBodyBytes() int       { return c.webhookMaxBodyBytes }
func (c *Catalog) EnableSandboxFallback() bool    { return c.enableSandboxFallback }
func (c *Catalog) StoreRawPayloads() bool         { return c.storeRawPayloads }

// Products 返回 catalog 内产品的拷贝，调用方可自由修改返回切片而不影响内部状态。
func (c *Catalog) Products() []Product {
	if c == nil || len(c.products) == 0 {
		return nil
	}
	out := make([]Product, len(c.products))
	copy(out, c.products)
	return out
}

// AllowedEntitlementEnvironments 返回声明顺序保留的允许授权环境列表的拷贝。
func (c *Catalog) AllowedEntitlementEnvironments() []Environment {
	if c == nil || len(c.entitlementOrder) == 0 {
		return nil
	}
	out := make([]Environment, len(c.entitlementOrder))
	copy(out, c.entitlementOrder)
	return out
}
