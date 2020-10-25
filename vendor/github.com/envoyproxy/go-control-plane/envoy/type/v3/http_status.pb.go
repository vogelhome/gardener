// Code generated by protoc-gen-go. DO NOT EDIT.
// source: envoy/type/v3/http_status.proto

package envoy_type_v3

import (
	fmt "fmt"
	_ "github.com/cncf/udpa/go/udpa/annotations"
	_ "github.com/envoyproxy/protoc-gen-validate/validate"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type StatusCode int32

const (
	StatusCode_Empty                         StatusCode = 0
	StatusCode_Continue                      StatusCode = 100
	StatusCode_OK                            StatusCode = 200
	StatusCode_Created                       StatusCode = 201
	StatusCode_Accepted                      StatusCode = 202
	StatusCode_NonAuthoritativeInformation   StatusCode = 203
	StatusCode_NoContent                     StatusCode = 204
	StatusCode_ResetContent                  StatusCode = 205
	StatusCode_PartialContent                StatusCode = 206
	StatusCode_MultiStatus                   StatusCode = 207
	StatusCode_AlreadyReported               StatusCode = 208
	StatusCode_IMUsed                        StatusCode = 226
	StatusCode_MultipleChoices               StatusCode = 300
	StatusCode_MovedPermanently              StatusCode = 301
	StatusCode_Found                         StatusCode = 302
	StatusCode_SeeOther                      StatusCode = 303
	StatusCode_NotModified                   StatusCode = 304
	StatusCode_UseProxy                      StatusCode = 305
	StatusCode_TemporaryRedirect             StatusCode = 307
	StatusCode_PermanentRedirect             StatusCode = 308
	StatusCode_BadRequest                    StatusCode = 400
	StatusCode_Unauthorized                  StatusCode = 401
	StatusCode_PaymentRequired               StatusCode = 402
	StatusCode_Forbidden                     StatusCode = 403
	StatusCode_NotFound                      StatusCode = 404
	StatusCode_MethodNotAllowed              StatusCode = 405
	StatusCode_NotAcceptable                 StatusCode = 406
	StatusCode_ProxyAuthenticationRequired   StatusCode = 407
	StatusCode_RequestTimeout                StatusCode = 408
	StatusCode_Conflict                      StatusCode = 409
	StatusCode_Gone                          StatusCode = 410
	StatusCode_LengthRequired                StatusCode = 411
	StatusCode_PreconditionFailed            StatusCode = 412
	StatusCode_PayloadTooLarge               StatusCode = 413
	StatusCode_URITooLong                    StatusCode = 414
	StatusCode_UnsupportedMediaType          StatusCode = 415
	StatusCode_RangeNotSatisfiable           StatusCode = 416
	StatusCode_ExpectationFailed             StatusCode = 417
	StatusCode_MisdirectedRequest            StatusCode = 421
	StatusCode_UnprocessableEntity           StatusCode = 422
	StatusCode_Locked                        StatusCode = 423
	StatusCode_FailedDependency              StatusCode = 424
	StatusCode_UpgradeRequired               StatusCode = 426
	StatusCode_PreconditionRequired          StatusCode = 428
	StatusCode_TooManyRequests               StatusCode = 429
	StatusCode_RequestHeaderFieldsTooLarge   StatusCode = 431
	StatusCode_InternalServerError           StatusCode = 500
	StatusCode_NotImplemented                StatusCode = 501
	StatusCode_BadGateway                    StatusCode = 502
	StatusCode_ServiceUnavailable            StatusCode = 503
	StatusCode_GatewayTimeout                StatusCode = 504
	StatusCode_HTTPVersionNotSupported       StatusCode = 505
	StatusCode_VariantAlsoNegotiates         StatusCode = 506
	StatusCode_InsufficientStorage           StatusCode = 507
	StatusCode_LoopDetected                  StatusCode = 508
	StatusCode_NotExtended                   StatusCode = 510
	StatusCode_NetworkAuthenticationRequired StatusCode = 511
)

var StatusCode_name = map[int32]string{
	0:   "Empty",
	100: "Continue",
	200: "OK",
	201: "Created",
	202: "Accepted",
	203: "NonAuthoritativeInformation",
	204: "NoContent",
	205: "ResetContent",
	206: "PartialContent",
	207: "MultiStatus",
	208: "AlreadyReported",
	226: "IMUsed",
	300: "MultipleChoices",
	301: "MovedPermanently",
	302: "Found",
	303: "SeeOther",
	304: "NotModified",
	305: "UseProxy",
	307: "TemporaryRedirect",
	308: "PermanentRedirect",
	400: "BadRequest",
	401: "Unauthorized",
	402: "PaymentRequired",
	403: "Forbidden",
	404: "NotFound",
	405: "MethodNotAllowed",
	406: "NotAcceptable",
	407: "ProxyAuthenticationRequired",
	408: "RequestTimeout",
	409: "Conflict",
	410: "Gone",
	411: "LengthRequired",
	412: "PreconditionFailed",
	413: "PayloadTooLarge",
	414: "URITooLong",
	415: "UnsupportedMediaType",
	416: "RangeNotSatisfiable",
	417: "ExpectationFailed",
	421: "MisdirectedRequest",
	422: "UnprocessableEntity",
	423: "Locked",
	424: "FailedDependency",
	426: "UpgradeRequired",
	428: "PreconditionRequired",
	429: "TooManyRequests",
	431: "RequestHeaderFieldsTooLarge",
	500: "InternalServerError",
	501: "NotImplemented",
	502: "BadGateway",
	503: "ServiceUnavailable",
	504: "GatewayTimeout",
	505: "HTTPVersionNotSupported",
	506: "VariantAlsoNegotiates",
	507: "InsufficientStorage",
	508: "LoopDetected",
	510: "NotExtended",
	511: "NetworkAuthenticationRequired",
}

var StatusCode_value = map[string]int32{
	"Empty":                         0,
	"Continue":                      100,
	"OK":                            200,
	"Created":                       201,
	"Accepted":                      202,
	"NonAuthoritativeInformation":   203,
	"NoContent":                     204,
	"ResetContent":                  205,
	"PartialContent":                206,
	"MultiStatus":                   207,
	"AlreadyReported":               208,
	"IMUsed":                        226,
	"MultipleChoices":               300,
	"MovedPermanently":              301,
	"Found":                         302,
	"SeeOther":                      303,
	"NotModified":                   304,
	"UseProxy":                      305,
	"TemporaryRedirect":             307,
	"PermanentRedirect":             308,
	"BadRequest":                    400,
	"Unauthorized":                  401,
	"PaymentRequired":               402,
	"Forbidden":                     403,
	"NotFound":                      404,
	"MethodNotAllowed":              405,
	"NotAcceptable":                 406,
	"ProxyAuthenticationRequired":   407,
	"RequestTimeout":                408,
	"Conflict":                      409,
	"Gone":                          410,
	"LengthRequired":                411,
	"PreconditionFailed":            412,
	"PayloadTooLarge":               413,
	"URITooLong":                    414,
	"UnsupportedMediaType":          415,
	"RangeNotSatisfiable":           416,
	"ExpectationFailed":             417,
	"MisdirectedRequest":            421,
	"UnprocessableEntity":           422,
	"Locked":                        423,
	"FailedDependency":              424,
	"UpgradeRequired":               426,
	"PreconditionRequired":          428,
	"TooManyRequests":               429,
	"RequestHeaderFieldsTooLarge":   431,
	"InternalServerError":           500,
	"NotImplemented":                501,
	"BadGateway":                    502,
	"ServiceUnavailable":            503,
	"GatewayTimeout":                504,
	"HTTPVersionNotSupported":       505,
	"VariantAlsoNegotiates":         506,
	"InsufficientStorage":           507,
	"LoopDetected":                  508,
	"NotExtended":                   510,
	"NetworkAuthenticationRequired": 511,
}

func (x StatusCode) String() string {
	return proto.EnumName(StatusCode_name, int32(x))
}

func (StatusCode) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_588aaadec77e6b51, []int{0}
}

