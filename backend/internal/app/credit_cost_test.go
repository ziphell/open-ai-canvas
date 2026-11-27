package app

import (
	"encoding/json"
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

func TestCreditCostSnapshotPersistsWithoutChangingSalesOrPublicResponses(t *testing.T) {
	svc, db := newChannelModelTestService(t)
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.BillingOrder{}); err != nil {
		t.Fatal(err)
	}
	ensureTestUsers(t, db, "user")
	admin := &model.User{ID: "admin", Role: model.UserRoleAdmin}
	channel := model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Name: "供应渠道", Enabled: true, ModelsJSON: `[]`}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	req := ChannelModelRequest{
		ModelKey: "model-a", DisplayName: "产品模型", ChannelLabel: "优惠渠道", Capability: "text", Protocol: string(model.ChannelInterfaceChatCompletion),
		CapabilityConfig: DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), "model-a"),
		PriceTiers:       []ChannelModelPriceTierRequest{{BillingMode: "fixed_request", UnitPriceMicrocredits: 900_000, PriceConfigured: true, CostPricing: model.CreditCostPricing{Configured: true, UnitPriceMicrocredits: 123_456}}},
	}
	for _, actor := range []*model.User{nil, {ID: "user", Role: model.UserRoleUser}} {
		if _, err := svc.SaveAdminChannelModel(actor, channel.ID, "", req); err == nil {
			t.Fatal("non-admin wrote cost")
		}
		if _, err := svc.AdminChannelModels(actor, channel.ID); err == nil {
			t.Fatal("non-admin read cost")
		}
		if _, err := svc.AdminAPICallLogs(actor, APICallLogQuery{}); err == nil {
			t.Fatal("non-admin read request cost")
		}
	}
	saved, err := svc.SaveAdminChannelModel(admin, channel.ID, "", req)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.AdminChannelModels(admin, channel.ID)
	if err != nil || len(rows) != 1 || rows[0].PriceTiers[0].CostPricing.UnitPriceMicrocredits != 123_456 {
		t.Fatalf("persisted cost: %#v %v", rows, err)
	}
	order, err := svc.newBillingOrder("user", "task", "request", channel.ID, saved.ModelKey, "text", "test", 1, tokenBillingEstimate{})
	if err != nil {
		t.Fatal(err)
	}
	if order.AmountMicrocredits != 900_000 || order.CostPricing.UnitPriceMicrocredits != 123_456 {
		t.Fatalf("sales/cost snapshot: %#v", order)
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatal(err)
	}
	req.PriceTiers[0].CostPricing.UnitPriceMicrocredits = 654_321
	req.ExpectedUpdatedAt, req.ExpectedPriceVersion = saved.UpdatedAt, saved.PriceVersion
	if _, err := svc.SaveAdminChannelModel(admin, channel.ID, saved.ID, req); err != nil {
		t.Fatal(err)
	}
	var historical model.BillingOrder
	if err := db.First(&historical, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	historical.Status = model.BillingStatusSettled
	cost, err := billingCreditCost(historical)
	if err != nil || cost == nil || *cost != 123_456 {
		t.Fatalf("historical cost changed: %v %v", cost, err)
	}
	catalog, err := svc.ModelCatalog(testGroupAccess(svc), nil)
	if err != nil {
		t.Fatal(err)
	}
	public := publicChannel(channel, false, []model.ChannelModel{*saved})
	for _, value := range []any{saved, saved.PriceTiers, order, catalog, public, quoteFromOrder("", order, tokenBillingEstimate{})} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"costPricing", "costConfigured", "CostPricing", "123456", "654321"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("cost leaked into %T: %s", value, encoded)
			}
		}
	}
	// The catalog must not query logical models, even if their old feature flag is enabled.
	if err := db.Create(&model.SystemSetting{Key: featureAvailabilitySettingKey, ValueJSON: `{"frontendModelsEnabled":true}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&model.LogicalModel{}); err != nil {
		t.Fatal(err)
	}
	catalog, err = svc.ModelCatalog(testGroupAccess(svc), nil)
	if err != nil || catalog.Source != ModelCatalogSourceSystem || len(catalog.Channels) != 1 {
		t.Fatalf("catalog depends on logical models: %#v %v", catalog, err)
	}
}

func TestBillingCreditCostUsesExactUsageAndIgnoresSalesMultiplier(t *testing.T) {
	for _, tc := range []struct {
		name, mode, capability                         string
		quantity, input, output, cached, formula, want int64
		usage                                          bool
	}{
		{name: "request", mode: "fixed_request", quantity: 1, want: 90_000},
		{name: "seconds", mode: "per_second", quantity: 15, want: 1_350_000},
		{name: "text cached tokens", mode: "token", capability: "text", input: 1_000_000, output: 500_000, cached: 200_000, usage: true, want: 1_820_000},
		{name: "video formula", mode: "token", capability: "video", formula: 250_000, want: 500_000},
		{name: "video provider", mode: "token", capability: "video", output: 100_000, formula: 250_000, usage: true, want: 200_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := model.BillingOrder{Status: model.BillingStatusSettled, Capability: tc.capability, InputTokens: tc.input, OutputTokens: tc.output, CachedTokens: tc.cached, UsageAvailable: tc.usage, MultiplierBasisPoints: 30_000, ChargeLimitMicrocredits: 1,
				BillingCostSnapshot: model.BillingCostSnapshot{CostBillingMode: tc.mode, CostQuantity: tc.quantity, CostVideoFormulaTokens: tc.formula,
					CostPricing: model.CreditCostPricing{Configured: true, UnitPriceMicrocredits: 90_000, InputTokenPriceMicrocredits: 1_000_000, OutputTokenPriceMicrocredits: 2_000_000, CachedTokenPriceMicrocredits: 100_000}}}
			got, err := billingCreditCost(order)
			if err != nil || got == nil || *got != tc.want {
				t.Fatalf("cost=%v want=%d err=%v", got, tc.want, err)
			}
			order.CostPricing = model.CreditCostPricing{Configured: true}
			got, err = billingCreditCost(order)
			if err != nil || got == nil || *got != 0 {
				t.Fatal("explicit zero cost must remain available")
			}
			for _, status := range []model.BillingStatus{model.BillingStatusReserved, model.BillingStatusRunning, model.BillingStatusRefunded, model.BillingStatusUncertain} {
				order.Status = status
				got, err = billingCreditCost(order)
				if err != nil || got != nil {
					t.Fatalf("unsettled cost fabricated for %s", status)
				}
			}
		})
	}
}

func TestZeroPricedTokenOrderKeepsCostSnapshot(t *testing.T) {
	svc, db := newChannelModelTestService(t)
	if err := db.AutoMigrate(&model.CreditAccount{}, &model.BillingOrder{}, &model.ApiCallLog{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	ensureTestUsers(t, db, "user")
	admin := &model.User{ID: "admin", Role: model.UserRoleAdmin}
	channel := model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Name: "供应渠道", Enabled: true, ModelsJSON: `[]`}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	saved, err := svc.SaveAdminChannelModel(admin, channel.ID, "", ChannelModelRequest{
		ModelKey: "free-text", DisplayName: "免费模型", ChannelLabel: "优惠渠道", Capability: "text", Protocol: string(model.ChannelInterfaceChatCompletion),
		CapabilityConfig: DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), "free-text"),
		PriceTiers: []ChannelModelPriceTierRequest{{
			BillingMode: "token", PriceConfigured: true, Enabled: boolPtr(true),
			CostPricing: model.CreditCostPricing{Configured: true, InputTokenPriceMicrocredits: 1_000_000, OutputTokenPriceMicrocredits: 2_000_000},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := svc.newBillingOrder("user", "task", "request", channel.ID, saved.ModelKey, "text", "test", 1, tokenBillingEstimate{InputTokens: 100, OutputTokens: 50})
	if err != nil {
		t.Fatal(err)
	}
	if order.AmountMicrocredits != 0 || order.CostPricing.InputTokenPriceMicrocredits != 1_000_000 {
		t.Fatalf("zero price snapshot = %#v", order)
	}
	if err := db.Create(&model.CreditAccount{UserID: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ApiCallLog{
		ID: "zero-price-log", BillingOrderID: order.ID, UserID: "user", ChannelID: channel.ID, RequestKind: "create", Billable: true,
		Status: model.ApiCallStatusSucceeded, UsageAvailable: true, InputTokens: 100, OutputTokens: 50,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SettleBillingOrder(order.ID, "provider-request"); err != nil {
		t.Fatal(err)
	}
	var settled model.BillingOrder
	if err := db.First(&settled, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	cost, err := billingCreditCost(settled)
	if err != nil || cost == nil || *cost != 200 {
		t.Fatalf("zero-priced cost = %v, %v", cost, err)
	}
}

func TestBillingCreditCostKeepsZeroPricedOrdersInLedger(t *testing.T) {
	zeroCost := model.CreditCostPricing{Configured: true}
	costedZeroPrice := model.BillingOrder{
		Status: model.BillingStatusSettled, Capability: "text", BillingMode: "token",
		BillingCostSnapshot: model.BillingCostSnapshot{CostBillingMode: "token", CostPricing: zeroCost},
	}
	if cost, err := billingCreditCost(costedZeroPrice); err != nil || cost == nil || *cost != 0 {
		t.Fatalf("zero cost without usage = %v, %v", cost, err)
	}

	costedZeroPrice.UsageAvailable = true
	costedZeroPrice.InputTokens, costedZeroPrice.OutputTokens = 1_000_000, 500_000
	costedZeroPrice.CostPricing = model.CreditCostPricing{Configured: true, OutputTokenPriceMicrocredits: 2_000_000}
	if cost, err := billingCreditCost(costedZeroPrice); err != nil || cost == nil || *cost != 1_000_000 {
		t.Fatalf("zero sale price cost = %v, %v", cost, err)
	}

	video := model.BillingOrder{
		Status: model.BillingStatusSettled, Capability: "video", BillingMode: "token",
		BillingCostSnapshot: model.BillingCostSnapshot{CostBillingMode: "token", CostPricing: model.CreditCostPricing{Configured: true}},
	}
	if cost, err := billingCreditCost(video); err != nil || cost == nil || *cost != 0 {
		t.Fatalf("zero video cost = %v, %v", cost, err)
	}
	video.CostPricing.OutputTokenPriceMicrocredits = 1_000_000
	if cost, err := billingCreditCost(video); err != nil || cost != nil {
		t.Fatalf("video cost without usage fabricated: %v, %v", cost, err)
	}
}

func TestCreditCostRejectsInvalidPrices(t *testing.T) {
	for _, price := range []int64{-1, 1_000_000_000_001} {
		if err := validateCreditCostPricing("text", "fixed_request", model.CreditCostPricing{Configured: true, UnitPriceMicrocredits: price}); err == nil {
			t.Fatalf("accepted cost %d", price)
		}
	}
	if err := validateCreditCostPricing("video", "token", model.CreditCostPricing{Configured: true, InputTokenPriceMicrocredits: 1}); err == nil {
		t.Fatal("accepted video input cost")
	}
}
