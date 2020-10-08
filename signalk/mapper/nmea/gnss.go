package nmea

// FixQuality retrieves the fix quality from the sentence
type FixQuality interface {
	GetFixQuality() (string, error)
}

// FixType retrieves the fix type from the sentence
type FixType interface {
	GetFixType() (string, error)
}

// NumberOfSatelites retrieves the number of satelites from the sentence
type NumberOfSatelites interface {
	GetNumberOfSatelites() (int64, error)
}

// GetFixQuality retrieves the fix quality from the sentence
func (s GGA) GetFixQuality() (string, error) {
	return s.FixQuality, nil
}

// GetFixType retrieves the fix type from the sentence
func (s GSA) GetFixType() (string, error) {
	return s.FixType, nil
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GGA) GetNumberOfSatelites() (int64, error) {
	return s.NumSatellites, nil
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GSA) GetNumberOfSatelites() (int64, error) {
	return int64(len(s.SV)), nil
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GSV) GetNumberOfSatelites() (int64, error) {
	return s.NumberSVsInView, nil
}
