package assets

import (
	"context"
	"fmt"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/modregistry"
)

func (assetsModule) DevSeed(ctx context.Context, deps modregistry.Dependencies) error {
	existing, err := deps.Queries.ListAssets(ctx, db.ListAssetsParams{
		Q:          "",
		Status:     "",
		PageOffset: 0,
		PageSize:   1000,
	})
	if err != nil {
		return fmt.Errorf("list assets: %w", err)
	}
	byName := make(map[string]db.Asset, len(existing))
	for _, asset := range existing {
		byName[asset.Name] = asset
	}

	type assetSeed struct {
		Name        string
		Type        string
		Status      string
		Criticality string
		Owner       string
	}
	seeds := []assetSeed{
		{Name: "Identity Provider (SSO)", Type: "application", Status: "active", Criticality: "high", Owner: "Platform"},
		{Name: "Customer Data Warehouse", Type: "database", Status: "active", Criticality: "high", Owner: "Data Engineering"},
		{Name: "Endpoint Management Platform", Type: "service", Status: "active", Criticality: "medium", Owner: "IT Operations"},
		{Name: "Public API Gateway", Type: "infrastructure", Status: "planned", Criticality: "high", Owner: "Engineering"},
		{Name: "Legacy File Transfer Server", Type: "infrastructure", Status: "retired", Criticality: "low", Owner: "Operations"},
	}
	for _, seed := range seeds {
		if _, ok := byName[seed.Name]; ok {
			continue
		}
		_, createErr := deps.Queries.CreateAsset(ctx, db.CreateAssetParams{
			Name:        seed.Name,
			Description: "Seeded for realistic risk mapping and control linkage workflows.",
			AssetType:   seed.Type,
			Owner:       seed.Owner,
			Status:      seed.Status,
			Criticality: seed.Criticality,
		})
		if createErr != nil {
			return fmt.Errorf("create asset %q: %w", seed.Name, createErr)
		}
	}
	return nil
}
