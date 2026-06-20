package geo

type BBox struct {
	MinLat, MaxLat float64
	MinLon, MaxLon float64
}

func (b BBox) Contains(lat, lon float64) bool {
	return lat >= b.MinLat && lat <= b.MaxLat && lon >= b.MinLon && lon <= b.MaxLon
}

var GreaterBoston = BBox{
	MinLat: 42.0,
	MaxLat: 42.75,
	MinLon: -71.6,
	MaxLon: -70.7,
}

var Center = struct{ Lat, Lon float64 }{Lat: 42.3601, Lon: -71.0589}
