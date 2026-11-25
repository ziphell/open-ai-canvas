package app

import (
	"os"
	"testing"
	"yingce/backend/internal/model"
	"yingce/backend/internal/providercatalog"
)

func TestMetasoCatalogBuild(t *testing.T) {
	body, err := os.ReadFile("../../../scripts/provider-catalog/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, warnings, err := providercatalog.Parse(body, providercatalog.DefaultLogoBase)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("parse: %v, %v", err, warnings)
	}
	var provider providercatalog.Provider
	found := false
	for _, item := range catalog.Providers {
		if item.ID == "metaso" {
			provider, found = item, true
			break
		}
	}
	if !found || len(provider.Models) == 0 {
		t.Fatalf("metaso provider missing: %#v", catalog.Providers)
	}
	svc, _ := newChannelModelTestService(t)
	entry := provider.Models[0]
	item, tiers, err := svc.buildCatalogChannelModel(&model.ModelChannel{ID: "test"}, entry, 3000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if item.ChannelLabel != "MiniMax-H3" || len(item.Tags) != 2 || item.Icon != "minimax" {
		t.Fatalf("metadata: %#v", item)
	}
	if len(tiers) != 3 {
		t.Fatalf("tiers: %d", len(tiers))
	}
	for i, cost := range []int64{75000, 90000, 180000} {
		if tiers[i].CostPricing.UnitPriceMicrocredits != cost || tiers[i].BillingMode != "per_second" {
			t.Fatalf("tier: %#v", tiers[i])
		}
	}
}
