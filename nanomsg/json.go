package nanomsg

import "encoding/json"

func (v MappedData_DoubleValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.DoubleValue)
}

func (v MappedData_StringValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.StringValue)
}

func (v MappedData_PositionValue) MarshalJSON() ([]byte, error) {
	var position struct {
		Altitude  float64 `json:"altitude,omitempty"`
		Latitude  float64 `json:"latitude,omitempty"`
		Longitude float64 `json:"longitude,omitempty"`
	}
	if v.PositionValue.Altitude != nil {
		position.Altitude = v.PositionValue.GetAltitude()
	}
	if v.PositionValue.Latitude != nil {
		position.Latitude = v.PositionValue.GetLatitude()
	}
	if v.PositionValue.Longitude != nil {
		position.Longitude = v.PositionValue.GetLongitude()
	}
	return json.Marshal(position)
}

func (v MappedData_LengthValue) MarshalJSON() ([]byte, error) {
	var length struct {
		Hull      float64 `json:"hull,omitempty"`
		Overall   float64 `json:"overall,omitempty"`
		Waterline float64 `json:"waterline,omitempty"`
	}
	if v.LengthValue.Hull != nil {
		length.Hull = v.LengthValue.GetHull()
	}
	if v.LengthValue.Overall != nil {
		length.Overall = v.LengthValue.GetOverall()
	}
	if v.LengthValue.Waterline != nil {
		length.Waterline = v.LengthValue.GetWaterline()
	}
	return json.Marshal(length)
}

func (v MappedData_VesselDataValue) MarshalJSON() ([]byte, error) {
	var vesselData struct {
		Mmsi string `json:"mmsi,omitempty"`
		Name string `json:"name,omitempty"`
	}
	if v.VesselDataValue.Mmsi != nil {
		vesselData.Mmsi = v.VesselDataValue.GetMmsi()
	}
	if v.VesselDataValue.Name != nil {
		vesselData.Name = v.VesselDataValue.GetName()
	}
	return json.Marshal(vesselData)
}
