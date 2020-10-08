package nmea

// FixQuality retrieves the fix quality from the sentence
type FixQuality interface {
	GetFixQuality() (string, uint32, error)
}

// FixType retrieves the fix type from the sentence
type FixType interface {
	GetFixType() (string, uint32, error)
}

// NumberOfSatelites retrieves the number of satelites from the sentence
type NumberOfSatelites interface {
	GetNumberOfSatelites() (int64, uint32, error)
}

// GetFixQuality retrieves the fix quality from the sentence
func (s GGA) GetFixQuality() (string, uint32, error) {
	return s.FixQuality, 0, nil
}

// GetFixType retrieves the fix type from the sentence
func (s GSA) GetFixType() (string, uint32, error) {
	return s.FixType, 0, nil
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GGA) GetNumberOfSatelites() (int64, uint32, error) {
	return s.NumSatellites, 0, nil
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GSA) GetNumberOfSatelites() (int64, uint32, error) {
	return int64(len(s.SV)), 0, nil
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GSV) GetNumberOfSatelites() (int64, uint32, error) {
	return s.NumberSVsInView, 0, nil
}
