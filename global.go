package main

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sync"

	wl_crypto "github.com/wsva/lib_go/crypto"
	wl_db "github.com/wsva/lib_go_db"
	mlib "github.com/wsva/monitor_lib_go"

	"github.com/tidwall/pretty"
)

type TargetOracle struct {
	Name              string   `json:"Name"`
	Enable            bool     `json:"Enable"`
	Address           string   `json:"Address"`
	DB                wl_db.DB `json:"DB"`
	ExcludeTableSpace []string `json:"ExcludeTableSpace"`
}

// file comment
var (
	MainConfigFile = "gm_oracle_targets.json"
)

const (
	AESKey = "1"
	AESIV  = "2"
)

var targetList []TargetOracle

var resultsRuntime []mlib.MR
var resultsRuntimeLock sync.Mutex

func initGlobals() error {
	basepath, err := os.Executable()
	if err != nil {
		return err
	}
	MainConfigFile = path.Join(filepath.Dir(basepath), MainConfigFile)

	contentBytes, err := os.ReadFile(MainConfigFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(contentBytes, &targetList)
	if err != nil {
		return err
	}

	err = decryptMainConfig()
	if err != nil {
		return err
	}
	err = encryptMainConfigFile()
	if err != nil {
		return err
	}

	return nil
}

func decryptMainConfig() error {
	for k := range targetList {
		if _, ok := wl_crypto.ParseAES256Text(targetList[k].DB.Oracle.Password); ok {
			text, err := wl_crypto.AES256Decrypt(
				AESKey, AESIV, targetList[k].DB.Oracle.Password)
			if err != nil {
				return err
			}
			targetList[k].DB.Oracle.Password = text
		}
	}
	return nil
}

func encryptMainConfigFile() error {
	newTargetList := make([]TargetOracle, len(targetList))
	copy(newTargetList, targetList)
	for k := range newTargetList {
		ctext, err := wl_crypto.AES256Encrypt(
			AESKey, AESIV, newTargetList[k].DB.Oracle.Password)
		if err != nil {
			return err
		}
		newTargetList[k].DB.Oracle.Password = ctext
	}
	jsonBytes, err := json.Marshal(newTargetList)
	if err != nil {
		return err
	}
	err = os.WriteFile(MainConfigFile, pretty.Pretty(jsonBytes), 0666)
	if err != nil {
		return err
	}
	return nil
}
