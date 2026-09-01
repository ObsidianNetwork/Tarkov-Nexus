package maps

import "strings"

// MapData represents map information
type MapData struct {
	NameID         string `json:"nameId"`
	NormalizedName string `json:"normalizedName"`
	Name           string `json:"name"`
}

// Resolver converts internal game map names to tarkov.dev normalized names
type Resolver struct {
	mapData []MapData
}

// NewResolver creates a new map resolver
func NewResolver() *Resolver {
	return &Resolver{
		mapData: []MapData{
			{NameID: "factory", NormalizedName: "factory", Name: "Factory"},
			{NameID: "factory_day", NormalizedName: "factory", Name: "Factory Day"},
			{NameID: "factory_night", NormalizedName: "factory", Name: "Factory Night"},
			{NameID: "factory4_day", NormalizedName: "factory", Name: "Factory 4 Day"},
			{NameID: "factory4_night", NormalizedName: "factory", Name: "Factory 4 Night"},
			{NameID: "bigmap", NormalizedName: "customs", Name: "Customs"},
			{NameID: "customs", NormalizedName: "customs", Name: "Customs"},
			{NameID: "woods", NormalizedName: "woods", Name: "Woods"},
			{NameID: "shoreline", NormalizedName: "shoreline", Name: "Shoreline"},
			{NameID: "interchange", NormalizedName: "interchange", Name: "Interchange"},
			{NameID: "rezervbase", NormalizedName: "reserve", Name: "Reserve"},
			{NameID: "reserve", NormalizedName: "reserve", Name: "Reserve"},
			{NameID: "lighthouse", NormalizedName: "lighthouse", Name: "Lighthouse"},
			{NameID: "tarkovstreets", NormalizedName: "streets-of-tarkov", Name: "Streets of Tarkov"},
			{NameID: "streets", NormalizedName: "streets-of-tarkov", Name: "Streets of Tarkov"},
			{NameID: "laboratory", NormalizedName: "the-lab", Name: "The Lab"},
			{NameID: "labs", NormalizedName: "the-lab", Name: "The Lab"},
			{NameID: "groundzero", NormalizedName: "ground-zero", Name: "Ground Zero"},
			{NameID: "ground-zero", NormalizedName: "ground-zero", Name: "Ground Zero"},
			{NameID: "sandbox", NormalizedName: "ground-zero", Name: "Ground Zero"},
			{NameID: "sandbox_high", NormalizedName: "ground-zero", Name: "Ground Zero"},
		},
	}
}

// FindByNameID finds map by internal game name ID
func (r *Resolver) FindByNameID(nameID string) *MapData {
	if nameID == "" {
		return nil
	}

	// Convert to lowercase for case-insensitive matching
	searchNameID := strings.ToLower(nameID)

	for _, mapData := range r.mapData {
		if strings.ToLower(mapData.NameID) == searchNameID {
			return &mapData
		}
	}

	return nil
}

// GetNormalizedName gets normalized map name for tarkov.dev API
func (r *Resolver) GetNormalizedName(nameID string) string {
	mapData := r.FindByNameID(nameID)
	if mapData != nil {
		return mapData.NormalizedName
	}
	return ""
}

// GetDisplayName gets display name for map
func (r *Resolver) GetDisplayName(nameID string) string {
	mapData := r.FindByNameID(nameID)
	if mapData != nil {
		return mapData.Name
	}
	return ""
}

// IsValidMap checks if a map name ID is valid
func (r *Resolver) IsValidMap(nameID string) bool {
	return r.FindByNameID(nameID) != nil
}

// GetAllMaps returns all available maps
func (r *Resolver) GetAllMaps() []MapData {
	result := make([]MapData, len(r.mapData))
	copy(result, r.mapData)
	return result
}