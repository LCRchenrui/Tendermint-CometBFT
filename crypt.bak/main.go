package main

import (
	"crypt/service"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.POST("/keygen", service.KeyGenHandler)
	r.POST("/encrypt", service.EncryptHandler)
	r.POST("/decrypt", service.DecryptHandler)

	r.Run(":8989")

}
