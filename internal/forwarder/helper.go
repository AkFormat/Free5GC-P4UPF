package forwarder

import (
	"fmt"
	"net"
	"strconv"
)

// TODO: Function， 将string的IPv6转换为byte
//点分十进制转化为32bit字符
func ConvertIpv4ToBytes(ipv4 string) ([]byte, error) {
	ip := net.ParseIP(ipv4)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address")
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("convert failed, because it is not an IPv4 address")
	}
	return ip4, nil
}

// TODO: 将0x开头的字符串当做2进制，转义为对应的byte
func ByteStringToByte(byteString string) (byte, error) {
	n, err := strconv.ParseUint(byteString[2:], 2, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid binary string")
	}
	return byte(n), nil
}

func StrToInt32(str string) (int32, error) {
	// 第2个参数为整数转换的进制，这里使用十进制
	i64, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, err
	}
	// 将int64类型的值转换为int32类型的值
	i32 := int32(i64)
	return i32, nil
}

func isBitSet(n int, i uint) bool {
	// 用于判断bit 是否为1
	return (n & (1 << i)) != 0
}
