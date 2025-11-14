package provider

import (
	"context"

	"github.com/fbettag/pfui/internal/modelcatalog"
)

// AsCatalogSources converts providers into modelcatalog sources.
func AsCatalogSources(reg Registry) []modelcatalog.Source {
	var sources []modelcatalog.Source
	for _, p := range reg.Providers() {
		sources = append(sources, providerSource{provider: p})
	}
	return sources
}

type providerSource struct {
	provider Provider
}

func (s providerSource) Name() string {
	return s.provider.Name()
}

func (s providerSource) ListModels(ctx context.Context) ([]modelcatalog.Model, error) {
	models, err := s.provider.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]modelcatalog.Model, 0, len(models))
	for _, m := range models {
		out = append(out, modelcatalog.Model{
			Name:         m.Name,
			Description:  m.Description,
			Capabilities: m.Capabilities,
			Provider:     s.provider.Name(),
			Tags:         m.Tags,
		})
	}
	return out, nil
}
