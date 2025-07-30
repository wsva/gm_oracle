package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tidwall/pretty"
	wl_crypto "github.com/wsva/lib_go/crypto"
	wl_db "github.com/wsva/lib_go_db"
)

type TargetOracle struct {
	Name              string   `json:"Name"`
	Enable            bool     `json:"Enable"`
	Address           string   `json:"Address"`
	DB                wl_db.DB `json:"DB"`
	ExcludeTableSpace []string `json:"ExcludeTableSpace"`
}

const (
	AESKey = "1"
	AESIV  = "2"
)

var MainTargetFile = "gm_oracle_targets.json"
var targetList []TargetOracle

func main() {
	fmt.Println("read file: " + MainTargetFile)
	contentBytes, err := os.ReadFile(MainTargetFile)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = json.Unmarshal(contentBytes, &targetList)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("unmarshal json:", len(targetList))

	for k, v := range targetList {
		if _, ok := wl_crypto.ParseAES256Text(v.DB.Oracle.Password); ok {
			text, err := wl_crypto.AES256Decrypt(AESKey, AESIV, v.DB.Oracle.Password)
			if err != nil {
				fmt.Println(err)
				return
			}
			targetList[k].DB.Oracle.Password = text
		}
	}

	fmt.Println("write to decrypt.json")
	jsonBytes, err := json.Marshal(targetList)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = os.WriteFile("decrypt.json", pretty.Pretty(jsonBytes), 0666)
	if err != nil {
		fmt.Println(err)
		return
	}
}
