package models

// TopTemplateRow is one row of the creator top-templates table, aggregated
// over a range window. Rate (uses/views) is computed in the service layer.
type TopTemplateRow struct {
	ID        string
	Title     string
	Views     int32
	Uses      int32
	Likes     int32
	Favorites int32
}