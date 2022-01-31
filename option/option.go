package option

import (
	"akt-rpc/codec"
)

const MagicNumber = 0x3bef5c

// 存储编解码信息，使用json传递
// | Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
// | <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|
type Option struct {
	MagicNumber int        // 表示为rpc请求
	CodecType   codec.Type // 解码形式
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}
