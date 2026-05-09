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

var (
	globalParams ckks.Parameters
	globalPK     *rlwe.PublicKey
	globalSK     *rlwe.SecretKey
)

// 初始化密钥（从文件加载）
func InitKeys(pkPath, skPath string) error {
	// 创建加密参数
	params, err := ckks.NewParametersFromLiteral(ckks.ParametersLiteral{
		LogN:            14,
		LogQ:            []int{55, 45, 45, 45, 45, 45, 45, 45},
		LogP:            []int{61},
		LogDefaultScale: 45,
	})
	if err != nil {
		return fmt.Errorf("failed to create parameters: %w", err)
	}
	globalParams = params

	// 加载公钥
	//pubFile, err := os.Open
	pkBytes, err := os.Open(pkPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	globalPK = rlwe.NewPublicKey(globalParams)
	if _, err := globalPK.ReadFrom(pkBytes); err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// 加载私钥
	skBytes, err := os.Open(skPath)
	if err != nil {
		return fmt.Errorf("failed to read secret key: %w", err)
	}

	globalSK = rlwe.NewSecretKey(globalParams)
	if _, err := globalSK.ReadFrom(skBytes); err != nil {
		return fmt.Errorf("failed to parse secret key: %w", err)
	}

	return nil
}

func EncryptHandler(c *gin.Context) {

	// 2. 读取明文数据
	jsonStr := c.PostForm("data")

	var Data []float64
	if err := json.Unmarshal([]byte(jsonStr), &Data); err != nil {
		fmt.Println("invalid plain data")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plain data"})
		return
	}

	encoder := ckks.NewEncoder(globalParams)
	pt := ckks.NewPlaintext(globalParams, globalParams.MaxLevel())
	encoder.Encode(Data, pt)

	encryptor := rlwe.NewEncryptor(globalParams, globalPK)
	ct, _ := encryptor.EncryptNew(pt)

	ctBytes, _ := ct.MarshalBinary()
	b64 := base64.StdEncoding.EncodeToString(ctBytes)

	c.JSON(http.StatusOK, gin.H{"ciphertext": b64})
}
