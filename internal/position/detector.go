package position

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PositionData represents extracted position information
type PositionData struct {
	X          float64   `json:"x"`
	Y          float64   `json:"y"`
	Z          float64   `json:"z"`
	Rotation   float64   `json:"rotation"`
	Map        string    `json:"map"`
	Source     string    `json:"source"`
	Confidence float64   `json:"confidence"`
	Filepath   string    `json:"filepath"`
	Filename   string    `json:"filename"`
	Timestamp  time.Time `json:"timestamp"`
	Processed  bool      `json:"processed"`
}

// Detector handles position extraction from screenshot filenames
type Detector struct {
	// Regex patterns for different screenshot formats
	tarkovMonitorPattern *regexp.Regexp
	standardPattern      *regexp.Regexp
	altPattern          *regexp.Regexp
	mapPattern          *regexp.Regexp
	metadataPattern     *regexp.Regexp
}

// NewDetector creates a new position detector
func NewDetector() *Detector {
	return &Detector{
		// TarkovMonitor format: 2025-07-11[22-31]_25.85, 0.78, -8.95_0.00593, -0.99715, 0.05100, 0.05531_15.47 (0).png
		tarkovMonitorPattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}\[\d{2}-\d{2}\]_(.+) \(\d\)\.png`),
		
		// Standard EFT format: EscapeFromTarkov_2024-01-15_14-30-25_123.45_67.89_180.0.png
		standardPattern: regexp.MustCompile(`EscapeFromTarkov_\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}_(-?\d+\.?\d*)_(-?\d+\.?\d*)_(-?\d+\.?\d*)`),
		
		// Alternative format: screenshot_123.45x67.89r180.png
		altPattern: regexp.MustCompile(`screenshot_(-?\d+\.?\d*)x(-?\d+\.?\d*)r(-?\d+\.?\d*)`),
		
		// Map format: tarkov_customs_2024_123.45_67.89_180.0.png
		mapPattern: regexp.MustCompile(`tarkov_(\w+)_\d{4}_(-?\d+\.?\d*)_(-?\d+\.?\d*)_(-?\d+\.?\d*)`),
		
		// Metadata format: coord[s]_123.45,67.89 or rot[ation]_180.0
		metadataPattern: regexp.MustCompile(`coord[s]?[_-]?(-?\d+\.?\d*)[_,-](-?\d+\.?\d*)`),
	}
}

// AnalyzeScreenshot extracts position data from a screenshot file
func (d *Detector) AnalyzeScreenshot(filepath string) (*PositionData, error) {
	baseName := strings.ToLower(filepath)

	// Fast path: extract position directly from filename
	positionData := d.extractPositionFromFilename(baseName)
	if positionData == nil {
		return nil, fmt.Errorf("no position data found in filename")
	}

	// Fast validation - only check for NaN
	if !d.validatePosition(positionData) {
		return nil, fmt.Errorf("invalid position data extracted")
	}

	// Set metadata
	positionData.Filepath = filepath
	positionData.Filename = filepath
	positionData.Timestamp = time.Now()
	positionData.Processed = true

	return positionData, nil
}

// extractPositionFromFilename extracts position data from filename
func (d *Detector) extractPositionFromFilename(filename string) *PositionData {
	// Try TarkovMonitor format first
	if matches := d.tarkovMonitorPattern.FindStringSubmatch(filename); matches != nil {
		return d.parseTarkovMonitorFormat(matches[1], filename)
	}

	// Try standard EFT format
	if matches := d.standardPattern.FindStringSubmatch(filename); matches != nil {
		x, _ := strconv.ParseFloat(matches[1], 64)
		y, _ := strconv.ParseFloat(matches[2], 64)
		rotation, _ := strconv.ParseFloat(matches[3], 64)

		return &PositionData{
			X:          x,
			Y:          y,
			Z:          0, // Standard format doesn't include Z
			Rotation:   rotation,
			Map:        d.detectMapFromContext(filename),
			Source:     "filename_standard",
			Confidence: 0.9,
		}
	}

	// Try alternative format
	if matches := d.altPattern.FindStringSubmatch(filename); matches != nil {
		x, _ := strconv.ParseFloat(matches[1], 64)
		y, _ := strconv.ParseFloat(matches[2], 64)
		rotation, _ := strconv.ParseFloat(matches[3], 64)

		return &PositionData{
			X:          x,
			Y:          y,
			Z:          0,
			Rotation:   rotation,
			Map:        d.detectMapFromContext(filename),
			Source:     "filename_alt",
			Confidence: 0.8,
		}
	}

	// Try map-specific format
	if matches := d.mapPattern.FindStringSubmatch(filename); matches != nil {
		x, _ := strconv.ParseFloat(matches[2], 64)
		y, _ := strconv.ParseFloat(matches[3], 64)
		rotation, _ := strconv.ParseFloat(matches[4], 64)

		return &PositionData{
			X:          x,
			Y:          y,
			Z:          0,
			Rotation:   rotation,
			Map:        matches[1],
			Source:     "filename_map",
			Confidence: 0.95,
		}
	}

	// Try metadata extraction
	return d.extractFromMetadata(filename)
}

// parseTarkovMonitorFormat parses the TarkovMonitor position string
func (d *Detector) parseTarkovMonitorFormat(positionString, filename string) *PositionData {
	// Parse position: x, y, z_rx, ry, rz, rw
	positionRegex := regexp.MustCompile(`(-?[\d]+\.[\d]{2}), (-?[\d]+\.[\d]{2}), (-?[\d]+\.[\d]{2})_(-?[\d.]{1}\.[\d]{1,5}), (-?[\d.]{1}\.[\d]{1,5}), (-?[\d.]{1}\.[\d]{1,5}), (-?[\d.]{1}\.[\d]{1,5})`)
	
	matches := positionRegex.FindStringSubmatch(positionString)
	if matches == nil {
		return nil
	}

	x, _ := strconv.ParseFloat(matches[1], 64)
	y, _ := strconv.ParseFloat(matches[2], 64)
	z, _ := strconv.ParseFloat(matches[3], 64)
	rx, _ := strconv.ParseFloat(matches[4], 64)
	ry, _ := strconv.ParseFloat(matches[5], 64)
	rz, _ := strconv.ParseFloat(matches[6], 64)
	rw, _ := strconv.ParseFloat(matches[7], 64)

	// Convert quaternion to yaw angle (based on TarkovMonitor implementation)
	// Note: JavaScript calls quaternionToYaw(rx, ry, rz, rw) which maps to function params (x, z, y, w)
	yaw := d.quaternionToYaw(rx, ry, rz, rw)

	return &PositionData{
		X:          x,
		Y:          y,
		Z:          z,
		Rotation:   yaw,
		Map:        d.detectMapFromContext(filename),
		Source:     "tarkovmonitor",
		Confidence: 0.95,
	}
}

// extractFromMetadata tries to extract coordinates from metadata patterns
func (d *Detector) extractFromMetadata(filename string) *PositionData {
	coordMatches := d.metadataPattern.FindStringSubmatch(filename)
	if coordMatches == nil {
		return nil
	}

	x, _ := strconv.ParseFloat(coordMatches[1], 64)
	y, _ := strconv.ParseFloat(coordMatches[2], 64)

	// Look for rotation
	rotationRegex := regexp.MustCompile(`rot[ation]?[_-]?(-?\d+\.?\d*)`)
	rotationMatches := rotationRegex.FindStringSubmatch(filename)
	
	rotation := 0.0
	if rotationMatches != nil {
		rotation, _ = strconv.ParseFloat(rotationMatches[1], 64)
	}

	return &PositionData{
		X:          x,
		Y:          y,
		Z:          0,
		Rotation:   rotation,
		Map:        d.detectMapFromContext(filename),
		Source:     "metadata",
		Confidence: 0.7,
	}
}

// detectMapFromContext tries to detect map name from filename
func (d *Detector) detectMapFromContext(filename string) string {
	lowerFilename := strings.ToLower(filename)

	// Check for map names in filename
	maps := []string{"factory", "customs", "woods", "shoreline", "interchange", "reserve", "lighthouse", "streets", "labs", "ground-zero"}
	for _, mapName := range maps {
		if strings.Contains(lowerFilename, mapName) {
			return mapName
		}
	}

	// Check for map aliases
	aliases := map[string][]string{
		"factory":     {"factory", "zavod"},
		"customs":     {"customs", "tamozhennya"},
		"woods":       {"woods", "les"},
		"shoreline":   {"shoreline", "berezhok"},
		"interchange": {"interchange", "razvyazka"},
		"reserve":     {"reserve", "rezerv"},
		"lighthouse":  {"lighthouse", "mayak"},
		"streets":     {"streets", "ulitsy"},
		"labs":        {"labs", "laboratoriya"},
		"ground-zero": {"ground-zero", "groundzero", "gz"},
	}

	for mapName, aliasSlice := range aliases {
		for _, alias := range aliasSlice {
			if strings.Contains(lowerFilename, alias) {
				return mapName
			}
		}
	}

	// Return empty string if no map detected - will be filled in by GameWatcher
	return ""
}

// quaternionToYaw converts quaternion to yaw angle (based on TarkovMonitor implementation)
func (d *Detector) quaternionToYaw(x, z, y, w float64) float64 {
	// Convert quaternion to yaw angle
	sinyCosp := 2.0 * (w*z + x*y)
	cosyCosp := 1.0 - 2.0*(y*y+z*z)
	yaw := math.Atan2(sinyCosp, cosyCosp)

	// Convert radians to degrees
	yaw *= (180.0 / math.Pi)

	// Normalize to 0-360 range
	for yaw < 0 {
		yaw += 360
	}
	for yaw >= 360 {
		yaw -= 360
	}

	return yaw
}

// validatePosition performs fast validation on position data
func (d *Detector) validatePosition(position *PositionData) bool {
	if position == nil {
		return false
	}

	// Fast validation - only check for NaN, skip bounds checking for speed
	return !math.IsNaN(position.X) && !math.IsNaN(position.Y) && !math.IsNaN(position.Rotation)
}

// IsEFTScreenshot checks if a filename appears to be an EFT screenshot
func IsEFTScreenshot(filename string) bool {
	lowerFilename := strings.ToLower(filename)

	// Check for TarkovMonitor screenshot pattern
	tarkovMonitorPattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2}\[\d{2}-\d{2}\]_.*\(\d\)\.png$`)
	if tarkovMonitorPattern.MatchString(filename) {
		return true
	}

	// Check for other EFT screenshot patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)escapefromtarkov`),
		regexp.MustCompile(`(?i)tarkov`),
		regexp.MustCompile(`(?i)eft`),
		regexp.MustCompile(`(?i)screenshot.*\d{4}-\d{2}-\d{2}`), // Date pattern
		regexp.MustCompile(`(?i)\d+\.\d+_\d+\.\d+_\d+\.\d+`),   // Coordinate pattern
	}

	for _, pattern := range patterns {
		if pattern.MatchString(lowerFilename) {
			return true
		}
	}

	return false
}