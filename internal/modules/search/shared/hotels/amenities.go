// hotels — amenity code mappings for SerpAPI hotel filters.
package hotels

// AmenityCode maps human-readable amenity names to SerpAPI numeric codes.
var AmenityCode = map[string]int{
	"free_parking":       1,
	"hot_tub":            2,
	"parking":            3,
	"indoor_pool":        4,
	"outdoor_pool":       5,
	"pool":               6,
	"outdoor_grill":      6,
	"fitness_center":     7,
	"restaurant":         8,
	"free_breakfast":     9,
	"spa":                10,
	"fireplace":          10,
	"beach_access":       11,
	"child_friendly":     12,
	"patio":              12,
	"bar":                15,
	"kitchen":            15,
	"fitness_centre":     16,
	"cot":                18,
	"pet_friendly":       19,
	"room_service":       22,
	"free_wifi":          35,
	"air_conditioned":    40,
	"all_inclusive":      52,
	"wheelchair_accessible": 53,
	"ev_charger":         61,
}

// SortByCode maps human-readable sort order strings to SerpAPI numeric codes.
var SortByCode = map[string]int{
	"top_picks":    3,
	"lowest_price": 5,
	"rating":       8,
	"distance":     13,
}

// RatingCode maps rating threshold strings to SerpAPI numeric codes.
var RatingCode = map[string]int{
	"3.0_and_up": 6,
	"3.5_and_up": 7,
	"4.0_and_up": 8,
	"4.5_and_up": 9,
}
