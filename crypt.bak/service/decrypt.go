package service

import (
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
)

type DecryptRequest struct {
	Ciphertext string `json:"ciphertext"`
}

func DecryptHandler(c *gin.Context) {

	// 1. 读取私钥二进制
	skFile, _, err := c.Request.FormFile("secret_key")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "secret_key missing"})
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

	sk := rlwe.NewSecretKey(params)

	if sksize, err := sk.ReadFrom(skFile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid secret key", "sksize": sksize})
		return
	}
	// 2. 读取密文 JSON

	jsonStr := c.PostForm("data")
	// fmt.Println("Ciphertext:", jsonStr)
	// var req struct {
	// 	Ciphertext string `json:"data" binding:"required"`
	// }
	// if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Unmarshal invalid cypher data"})
	// 	return
	// }

	// 3. 解码密文
	//fmt.Println("Ciphertext:", req.Ciphertext)
	ctBytes, err := base64.StdEncoding.DecodeString(jsonStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ciphertext format"})
		return
	}

	ct := rlwe.NewCiphertext(params, 1, params.MaxLevel())
	if err := ct.UnmarshalBinary(ctBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unmarshal failed"})
		return
	}
	have := make([]float64, params.LogMaxSlots())
	decryptor := rlwe.NewDecryptor(params, sk)
	pt := decryptor.DecryptNew(ct)
	encoder := ckks.NewEncoder(params)
	if err = encoder.Decode(pt, have); err != nil {
		panic(err)
	}

	c.JSON(http.StatusOK, gin.H{"result": have})
}
