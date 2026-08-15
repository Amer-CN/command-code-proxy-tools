// Command licensegen 是作者本地的激活码签发工具（私钥在 .author-keys/）。
//
// 用法：
//
//	go run ./cmd/licensegen XXXX-XXXX-XXXX-XXXX
//
// 输出对应机器码的激活码，发给用户即可。私钥绝不外发/入库。
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: licensegen <机器码，如 XXXX-XXXX-XXXX-XXXX>")
		fmt.Println("机器码在软件「激活」界面查看，或看日志里的 MachineCode。")
		os.Exit(1)
	}
	mc := strings.ToUpper(strings.TrimSpace(os.Args[1]))

	privB64, err := os.ReadFile(filepath.Join(".author-keys", "license_priv.b64"))
	if err != nil {
		fmt.Println("读私钥失败（应在本仓库根目录运行，私钥在 .author-keys/）:", err)
		os.Exit(1)
	}
	priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(privB64)))
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		fmt.Println("私钥格式错误")
		os.Exit(1)
	}

	sig := ed25519.Sign(ed25519.PrivateKey(priv), []byte(mc))
	code := base64.StdEncoding.EncodeToString(sig)
	// 分 4 段方便核对，激活时程序自动忽略空格
	var parts []string
	for i := 0; i < len(code); i += 22 {
		end := i + 22
		if end > len(code) {
			end = len(code)
		}
		parts = append(parts, code[i:end])
	}
	fmt.Println("机器码:", mc)
	fmt.Println("激活码:", strings.Join(parts, " "))
}