type HttpStatus struct {
	Code                 StatusCode `protobuf:"varint,1,opt,name=code,proto3,enum=envoy.type.v3.StatusCode" json:"code,omitempty"`
	XXX_NoUnkeyedLiteral struct{}   `json:"-"`
	XXX_unrecognized     []byte     `json:"-"`
	XXX_sizecache        int32      `json:"-"`
}

func (m *HttpStatus) Reset()         { *m = HttpStatus{} }
func (m *HttpStatus) String() string { return proto.CompactTextString(m) }
func (*HttpStatus) ProtoMessage()    {}
func (*HttpStatus) Descriptor() ([]byte, []int) {
	return fileDescriptor_588aaadec77e6b51, []int{0}
}

func (m *HttpStatus) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_HttpStatus.Unmarshal(m, b)
}
func (m *HttpStatus) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_HttpStatus.Marshal(b, m, deterministic)
}
func (m *HttpStatus) XXX_Merge(src proto.Message) {
	xxx_messageInfo_HttpStatus.Merge(m, src)
}
func (m *HttpStatus) XXX_Size() int {
	return xxx_messageInfo_HttpStatus.Size(m)
}
func (m *HttpStatus) XXX_DiscardUnknown() {
	xxx_messageInfo_HttpStatus.DiscardUnknown(m)
}

