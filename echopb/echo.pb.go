// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v6.31.0
// source: proto/echo.proto

// TODO: use edition 2023 when supported by prost-build
// Also remove command line flags in Makefile
// edition = "2023";

package echopb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type EchoRequest struct {
	state            protoimpl.MessageState `protogen:"opaque.v1"`
	xxx_hidden_Input string                 `protobuf:"bytes,1,opt,name=input,proto3"`
	unknownFields    protoimpl.UnknownFields
	sizeCache        protoimpl.SizeCache
}

func (x *EchoRequest) Reset() {
	*x = EchoRequest{}
	mi := &file_proto_echo_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EchoRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EchoRequest) ProtoMessage() {}

func (x *EchoRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_echo_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *EchoRequest) GetInput() string {
	if x != nil {
		return x.xxx_hidden_Input
	}
	return ""
}

func (x *EchoRequest) SetInput(v string) {
	x.xxx_hidden_Input = v
}

type EchoRequest_builder struct {
	_ [0]func() // Prevents comparability and use of unkeyed literals for the builder.

	Input string
}

func (b0 EchoRequest_builder) Build() *EchoRequest {
	m0 := &EchoRequest{}
	b, x := &b0, m0
	_, _ = b, x
	x.xxx_hidden_Input = b.Input
	return m0
}

type EchoResponse struct {
	state             protoimpl.MessageState `protogen:"opaque.v1"`
	xxx_hidden_Output string                 `protobuf:"bytes,1,opt,name=output,proto3"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *EchoResponse) Reset() {
	*x = EchoResponse{}
	mi := &file_proto_echo_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EchoResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EchoResponse) ProtoMessage() {}

func (x *EchoResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_echo_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *EchoResponse) GetOutput() string {
	if x != nil {
		return x.xxx_hidden_Output
	}
	return ""
}

func (x *EchoResponse) SetOutput(v string) {
	x.xxx_hidden_Output = v
}

type EchoResponse_builder struct {
	_ [0]func() // Prevents comparability and use of unkeyed literals for the builder.

	Output string
}

func (b0 EchoResponse_builder) Build() *EchoResponse {
	m0 := &EchoResponse{}
	b, x := &b0, m0
	_, _ = b, x
	x.xxx_hidden_Output = b.Output
	return m0
}

var File_proto_echo_proto protoreflect.FileDescriptor

const file_proto_echo_proto_rawDesc = "" +
	"\n" +
	"\x10proto/echo.proto\x12\x06echopb\"#\n" +
	"\vEchoRequest\x12\x14\n" +
	"\x05input\x18\x01 \x01(\tR\x05input\"&\n" +
	"\fEchoResponse\x12\x16\n" +
	"\x06output\x18\x01 \x01(\tR\x06output2;\n" +
	"\x04Echo\x123\n" +
	"\x04Echo\x12\x13.echopb.EchoRequest\x1a\x14.echopb.EchoResponse\"\x00b\x06proto3"

var file_proto_echo_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_proto_echo_proto_goTypes = []any{
	(*EchoRequest)(nil),  // 0: echopb.EchoRequest
	(*EchoResponse)(nil), // 1: echopb.EchoResponse
}
var file_proto_echo_proto_depIdxs = []int32{
	0, // 0: echopb.Echo.Echo:input_type -> echopb.EchoRequest
	1, // 1: echopb.Echo.Echo:output_type -> echopb.EchoResponse
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_proto_echo_proto_init() }
func file_proto_echo_proto_init() {
	if File_proto_echo_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_proto_echo_proto_rawDesc), len(file_proto_echo_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_echo_proto_goTypes,
		DependencyIndexes: file_proto_echo_proto_depIdxs,
		MessageInfos:      file_proto_echo_proto_msgTypes,
	}.Build()
	File_proto_echo_proto = out.File
	file_proto_echo_proto_goTypes = nil
	file_proto_echo_proto_depIdxs = nil
}
