package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
)

func EncryptHandler(c *gin.Context) {
	// 1. 读取公钥二进制
	pkFile, _, err := c.Request.FormFile("public_key")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "public_key missing"})
		return
	}

	params, _ := ckks.NewParametersFromLiteral(ckks.ParametersLiteral{
		LogN:            14,
		LogQ:            []int{55, 45, 45, 45, 45, 45, 45, 45},
		LogP:            []int{61},
		LogDefaultScale: 45,
	})
	// params, _ := ckks.NewParametersFromLiteral(ckks.ParametersLiteral{
	// 	LogN:            13,
	// 	LogQ:            []int{40, 35, 35, 35, 35},
	// 	LogP:            []int{61},
	// 	LogDefaultScale: 35,
	// 	RingType:        ring.ConjugateInvariant,
	// })
	pk := rlwe.NewPublicKey(params)

	if pksize, err := pk.ReadFrom(pkFile); err != nil {
		fmt.Println("invalid public key")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid public key", "pk_size": pksize})
		return
	}

	// 2. 读取明文数据
	jsonStr := c.PostForm("data")
	// var req struct {
	// 	Data []float64 `json:"data" binding:"required"`
	// }
	var Data []float64
	if err := json.Unmarshal([]byte(jsonStr), &Data); err != nil {
		fmt.Println("invalid plain data")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plain data"})
		return
	}

	encoder := ckks.NewEncoder(params)
	pt := ckks.NewPlaintext(params, params.MaxLevel())
	encoder.Encode(Data, pt)

	encryptor := rlwe.NewEncryptor(params, pk)
	ct, _ := encryptor.EncryptNew(pt)

	ctBytes, _ := ct.MarshalBinary()
	b64 := base64.StdEncoding.EncodeToString(ctBytes)
	fmt.Println("ciphertext binary size:", ct.BinarySize())
	fmt.Println("ciphertext metadata size:", ct.MetaData.BinarySize())
	fmt.Println("ciphertext Value size:", ct.Value.BinarySize())
	//fmt.Println("ciphertext binary size:", ct.BinarySize())
	str := []string{"ID", "Address"}
	file := bd{
		CipherData:     b64,
		CipherMetadata: str,
	}
	bdata, err := json.Marshal(file)
	if err != nil {
		fmt.Println("json marshal error")
	}
	//将 JSON 字节转换为字符串写入文件
	err = os.WriteFile("output1.json", bdata, 0644)
	if err != nil {
		fmt.Println("file write error:", err)
		return
	}

	fmt.Println("写入成功")

	c.JSON(http.StatusOK, gin.H{"ciphertext": b64})
}

type bd struct {
	CipherData     string   `json:"ciphertext"`
	CipherMetadata []string `json:"ciphertext_metadata"`
}
