package catalog

import (
	"github.com/dector/oir/internal/alias"
	"sort"
)

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
