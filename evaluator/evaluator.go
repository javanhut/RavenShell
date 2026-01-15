package evaluator

import (
	"fmt"
	"path"
)

func ChangeDir(targetDir string) {
	fmt.Println(path.Base(targetDir))
}
