package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

// ensureWorkDir prepare working directory
func ensureWorkDir(dir string) (string, error) {
	var err error
	if dir == "" {
		dir, err = ioutil.TempDir("", "fnGotplRender_")
		if err != nil {
			return "", err
		}
	} else {
		// create if missing
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			err := os.MkdirAll(dir, 0770)
			if err != nil {
				return "", err
			}
		}
	}
	return dir, nil
}

// stringInSlice boolean function
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// FlattenMap flatten context values to snake_case
// How about https://godoc.org/github.com/jeremywohl/flatten
func FlattenMap(prefix string, src map[string]interface{}, dest map[string]interface{}) {
	if len(prefix) > 0 {
		prefix += "_"
	}
	for k, v := range src {
		switch child := v.(type) {
		case map[string]interface{}:
			FlattenMap(prefix+k, child, dest)
		default:
			dest[prefix+k] = v
		}
	}
}

//WalkMatch returns list of files matching regex pattern
// func WalkMatch(root, pattern string) ([]string, error) {
// 	var matches []string
// 	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if info.IsDir() {
// 			return nil
// 			// TODO recursive
// 		}
// 		if matched, err := filepath.Match(pattern, filepath.Base(path)); err != nil {
// 			return err
// 		} else if matched {
// 			matches = append(matches, path)
// 		}
// 		return nil
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	return matches, nil
// }

//getRepoCreds read reference to credentials and returns go-getter URI
func getRepoCreds(repoCreds string) (string, error) {
	var cr = ""
	if repoCreds != "" {
		for _, e := range strings.Split(repoCreds, ",") {
			pair := strings.SplitN(e, "=", 2)
			//sshkey - for private git repositories
			if pair[0] == "sshkey" {
				key, err := ioutil.ReadFile(pair[1])
				if err != nil {
					return cr, err
				}
				keyb64 := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(string(key))))
				cr = fmt.Sprintf("%s?sshkey=%s", cr, string(keyb64))
			}
		}
	}
	return cr, nil
}
