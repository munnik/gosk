// build using `protoc --go_out=paths=source_relative:. .\nanomsg\messages.proto`

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.12.4
// source: nanomsg/messages.proto

package nanomsg

import (
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Header struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	HeaderSegments []string `protobuf:"bytes,1,rep,name=headerSegments,proto3" json:"headerSegments,omitempty"`
}

func (x *Header) Reset() {
	*x = Header{}
	if protoimpl.UnsafeEnabled {
		mi := &file_nanomsg_messages_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Header) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Header) ProtoMessage() {}

func (x *Header) ProtoReflect() protoreflect.Message {
	mi := &file_nanomsg_messages_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Header.ProtoReflect.Descriptor instead.
func (*Header) Descriptor() ([]byte, []int) {
	return file_nanomsg_messages_proto_rawDescGZIP(), []int{0}
}

func (x *Header) GetHeaderSegments() []string {
	if x != nil {
		return x.HeaderSegments
	}
	return nil
}

type RawData struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Header    *Header              `protobuf:"bytes,1,opt,name=header,proto3" json:"header,omitempty"`
	Timestamp *timestamp.Timestamp `protobuf:"bytes,2,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Payload   []byte               `protobuf:"bytes,3,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *RawData) Reset() {
	*x = RawData{}
	if protoimpl.UnsafeEnabled {
		mi := &file_nanomsg_messages_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RawData) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RawData) ProtoMessage() {}

