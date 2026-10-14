package catalog

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dector/oir/internal/alias"
	"github.com/dector/oir/internal/spec"
)

// Resolution keeps provenance separate from the installer input.
type Resolution struct {
	Spec   spec.Spec
	Source string
}

// Explicit reports arguments that bypass alias lookup.
func Explicit(raw string) bool { return strings.ContainsAny(strings.TrimSpace(raw), "/:") }

// Resolve checks user aliases before loading registries. Fresh misses never
// cause another download; Entries alone owns the cache policy.
func (c *Catalog) Resolve(ctx context.Context, raw string, users []alias.Entry) (Resolution, error) {
	raw = strings.TrimSpace(raw)
	if Explicit(raw) {
		sp, err := spec.Parse(raw)
		return Resolution{Spec: sp}, err
	}
	name, version, _ := strings.Cut(raw, "@")
	var winner Entry
	for _, e := range users {
		if e.Key == name {
			winner = Entry{e.Key, e.Spec, "user"}
			break
		}
	}
	if winner.Key == "" {
		entries, err := c.Entries(ctx)
		if err != nil {
			return Resolution{}, err
		}
		for _, e := range entries {
			if e.Key == name {
				winner = e
				break
			}
		}
	}
	if winner.Key == "" {
		return Resolution{}, fmt.Errorf("no tool registered with name %q; try oir refresh", name)
	}
	sp, err := spec.Parse(winner.Spec)
	if err != nil {
		return Resolution{}, fmt.Errorf("alias %q: %w", name, err)
	}
	if version != "" {
		sp.Version = version
	}
	return Resolution{Spec: sp, Source: winner.Source}, nil
}

// Merge deduplicates names using user precedence, then source registration order.
func Merge(users []alias.Entry, registries []Entry) []Entry {
	winners := make(map[string]Entry)
	for _, e := range users {
		winners[e.Key] = Entry{e.Key, e.Spec, "user"}
	}
	for _, e := range registries {
		if _, ok := winners[e.Key]; !ok {
			winners[e.Key] = e
		}
	}
	keys := make([]string, 0, len(winners))
	for k := range winners {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]Entry, 0, len(keys))
	for _, k := range keys {
		result = append(result, winners[k])
	}
	return result
}