var xxx_messageInfo_HttpStatus proto.InternalMessageInfo

func (m *HttpStatus) GetCode() StatusCode {
	if m != nil {
		return m.Code
	}
	return StatusCode_Empty
}

func init() {
	proto.RegisterEnum("envoy.type.v3.StatusCode", StatusCode_name, StatusCode_value)
	proto.RegisterType((*HttpStatus)(nil), "envoy.type.v3.HttpStatus")
}

func init() { proto.RegisterFile("envoy/type/v3/http_status.proto", fileDescriptor_588aaadec77e6b51) }

var fileDescriptor_588aaadec77e6b51 = []byte{
	// 957 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x54, 0x49, 0x73, 0x1b, 0x45,
	0x14, 0xce, 0xa8, 0x9d, 0x38, 0xee, 0x78, 0xe9, 0x74, 0x62, 0x9c, 0x38, 0x01, 0x4c, 0x4e, 0x14,
	0x07, 0x8b, 0xc2, 0x27, 0xb8, 0xd9, 0x8e, 0x1d, 0xbb, 0xb0, 0x14, 0x95, 0x2c, 0xe5, 0x4a, 0xb5,
	0xa7, 0x9f, 0xa4, 0xae, 0x8c, 0xfa, 0x4d, 0x7a, 0xde, 0xc8, 0x1e, 0x8e, 0x9c, 0x38, 0xb2, 0x2f,
	0x61, 0x3f, 0xb0, 0x14, 0x95, 0x10, 0x28, 0xe0, 0x27, 0x50, 0xc5, 0x0e, 0xbf, 0x81, 0xdf, 0xc0,
	0x1a, 0x28, 0xa0, 0xba, 0x47, 0x92, 0x93, 0x43, 0x6e, 0x33, 0x6f, 0xde, 0xf2, 0x2d, 0x6f, 0x1e,
	0x7f, 0x10, 0xec, 0x00, 0x8b, 0x2a, 0x15, 0x29, 0x54, 0x07, 0x2b, 0xd5, 0x1e, 0x51, 0xfa, 0x54,
	0x46, 0x8a, 0xf2, 0x6c, 0x39, 0x75, 0x48, 0x28, 0x67, 0x42, 0xc2, 0xb2, 0x4f, 0x58, 0x1e, 0xac,
	0x2c, 0x3e, 0x94, 0xeb, 0x54, 0x55, 0x95, 0xb5, 0x48, 0x8a, 0x0c, 0xda, 0xac, 0x3a, 0x00, 0x97,
	0x19, 0xb4, 0xc6, 0x76, 0xcb, 0x8a, 0xc5, 0x85, 0x81, 0x4a, 0x8c, 0x56, 0x04, 0xd5, 0xd1, 0x43,
	0xf9, 0xe1, 0x02, 0x70, 0xbe, 0x45, 0x94, 0xee, 0x86, 0xf6, 0xf2, 0x71, 0x3e, 0x11, 0xa3, 0x86,
	0x33, 0xd1, 0x52, 0xf4, 0xf0, 0xec, 0x63, 0x67, 0x97, 0xef, 0x9a, 0xb3, 0x5c, 0x26, 0xad, 0xa3,
	0x86, 0x35, 0x7e, 0x7b, 0x6d, 0xf2, 0x99, 0x68, 0x42, 0x44, 0x4b, 0x47, 0x9a, 0xa1, 0xe4, 0x89,
	0xf3, 0xd7, 0xbf, 0x7a, 0xf6, 0x81, 0x05, 0x3e, 0x7f, 0x47, 0xc9, 0x61, 0xe3, 0x47, 0xbe, 0x9c,
	0xe2, 0xfc, 0xb0, 0x5c, 0x4e, 0xf1, 0xa3, 0x1b, 0xfd, 0x94, 0x0a, 0x71, 0x44, 0x4e, 0xf3, 0xe3,
	0xeb, 0x68, 0xc9, 0xd8, 0x1c, 0x84, 0x96, 0x93, 0xbc, 0x72, 0xf9, 0x49, 0xf1, 0x75, 0x24, 0xa7,
	0xf9, 0xe4, 0xba, 0x03, 0x45, 0xa0, 0xc5, 0x37, 0x91, 0x9c, 0xe1, 0xc7, 0x57, 0xe3, 0x18, 0x52,
	0xff, 0xfa, 0x6d, 0x24, 0x97, 0xf8, 0xb9, 0x3a, 0xda, 0xd5, 0x9c, 0x7a, 0xe8, 0x8c, 0xe7, 0x3c,
	0x80, 0x6d, 0xdb, 0x41, 0xd7, 0x0f, 0xf4, 0xc5, 0x77, 0x91, 0x9c, 0xe5, 0x53, 0x75, 0xf4, 0x7d,
	0xc1, 0x92, 0xf8, 0x3e, 0x92, 0x27, 0xf9, 0x74, 0x13, 0x32, 0xa0, 0x51, 0xe8, 0x87, 0x48, 0x9e,
	0xe2, 0xb3, 0x0d, 0xe5, 0xc8, 0xa8, 0x64, 0x14, 0xfc, 0x31, 0x92, 0x82, 0x9f, 0xa8, 0xe5, 0x09,
	0x99, 0x12, 0xab, 0xf8, 0x29, 0x92, 0xa7, 0xf9, 0xdc, 0x6a, 0xe2, 0x40, 0xe9, 0xa2, 0x09, 0x29,
	0x3a, 0x8f, 0xe0, 0xe7, 0x48, 0x9e, 0xe0, 0xc7, 0xb6, 0x6b, 0xed, 0x0c, 0xb4, 0xf8, 0x25, 0xa4,
	0x84, 0xa2, 0x34, 0x81, 0xf5, 0x1e, 0x9a, 0x18, 0x32, 0x71, 0xa3, 0x22, 0xe7, 0xb9, 0xa8, 0xe1,
	0x00, 0x74, 0x03, 0x5c, 0x5f, 0x59, 0xb0, 0x94, 0x14, 0xe2, 0x66, 0x45, 0x72, 0x7e, 0x74, 0x13,
	0x73, 0xab, 0xc5, 0x27, 0x15, 0x4f, 0x6b, 0x17, 0xe0, 0x32, 0xf5, 0xc0, 0x89, 0x5b, 0x15, 0x3f,
	0xbc, 0x8e, 0x54, 0x43, 0x6d, 0x3a, 0x06, 0xb4, 0xf8, 0x34, 0x24, 0xb4, 0x33, 0x68, 0x38, 0x3c,
	0x28, 0xc4, 0x67, 0x15, 0x79, 0x1f, 0x3f, 0xd9, 0x82, 0x7e, 0x8a, 0x4e, 0xb9, 0xa2, 0x09, 0xda,
	0x38, 0x88, 0x49, 0x7c, 0x1e, 0xe2, 0xe3, 0x29, 0xe3, 0xf8, 0x17, 0x15, 0x39, 0xc7, 0xf9, 0x9a,
	0xd2, 0x4d, 0xb8, 0x96, 0x43, 0x46, 0xe2, 0x39, 0xe6, 0x65, 0x68, 0x5b, 0x55, 0xea, 0xf6, 0x34,
	0x68, 0xf1, 0x3c, 0xf3, 0xe0, 0x1b, 0xaa, 0xe8, 0x87, 0xca, 0x6b, 0xb9, 0x71, 0xa0, 0xc5, 0x0b,
	0xcc, 0xeb, 0xb7, 0x89, 0x6e, 0xcf, 0x68, 0x0d, 0x56, 0xbc, 0xc8, 0x3c, 0x90, 0x3a, 0x52, 0x09,
	0xfc, 0x25, 0x16, 0xb8, 0x01, 0xf5, 0x50, 0xd7, 0x91, 0x56, 0x93, 0x04, 0xf7, 0x41, 0x8b, 0x97,
	0x99, 0x94, 0x7c, 0xc6, 0x07, 0x82, 0x53, 0x6a, 0x2f, 0x01, 0xf1, 0x0a, 0xf3, 0x5e, 0x05, 0xfc,
	0xde, 0x2d, 0xb0, 0x64, 0xe2, 0xe0, 0xd1, 0x78, 0xd6, 0xab, 0xcc, 0x1b, 0x31, 0x84, 0xd8, 0x32,
	0x7d, 0xc0, 0x9c, 0xc4, 0x6b, 0x61, 0xe0, 0x3a, 0xda, 0x4e, 0x62, 0x62, 0x12, 0xaf, 0x33, 0x39,
	0xc5, 0x27, 0x2e, 0xa1, 0x05, 0x71, 0x3d, 0xa4, 0xef, 0x80, 0xed, 0x52, 0x6f, 0xdc, 0xe3, 0x0d,
	0x26, 0x17, 0xb8, 0x6c, 0x38, 0x88, 0xd1, 0x6a, 0xe3, 0xdb, 0x6f, 0x2a, 0x93, 0x80, 0x16, 0x6f,
	0x8e, 0xe8, 0x25, 0xa8, 0x74, 0x0b, 0x71, 0x47, 0xb9, 0x2e, 0x88, 0xb7, 0x98, 0x17, 0xa6, 0xdd,
	0xdc, 0xf6, 0x11, 0xb4, 0x5d, 0xf1, 0x36, 0x93, 0x67, 0xf9, 0xe9, 0xb6, 0xcd, 0xf2, 0xb4, 0x74,
	0xb8, 0x06, 0xda, 0xa8, 0x56, 0x91, 0x82, 0x78, 0x87, 0xc9, 0x33, 0xfc, 0x54, 0x53, 0xd9, 0x2e,
	0xd4, 0x91, 0x76, 0x15, 0x99, 0xac, 0x63, 0x02, 0xb5, 0x77, 0x99, 0x97, 0x7d, 0xe3, 0x20, 0x85,
	0xb8, 0xfc, 0xeb, 0x86, 0x33, 0xdf, 0x0b, 0x60, 0x6a, 0x26, 0x2b, 0x6d, 0x80, 0xb1, 0xfc, 0xef,
	0x87, 0x56, 0x6d, 0x9b, 0x3a, 0x8c, 0x21, 0xcb, 0x7c, 0x93, 0x0d, 0x4b, 0x86, 0x0a, 0xf1, 0x01,
	0xf3, 0xfb, 0xb4, 0x83, 0xf1, 0x55, 0xd0, 0xe2, 0xc3, 0xa0, 0x6e, 0xd9, 0xec, 0x22, 0xa4, 0x60,
	0x35, 0xd8, 0xb8, 0x10, 0x1f, 0x05, 0x2a, 0xed, 0xb4, 0xeb, 0x94, 0x86, 0x31, 0xf3, 0x8f, 0x03,
	0xf2, 0x3b, 0x99, 0x8f, 0x3f, 0xdd, 0x08, 0x05, 0x2d, 0xc4, 0x9a, 0xb2, 0xc5, 0x10, 0x43, 0x26,
	0x6e, 0x06, 0x43, 0x86, 0xaf, 0x5b, 0xa0, 0x34, 0xb8, 0x4d, 0x03, 0x89, 0xce, 0xc6, 0xea, 0xdc,
	0x0a, 0x30, 0xb7, 0x2d, 0x81, 0xb3, 0x2a, 0xd9, 0x05, 0x37, 0x00, 0xb7, 0xe1, 0x1c, 0x3a, 0xf1,
	0x6b, 0xd0, 0xbe, 0x8e, 0xb4, 0xdd, 0x4f, 0x13, 0xf0, 0x1b, 0x03, 0x5a, 0xfc, 0xc6, 0x86, 0x5b,
	0x76, 0x49, 0x11, 0xec, 0xab, 0x42, 0xfc, 0x1e, 0xf8, 0xfb, 0x3a, 0x13, 0x43, 0xdb, 0xaa, 0x81,
	0x32, 0x49, 0x10, 0xec, 0x8f, 0x50, 0x3e, 0x4c, 0x1b, 0x39, 0xfd, 0x27, 0x93, 0xe7, 0xf9, 0xc2,
	0x56, 0xab, 0xd5, 0xb8, 0x52, 0x9e, 0x2c, 0xaf, 0xf2, 0xc8, 0x06, 0xf1, 0x17, 0x93, 0x8b, 0x7c,
	0xfe, 0x8a, 0x72, 0x46, 0x59, 0x5a, 0x4d, 0x32, 0xac, 0x43, 0x17, 0xc9, 0x28, 0x82, 0x4c, 0xdc,
	0x1e, 0xe2, 0xcc, 0xf2, 0x4e, 0xc7, 0xc4, 0x06, 0x2c, 0xed, 0x12, 0x3a, 0xd5, 0x05, 0xf1, 0x77,
	0xd8, 0xf3, 0x1d, 0xc4, 0xf4, 0x22, 0x50, 0xb0, 0x40, 0xfc, 0xc3, 0x86, 0x3f, 0xd7, 0xc6, 0x01,
	0x79, 0x45, 0xb5, 0xf8, 0x97, 0xc9, 0x0b, 0xfc, 0xfe, 0x3a, 0xd0, 0x3e, 0xba, 0xab, 0xf7, 0xd8,
	0xcd, 0xff, 0xd8, 0xda, 0xa3, 0xfc, 0x9c, 0xc1, 0xf2, 0x0c, 0xa6, 0x7e, 0x8b, 0xef, 0xbe, 0x88,
	0x6b, 0x73, 0x87, 0x27, 0xae, 0xe1, 0xcf, 0x69, 0x23, 0xda, 0x3b, 0x16, 0xee, 0xea, 0xca, 0xff,
	0x01, 0x00, 0x00, 0xff, 0xff, 0xfa, 0x9a, 0xc8, 0xd2, 0xc5, 0x05, 0x00, 0x00,
}
