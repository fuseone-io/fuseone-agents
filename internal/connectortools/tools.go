package connectortools

import (
	"context"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

type ToolSource interface {
	Tools(ctx context.Context) ([]domain.ToolEntry, error)
}

type ToolList struct {
	base     ToolSource
	settings *Settings
}

func NewToolList(base ToolSource, settings *Settings) *ToolList {
	return &ToolList{base: base, settings: settings}
}

func (l *ToolList) Tools(ctx context.Context) ([]domain.ToolEntry, error) {
	var base []domain.ToolEntry
	if l != nil && l.base != nil {
		listed, err := l.base.Tools(ctx)
		if err != nil {
			return nil, err
		}
		base = listed
	}
	if l != nil && l.settings != nil {
		native, err := l.settings.ToolEntries(ctx)
		if err != nil {
			return nil, err
		}
		return mergeToolEntries(base, native), nil
	}
	return sortedToolEntries(base), nil
}

func mergeToolEntries(base, native []domain.ToolEntry) []domain.ToolEntry {
	byID := map[domain.ToolID]domain.ToolEntry{}
	for _, entry := range base {
		byID[entry.ID] = entry
	}
	for _, entry := range native {
		byID[entry.ID] = entry
	}
	return sortedToolEntriesFromMap(byID)
}

func sortedToolEntries(in []domain.ToolEntry) []domain.ToolEntry {
	byID := map[domain.ToolID]domain.ToolEntry{}
	for _, entry := range in {
		byID[entry.ID] = entry
	}
	return sortedToolEntriesFromMap(byID)
}

func sortedToolEntriesFromMap(byID map[domain.ToolID]domain.ToolEntry) []domain.ToolEntry {
	out := make([]domain.ToolEntry, 0, len(byID))
	for _, entry := range byID {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out
}
