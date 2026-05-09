package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
)

func KeyGenHandler(c *gin.Context) {
	// params, _ := ckks.NewParametersFromLiteral(ckks.ParametersLiteral{
	// 	LogN:            13,
	// 	LogQ:            []int{40, 35, 35, 35, 35},
	// 	LogP:            []int{61},
	// 	LogDefaultScale: 35,
	// 	RingType:        ring.ConjugateInvariant,
	// })
	params, _ := ckks.NewParametersFromLiteral(ckks.ParametersLiteral{
		LogN:            14,
		LogQ:            []int{55, 45, 45, 45, 45, 45, 45, 45},
		LogP:            []int{61},
		LogDefaultScale: 45,
	})
	kgen := rlwe.NewKeyGenerator(params)
	sk := kgen.GenSecretKeyNew()
	pk := kgen.GenPublicKeyNew(sk)

	// 创建内存中的 zip 文件
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// 写入 public.key
	pubFile, _ := zipWriter.Create("publickey.bin")
	pk.WriteTo(pubFile)
	fmt.Println("pksize=", pk.BinarySize())

	// 写入 secret.key
	secFile, _ := zipWriter.Create("secretkey.bin")
	sk.WriteTo(secFile)
	fmt.Println("sksize=", sk.BinarySize())

	//用于计算的密钥集合
	batch := 1
	n := params.MaxSlots()
	galoisKeys := kgen.GenGaloisKeysNew(params.GaloisElementsForReplicate(batch, n), sk)
	rlk := kgen.GenRelinearizationKeyNew(sk)
	evk := rlwe.NewMemEvaluationKeySet(rlk, galoisKeys...)
	evkFile, _ := zipWriter.Create("evkey.bin")
	evk.WriteTo(evkFile)

	// 完成压缩
	zipWriter.Close()

	// 设置 HTTP 响应头
	c.Header("Content-Disposition", "attachment; filename=keys.zip")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())

	// c.Writer.Header().Set("Content-Type", "application/octet-stream")
	// c.Writer.Write(append(pkBytes, skBytes...))
}
