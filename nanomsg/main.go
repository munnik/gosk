package nanomsg

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Used identify header segments
const (
	HEADERSEGMENTPROCESS  = 0
	HEADERSEGMENTPROTOCOL = 1
	HEADERSEGMENTSOURCE   = 2
)

// Used to determine the datatype of the value
const (
	DOUBLE     = 0
	STRING     = 1
	POSITION   = 2
	LENGTH     = 3
	VESSELDATA = 4
)

func ToString(value interface{}) (string, error) {
	switch typedValue := value.(type) {
	case *string:
		return *typedValue, nil
	case *int:
		return fmt.Sprintf("%d", *typedValue), nil
	case *int8:
		return fmt.Sprintf("%d", *typedValue), nil
	case *int16:
		return fmt.Sprintf("%d", *typedValue), nil
	case *int32:
		return fmt.Sprintf("%d", *typedValue), nil
	case *int64:
		return fmt.Sprintf("%d", *typedValue), nil
	case *uint:
		return fmt.Sprintf("%d", *typedValue), nil
	case *uint8:
		return fmt.Sprintf("%d", *typedValue), nil
	case *uint16:
		return fmt.Sprintf("%d", *typedValue), nil
	case *uint32:
		return fmt.Sprintf("%d", *typedValue), nil
	case *uint64:
		return fmt.Sprintf("%d", *typedValue), nil
	case *float32:
		return fmt.Sprintf("%f", *typedValue), nil
	case *float64:
		return fmt.Sprintf("%f", *typedValue), nil
	case *bool:
		return fmt.Sprintf("%t", *typedValue), nil
	case *MappedData_DoubleValue:
		return ToString(&typedValue.DoubleValue)
	case *MappedData_StringValue:
		return ToString(&typedValue.StringValue)
	}

	return "", fmt.Errorf("unsupported type: %T", value)
}

type FloatValue wrapperspb.FloatValue
type DoubleValue wrapperspb.DoubleValue
type StringValue wrapperspb.StringValue
type UInt32Value wrapperspb.UInt32Value
type UInt64Value wrapperspb.UInt64Value
type Int32Value wrapperspb.Int32Value
type Int64Value wrapperspb.Int64Value

type MappedDataCreator interface {
	CreateMappedData(*RawData, string, []string) *MappedData
}

func Float(v float32) *FloatValue {
	result := FloatValue(*wrapperspb.Float(v))
	return &result
}

func Double(v float64) *DoubleValue {
	result := DoubleValue(*wrapperspb.Double(v))
	return &result
}

func String(v string) *StringValue {
	result := StringValue(*wrapperspb.String(v))
	return &result
}

func UInt32(v uint32) *UInt32Value {
	result := UInt32Value(*wrapperspb.UInt32(v))
	return &result
}

func UInt64(v uint64) *UInt64Value {
	result := UInt64Value(*wrapperspb.UInt64(v))
	return &result
}

func Int32(v int32) *Int32Value {
	result := Int32Value(*wrapperspb.Int32(v))
	return &result
}

func Int64(v int64) *Int64Value {
	result := Int64Value(*wrapperspb.Int64(v))
	return &result
}

func createMappedDataWithoutValue(rawData *RawData, context string, path []string) *MappedData {
	return &MappedData{
		Header: &Header{
			HeaderSegments: append([]string{"mapper"}, rawData.Header.HeaderSegments[1:]...),
		},
		Timestamp: rawData.Timestamp,
		Context:   context,
		Path:      strings.Join(path, "."),
	}
}

func (v *FloatValue) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_DoubleValue{DoubleValue: float64(v.Value)}
	return m
}

func (v *DoubleValue) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_DoubleValue{DoubleValue: v.Value}
	return m
}

func (v *StringValue) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_StringValue{StringValue: v.Value}
	return m
}

func (v *UInt32Value) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_DoubleValue{DoubleValue: float64(v.Value)}
	return m
}

func (v *UInt64Value) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_DoubleValue{DoubleValue: float64(v.Value)}
	return m
}

func (v *Int32Value) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_DoubleValue{DoubleValue: float64(v.Value)}
	return m
}

func (v *Int64Value) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_DoubleValue{DoubleValue: float64(v.Value)}
	return m
}

func (v *PositionValue) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_PositionValue{PositionValue: v}
	return m
}

func (v *LengthValue) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_LengthValue{LengthValue: v}
	return m
}

func (v *VesselDataValue) CreateMappedData(rawData *RawData, context string, path []string) *MappedData {
	m := createMappedDataWithoutValue(rawData, context, path)
	m.Value = &MappedData_VesselDataValue{VesselDataValue: v}
	return m
}

func NewMappedDataCreator(in interface{}) (MappedDataCreator, error) {
	var result MappedDataCreator

	switch v := in.(type) {
	case int8:
		result = Int32(int32(v))
	case int16:
		result = Int32(int32(v))
	case int32:
		result = Int32(v)
	case int:
		result = Int64(int64(v))
	case int64:
		result = Int64(v)
	case uint8:
		result = UInt32(uint32(v))
	case uint16:
		result = UInt32(uint32(v))
	case uint32:
		result = UInt32(v)
	case uint:
		result = UInt64(uint64(v))
	case uint64:
		result = UInt64(v)
	case float32:
		result = Float(v)
	case float64:
		result = Double(v)
	case string:
		result = String(v)
	default:
		return nil, fmt.Errorf("type %T is not supported", in)
	}
	return result, nil
}
