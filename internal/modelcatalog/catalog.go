package modelcatalog

import (
	"context"
	"sort"
)

// Source exposes provider models.
type Source interface {
	Name() string
	ListModels(ctx context.Context) ([]Model, error)
}

// Model describes a provider model entry.
type Model struct {
	Name         string
	Description  string
	Capabilities []string
	Provider     string
	Tags         map[string]string
}

// Catalog aggregates models from multiple sources.
type Catalog struct {
	Entries map[string][]Model
}

// Build constructs a catalog by querying each source sequentially.
func Build(ctx context.Context, whitelist map[string]struct{}, sources ...Source) (Catalog, error) {
	result := Catalog{Entries: make(map[string][]Model)}
	for _, src := range sources {
		models, err := src.ListModels(ctx)
		if err != nil {
			return Catalog{}, err
		}
		filtered := filterModels(models, whitelist)
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Name < filtered[j].Name
		})
		result.Entries[src.Name()] = filtered
	}
	return result, nil
}

func filterModels(models []Model, whitelist map[string]struct{}) []Model {
	if len(whitelist) == 0 {
		return models
	}
	out := make([]Model, 0, len(models))
	for _, m := range models {
		if _, ok := whitelist[m.Name]; ok {
			out = append(out, m)
		}
	}
	return out
}
