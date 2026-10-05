// Code generated for vswitch/tunnel.proto. DO NOT EDIT.
package tunnel

import (
	reflect "reflect"
	sync "sync"

	proto "google.golang.org/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
)

type Frame struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
	Payload       []byte `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *Frame) Reset() {
	*x = Frame{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tunnel_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}
func (x *Frame) String() string { return protoimpl.X.MessageStringOf(x) }
func (*Frame) ProtoMessage()    {}
func (x *Frame) ProtoReflect() protoreflect.Message {
	mi := &file_tunnel_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}
func (*Frame) Descriptor() ([]byte, []int) { return file_tunnel_proto_rawDescGZIP(), []int{0} }
func (x *Frame) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

var File_tunnel_proto protoreflect.FileDescriptor
var file_tunnel_proto_rawDesc []byte
var file_tunnel_proto_once sync.Once
var file_tunnel_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_tunnel_proto_goTypes = []any{(*Frame)(nil)}
var file_tunnel_proto_depIdxs = []int32{0, 0}

func file_tunnel_proto_rawDescGZIP() []byte {
	file_tunnel_proto_once.Do(func() { file_tunnel_proto_rawDesc = protoimpl.X.CompressGZIP(file_tunnel_proto_rawDesc) })
	return file_tunnel_proto_rawDesc
}
func init() { file_tunnel_proto_init() }
func file_tunnel_proto_init() {
	if File_tunnel_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_tunnel_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*Frame); i {
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
	fd := &descriptorpb.FileDescriptorProto{Name: proto.String("tunnel.proto"), Package: proto.String("tunnel"), Syntax: proto.String("proto3"), Options: &descriptorpb.FileOptions{GoPackage: proto.String("vswitch/tunnel")}, MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Frame"), Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("payload"), JsonName: proto.String("payload"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()}}}}, Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("TunnelService"), Method: []*descriptorpb.MethodDescriptorProto{{Name: proto.String("Stream"), InputType: proto.String(".tunnel.Frame"), OutputType: proto.String(".tunnel.Frame"), ClientStreaming: proto.Bool(true), ServerStreaming: proto.Bool(true)}}}}}
	b, _ := proto.Marshal(fd)
	file_tunnel_proto_rawDesc = b
	out := protoimpl.TypeBuilder{File: protoimpl.DescBuilder{GoPackagePath: reflect.TypeOf(x{}).PkgPath(), RawDescriptor: file_tunnel_proto_rawDesc, NumEnums: 0, NumMessages: 1, NumExtensions: 0, NumServices: 1}, GoTypes: file_tunnel_proto_goTypes, DependencyIndexes: file_tunnel_proto_depIdxs, MessageInfos: file_tunnel_proto_msgTypes}.Build()
	File_tunnel_proto = out.File
	file_tunnel_proto_rawDesc = nil
	file_tunnel_proto_goTypes = nil
	file_tunnel_proto_depIdxs = nil
}