func (x *RawData) ProtoReflect() protoreflect.Message {
	mi := &file_nanomsg_messages_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RawData.ProtoReflect.Descriptor instead.
func (*RawData) Descriptor() ([]byte, []int) {
	return file_nanomsg_messages_proto_rawDescGZIP(), []int{1}
}

func (x *RawData) GetHeader() *Header {
	if x != nil {
		return x.Header
	}
	return nil
}

func (x *RawData) GetTimestamp() *timestamp.Timestamp {
	if x != nil {
		return x.Timestamp
	}
	return nil
}

func (x *RawData) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

type MappedData struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Header        *Header              `protobuf:"bytes,1,opt,name=header,proto3" json:"header,omitempty"`
	Context       string               `protobuf:"bytes,2,opt,name=context,proto3" json:"context,omitempty"`
	Path          string               `protobuf:"bytes,3,opt,name=path,proto3" json:"path,omitempty"`
	Timestamp     *timestamp.Timestamp `protobuf:"bytes,4,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Datatype      int32                `protobuf:"varint,5,opt,name=datatype,proto3" json:"datatype,omitempty"`
	DoubleValue   float64              `protobuf:"fixed64,6,opt,name=doubleValue,proto3" json:"doubleValue,omitempty"`
	StringValue   string               `protobuf:"bytes,7,opt,name=stringValue,proto3" json:"stringValue,omitempty"`
	PositionValue *Position            `protobuf:"bytes,8,opt,name=positionValue,proto3" json:"positionValue,omitempty"`
	LengthValue   *Length              `protobuf:"bytes,9,opt,name=lengthValue,proto3" json:"lengthValue,omitempty"`
}

func (x *MappedData) Reset() {
	*x = MappedData{}
	if protoimpl.UnsafeEnabled {
		mi := &file_nanomsg_messages_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MappedData) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MappedData) ProtoMessage() {}

func (x *MappedData) ProtoReflect() protoreflect.Message {
	mi := &file_nanomsg_messages_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MappedData.ProtoReflect.Descriptor instead.
func (*MappedData) Descriptor() ([]byte, []int) {
	return file_nanomsg_messages_proto_rawDescGZIP(), []int{2}
}

func (x *MappedData) GetHeader() *Header {
	if x != nil {
		return x.Header
	}
	return nil
}

func (x *MappedData) GetContext() string {
	if x != nil {
		return x.Context
	}
	return ""
}

func (x *MappedData) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *MappedData) GetTimestamp() *timestamp.Timestamp {
	if x != nil {
		return x.Timestamp
	}
	return nil
}

func (x *MappedData) GetDatatype() int32 {
	if x != nil {
		return x.Datatype
	}
	return 0
}

func (x *MappedData) GetDoubleValue() float64 {
	if x != nil {
		return x.DoubleValue
	}
	return 0
}

func (x *MappedData) GetStringValue() string {
	if x != nil {
		return x.StringValue
	}
	return ""
}

func (x *MappedData) GetPositionValue() *Position {
	if x != nil {
		return x.PositionValue
	}
	return nil
}

func (x *MappedData) GetLengthValue() *Length {
	if x != nil {
		return x.LengthValue
	}
	return nil
}

type Position struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Latitude  float64 `protobuf:"fixed64,1,opt,name=latitude,proto3" json:"latitude,omitempty"`
	Longitude float64 `protobuf:"fixed64,2,opt,name=longitude,proto3" json:"longitude,omitempty"`
	Altitude  float64 `protobuf:"fixed64,3,opt,name=altitude,proto3" json:"altitude,omitempty"`
}

func (x *Position) Reset() {
	*x = Position{}
	if protoimpl.UnsafeEnabled {
		mi := &file_nanomsg_messages_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Position) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Position) ProtoMessage() {}

func (x *Position) ProtoReflect() protoreflect.Message {
	mi := &file_nanomsg_messages_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Position.ProtoReflect.Descriptor instead.
func (*Position) Descriptor() ([]byte, []int) {
	return file_nanomsg_messages_proto_rawDescGZIP(), []int{3}
}

func (x *Position) GetLatitude() float64 {
	if x != nil {
		return x.Latitude
	}
	return 0
}

func (x *Position) GetLongitude() float64 {
	if x != nil {
		return x.Longitude
	}
	return 0
}

func (x *Position) GetAltitude() float64 {
	if x != nil {
		return x.Altitude
	}
	return 0
}

type Length struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Overall   float64 `protobuf:"fixed64,1,opt,name=overall,proto3" json:"overall,omitempty"`
	Hull      float64 `protobuf:"fixed64,2,opt,name=hull,proto3" json:"hull,omitempty"`
	Waterline float64 `protobuf:"fixed64,3,opt,name=waterline,proto3" json:"waterline,omitempty"`
}

func (x *Length) Reset() {
	*x = Length{}
	if protoimpl.UnsafeEnabled {
		mi := &file_nanomsg_messages_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Length) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Length) ProtoMessage() {}

func (x *Length) ProtoReflect() protoreflect.Message {
	mi := &file_nanomsg_messages_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Length.ProtoReflect.Descriptor instead.
func (*Length) Descriptor() ([]byte, []int) {
	return file_nanomsg_messages_proto_rawDescGZIP(), []int{4}
}

func (x *Length) GetOverall() float64 {
	if x != nil {
		return x.Overall
	}
	return 0
}

func (x *Length) GetHull() float64 {
	if x != nil {
		return x.Hull
	}
	return 0
}

func (x *Length) GetWaterline() float64 {
	if x != nil {
		return x.Waterline
	}
	return 0
}

var File_nanomsg_messages_proto protoreflect.FileDescriptor

var file_nanomsg_messages_proto_rawDesc = []byte{
	0x0a, 0x16, 0x6e, 0x61, 0x6e, 0x6f, 0x6d, 0x73, 0x67, 0x2f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x30, 0x0a, 0x06, 0x48, 0x65, 0x61,
	0x64, 0x65, 0x72, 0x12, 0x26, 0x0a, 0x0e, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x53, 0x65, 0x67,
	0x6d, 0x65, 0x6e, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0e, 0x68, 0x65, 0x61,
	0x64, 0x65, 0x72, 0x53, 0x65, 0x67, 0x6d, 0x65, 0x6e, 0x74, 0x73, 0x22, 0x7e, 0x0a, 0x07, 0x52,
	0x61, 0x77, 0x44, 0x61, 0x74, 0x61, 0x12, 0x1f, 0x0a, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x07, 0x2e, 0x48, 0x65, 0x61, 0x64, 0x65, 0x72, 0x52,
	0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x12, 0x38, 0x0a, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73,
	0x74, 0x61, 0x6d, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x22, 0xd1, 0x02, 0x0a, 0x0a,
	0x4d, 0x61, 0x70, 0x70, 0x65, 0x64, 0x44, 0x61, 0x74, 0x61, 0x12, 0x1f, 0x0a, 0x06, 0x68, 0x65,
	0x61, 0x64, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x07, 0x2e, 0x48, 0x65, 0x61,
	0x64, 0x65, 0x72, 0x52, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x12, 0x18, 0x0a, 0x07, 0x63,
	0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x63, 0x6f,
	0x6e, 0x74, 0x65, 0x78, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68, 0x12, 0x38, 0x0a, 0x09, 0x74, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54,
	0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x12, 0x1a, 0x0a, 0x08, 0x64, 0x61, 0x74, 0x61, 0x74, 0x79, 0x70, 0x65, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x08, 0x64, 0x61, 0x74, 0x61, 0x74, 0x79, 0x70, 0x65, 0x12,
	0x20, 0x0a, 0x0b, 0x64, 0x6f, 0x75, 0x62, 0x6c, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x06,
	0x20, 0x01, 0x28, 0x01, 0x52, 0x0b, 0x64, 0x6f, 0x75, 0x62, 0x6c, 0x65, 0x56, 0x61, 0x6c, 0x75,
	0x65, 0x12, 0x20, 0x0a, 0x0b, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x56, 0x61, 0x6c, 0x75, 0x65,
	0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x56, 0x61,
	0x6c, 0x75, 0x65, 0x12, 0x2f, 0x0a, 0x0d, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x56,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x50, 0x6f, 0x73,
	0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0d, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x56,
	0x61, 0x6c, 0x75, 0x65, 0x12, 0x29, 0x0a, 0x0b, 0x6c, 0x65, 0x6e, 0x67, 0x74, 0x68, 0x56, 0x61,
	0x6c, 0x75, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x07, 0x2e, 0x4c, 0x65, 0x6e, 0x67,
	0x74, 0x68, 0x52, 0x0b, 0x6c, 0x65, 0x6e, 0x67, 0x74, 0x68, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x22,
	0x60, 0x0a, 0x08, 0x50, 0x6f, 0x73, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1a, 0x0a, 0x08, 0x6c,
	0x61, 0x74, 0x69, 0x74, 0x75, 0x64, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x01, 0x52, 0x08, 0x6c,
	0x61, 0x74, 0x69, 0x74, 0x75, 0x64, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x6c, 0x6f, 0x6e, 0x67, 0x69,
	0x74, 0x75, 0x64, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x01, 0x52, 0x09, 0x6c, 0x6f, 0x6e, 0x67,
	0x69, 0x74, 0x75, 0x64, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x61, 0x6c, 0x74, 0x69, 0x74, 0x75, 0x64,
	0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x01, 0x52, 0x08, 0x61, 0x6c, 0x74, 0x69, 0x74, 0x75, 0x64,
	0x65, 0x22, 0x54, 0x0a, 0x06, 0x4c, 0x65, 0x6e, 0x67, 0x74, 0x68, 0x12, 0x18, 0x0a, 0x07, 0x6f,
	0x76, 0x65, 0x72, 0x61, 0x6c, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x01, 0x52, 0x07, 0x6f, 0x76,
	0x65, 0x72, 0x61, 0x6c, 0x6c, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x75, 0x6c, 0x6c, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x01, 0x52, 0x04, 0x68, 0x75, 0x6c, 0x6c, 0x12, 0x1c, 0x0a, 0x09, 0x77, 0x61, 0x74,
	0x65, 0x72, 0x6c, 0x69, 0x6e, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x01, 0x52, 0x09, 0x77, 0x61,
	0x74, 0x65, 0x72, 0x6c, 0x69, 0x6e, 0x65, 0x42, 0x20, 0x5a, 0x1e, 0x67, 0x69, 0x74, 0x68, 0x75,
	0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6d, 0x75, 0x6e, 0x6e, 0x69, 0x6b, 0x2f, 0x67, 0x6f, 0x73,
	0x6b, 0x2f, 0x6e, 0x61, 0x6e, 0x6f, 0x6d, 0x73, 0x67, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_nanomsg_messages_proto_rawDescOnce sync.Once
	file_nanomsg_messages_proto_rawDescData = file_nanomsg_messages_proto_rawDesc
)

func file_nanomsg_messages_proto_rawDescGZIP() []byte {
	file_nanomsg_messages_proto_rawDescOnce.Do(func() {
		file_nanomsg_messages_proto_rawDescData = protoimpl.X.CompressGZIP(file_nanomsg_messages_proto_rawDescData)
	})
	return file_nanomsg_messages_proto_rawDescData
}

var file_nanomsg_messages_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_nanomsg_messages_proto_goTypes = []interface{}{
	(*Header)(nil),              // 0: Header
	(*RawData)(nil),             // 1: RawData
	(*MappedData)(nil),          // 2: MappedData
	(*Position)(nil),            // 3: Position
	(*Length)(nil),              // 4: Length
	(*timestamp.Timestamp)(nil), // 5: google.protobuf.Timestamp
}
var file_nanomsg_messages_proto_depIdxs = []int32{
	0, // 0: RawData.header:type_name -> Header
	5, // 1: RawData.timestamp:type_name -> google.protobuf.Timestamp
	0, // 2: MappedData.header:type_name -> Header
	5, // 3: MappedData.timestamp:type_name -> google.protobuf.Timestamp
	3, // 4: MappedData.positionValue:type_name -> Position
	4, // 5: MappedData.lengthValue:type_name -> Length
	6, // [6:6] is the sub-list for method output_type
	6, // [6:6] is the sub-list for method input_type
	6, // [6:6] is the sub-list for extension type_name
	6, // [6:6] is the sub-list for extension extendee
	0, // [0:6] is the sub-list for field type_name
}

func init() { file_nanomsg_messages_proto_init() }
func file_nanomsg_messages_proto_init() {
	if File_nanomsg_messages_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_nanomsg_messages_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Header); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_nanomsg_messages_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RawData); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_nanomsg_messages_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MappedData); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_nanomsg_messages_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Position); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_nanomsg_messages_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Length); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_nanomsg_messages_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_nanomsg_messages_proto_goTypes,
		DependencyIndexes: file_nanomsg_messages_proto_depIdxs,
		MessageInfos:      file_nanomsg_messages_proto_msgTypes,
	}.Build()
	File_nanomsg_messages_proto = out.File
	file_nanomsg_messages_proto_rawDesc = nil
	file_nanomsg_messages_proto_goTypes = nil
	file_nanomsg_messages_proto_depIdxs = nil
}