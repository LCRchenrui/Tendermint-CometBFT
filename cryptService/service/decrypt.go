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

	// 2. 读取密文 JSON

	jsonStr := c.PostForm("data")

	ctBytes, err := base64.StdEncoding.DecodeString(jsonStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ciphertext format"})
		return
	}

	ct := rlwe.NewCiphertext(globalParams, 1, globalParams.MaxLevel())
	if err := ct.UnmarshalBinary(ctBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unmarshal failed"})
		return
	}
	have := make([]float64, globalParams.LogMaxSlots())
	decryptor := rlwe.NewDecryptor(globalParams, globalSK)
	pt := decryptor.DecryptNew(ct)
	encoder := ckks.NewEncoder(globalParams)
	if err = encoder.Decode(pt, have); err != nil {
		panic(err)
	}

	c.JSON(http.StatusOK, gin.H{"result": have})
}
