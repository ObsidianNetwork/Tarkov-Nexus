package game

import (
	"sort"
	"strings"
)

// MapData represents a map with its various identifiers
type MapData struct {
	NameID         string // Internal game map name
	NormalizedName string // tarkov.dev API name
	DisplayName    string // Human-readable name
}

// MapResolver converts internal game map names to tarkov.dev normalized names
type MapResolver struct {
	maps []MapData
}

// NewMapResolver creates a new map resolver
func NewMapResolver() *MapResolver {
	return &MapResolver{
		maps: []MapData{
			{NameID: "factory", NormalizedName: "factory", DisplayName: "Factory"},
			{NameID: "factory_day", NormalizedName: "factory", DisplayName: "Factory Day"},
			{NameID: "factory_night", NormalizedName: "factory", DisplayName: "Factory Night"},
			{NameID: "factory4_day", NormalizedName: "factory", DisplayName: "Factory 4 Day"},
			{NameID: "factory4_night", NormalizedName: "factory", DisplayName: "Factory 4 Night"},
			{NameID: "bigmap", NormalizedName: "customs", DisplayName: "Customs"},
			{NameID: "customs", NormalizedName: "customs", DisplayName: "Customs"},
			{NameID: "woods", NormalizedName: "woods", DisplayName: "Woods"},
			{NameID: "shoreline", NormalizedName: "shoreline", DisplayName: "Shoreline"},
			{NameID: "interchange", NormalizedName: "interchange", DisplayName: "Interchange"},
			{NameID: "rezervbase", NormalizedName: "reserve", DisplayName: "Reserve"},
			{NameID: "reserve", NormalizedName: "reserve", DisplayName: "Reserve"},
			{NameID: "lighthouse", NormalizedName: "lighthouse", DisplayName: "Lighthouse"},
			{NameID: "tarkovstreets", NormalizedName: "streets-of-tarkov", DisplayName: "Streets of Tarkov"},
			{NameID: "streets", NormalizedName: "streets-of-tarkov", DisplayName: "Streets of Tarkov"},
			{NameID: "laboratory", NormalizedName: "the-lab", DisplayName: "The Lab"},
			{NameID: "the-lab", NormalizedName: "the-lab", DisplayName: "The Lab"},
			{NameID: "labs", NormalizedName: "the-lab", DisplayName: "The Lab"},
			{NameID: "groundzero", NormalizedName: "ground-zero", DisplayName: "Ground Zero"},
			{NameID: "ground-zero", NormalizedName: "ground-zero", DisplayName: "Ground Zero"},
			{NameID: "sandbox", NormalizedName: "ground-zero", DisplayName: "Ground Zero"},
			{NameID: "sandbox_high", NormalizedName: "ground-zero", DisplayName: "Ground Zero"},
		},
	}
}

// FindByNameID finds a map by its internal game name ID, or by its tarkov.dev
// normalized name.
//
// Both are accepted because callers legitimately hold either: game logs and
// screenshot filenames yield name IDs ("bigmap", "tarkovstreets"), while the
// dashboard selector and anything that has already been through this resolver
// hold normalized names ("customs", "streets-of-tarkov"). Matching only name
// IDs meant a canonical name like "streets-of-tarkov" resolved to nothing,
// which showed as a blank entry in the map selector and made the manual
// override unusable for that map.
func (r *MapResolver) FindByNameID(nameID string) *MapData {
	if nameID == "" {
		return nil
	}

	// Case-insensitive matching
	search := strings.ToLower(nameID)

	for _, m := range r.maps {
		if strings.ToLower(m.NameID) == search {
			return &m
		}
	}

	// Fall back to the normalized name.
	for _, m := range r.maps {
		if strings.ToLower(m.NormalizedName) == search {
			return &m
		}
	}

	return nil
}

// GetNormalizedName returns the tarkov.dev normalized map name
func (r *MapResolver) GetNormalizedName(nameID string) string {
	m := r.FindByNameID(nameID)
	if m != nil {
		return m.NormalizedName
	}
	return ""
}

// GetDisplayName returns the human-readable map name
func (r *MapResolver) GetDisplayName(nameID string) string {
	m := r.FindByNameID(nameID)
	if m != nil {
		return m.DisplayName
	}
	return ""
}

// IsValidMap checks if a map name ID is valid
func (r *MapResolver) IsValidMap(nameID string) bool {
	return r.FindByNameID(nameID) != nil
}

// SelectableMap is one entry for the dashboard's manual map selector.
type SelectableMap struct {
	NormalizedName string
	DisplayName    string
}

// GetSelectableMaps returns the distinct maps a user can pick manually, keyed by
// the normalized name tarkov.dev accepts and labelled for display.
//
// It is derived from the resolver's own table rather than written out again in
// the UI layer. The previous hardcoded list drifted: it mixed name IDs
// ("streets", "sandbox") with normalized names, listed "factory-night" — which
// resolves to no map at all and rendered with a blank label — and offered Ground
// Zero twice. Deriving it means the selector cannot offer a map the rest of the
// pipeline will reject.
func (r *MapResolver) GetSelectableMaps() []SelectableMap {
	seen := make(map[string]bool, len(r.maps))
	out := make([]SelectableMap, 0, len(r.maps))

	for _, m := range r.maps {
		if m.NormalizedName == "" || seen[m.NormalizedName] {
			continue
		}
		seen[m.NormalizedName] = true
		out = append(out, SelectableMap{
			NormalizedName: m.NormalizedName,
			DisplayName:    m.DisplayName,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out
}

// GetAllMaps returns all available map data
func (r *MapResolver) GetAllMaps() []MapData {
	result := make([]MapData, len(r.maps))
	copy(result, r.maps)
	return result
}
