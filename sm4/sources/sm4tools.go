package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tjfoc/gmsm/sm4"
)

func main() {
	var decrypt bool
	var key string
	flag.StringVar(&key, "key", "12345678890abcdef", "The SM4 Key for encrypt/decrypt.")
	flag.BoolVar(&decrypt, "decrypt", false, "Decrypt the data instead of encrypt it.")
	hk := []byte(key)
	if len(os.Args) != 1 {

	}
	if len(key) != 16 {
		fmt.Errorf("key length error!(should be 16)")
		return
	}
	fmt.Printf("key = %v\n", hk)
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}
	fmt.Printf("data = %x\n", data)
	iv := []byte("0000000000000000")
	err := sm4.SetIV(iv)                      //设置SM4算法实现的IV值,不设置则使用默认值
	ecbMsg, err := sm4.Sm4Ecb(hk, data, true) //sm4Ecb模式pksc7填充加密
	if err != nil {
		fmt.Errorf("sm4 enc error:%s", err)
		return
	}
	fmt.Printf("ecbMsg = %x\n", ecbMsg)
	ecbDec, err := sm4.Sm4Ecb(hk, ecbMsg, false) //sm4Ecb模式pksc7填充解密
	if err != nil {
		fmt.Errorf("sm4 dec error:%s", err)
		return
	}
	fmt.Printf("ecbDec = %x\n", ecbDec)
}
