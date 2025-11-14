package providersetup

import (
	"fmt"
	"os"
	"strings"

	"github.com/fbettag/pfui/internal/authstore"
	"github.com/fbettag/pfui/internal/config"
	"github.com/fbettag/pfui/internal/provider"
	"github.com/fbettag/pfui/internal/provider/anthropic"
	"github.com/fbettag/pfui/internal/provider/openai"
)

// DefaultRegistry builds a provider registry based on configuration toggles.
func DefaultRegistry(cfg config.Config) provider.Registry {
	creds, err := authstore.Snapshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pfui: unable to read credentials: %v\n", err)
	}
	var providers []provider.Provider
	if cfg.Providers.OpenAI.Enabled {
		token := creds.APIKeys["openai"]
		providers = append(providers, openai.New("", token))
	}
	if cfg.Providers.Anthropic.Enabled {
		token := creds.APIKeys["anthropic"]
		providers = append(providers, anthropic.New("", token))
	}
	custom, err := provider.LoadManifests()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pfui: unable to load custom providers: %v\n", err)
	} else {
		for _, manifest := range custom {
			if manifest.Token == "" {
				if key, ok := creds.APIKeys[manifest.Name]; ok {
					manifest.Token = key
				}
			}
			if prov := instantiateCustom(manifest); prov != nil {
				providers = append(providers, prov)
			}
		}
	}
	return provider.NewRegistry(providers...)
}

func instantiateCustom(manifest provider.Manifest) provider.Provider {
	if strings.TrimSpace(manifest.Name) == "" {
		fmt.Fprintf(os.Stderr, "pfui: skipping custom provider with empty name\n")
		return nil
	}
	if manifest.Token == "" {
		fmt.Fprintf(os.Stderr, "pfui: skipping %s (missing token). Use pfui provider init --token ... or store a matching API key.\n", manifest.Name)
		return nil
	}
	switch manifest.Adapter {
	case provider.AdapterOpenAIChat, provider.AdapterOpenAIResponses:
		return openai.NewWithName(manifest.Host, manifest.Token, manifest.Name)
	case provider.AdapterAnthropicMessage:
		return anthropic.NewWithName(manifest.Host, manifest.Token, manifest.Name)
	default:
		fmt.Fprintf(os.Stderr, "pfui: adapter %s for %s is not supported yet\n", manifest.Adapter, manifest.Name)
		return nil
	}
}
