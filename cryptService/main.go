package main

import (
	"crypt/service"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	// 初始化密钥 - 替换为你的实际文件路径
	pkPath := "ckkskeys/publickey.bin" // Linux系统下的公钥路径
	skPath := "ckkskeys/secretkey.bin" // Linux系统下的私钥路径

	if err := service.InitKeys(pkPath, skPath); err != nil {
		log.Fatalf("Failed to initialize keys: %v", err)
	}

	r := gin.Default()

	r.POST("/keygen", service.KeyGenHandler)
	r.POST("/encrypt", service.EncryptHandler)
	r.POST("/decrypt", service.DecryptHandler)

	r.Run(":8989")

}
