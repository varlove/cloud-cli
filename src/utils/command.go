package utils

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
)

// ExecLinuxCmd 执行脚本
func ExecLinuxCmd(cmdStr string) ([]byte, error) {

	// 生成临时文件
	file, err := ioutil.TempFile("", "tmp-script")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	if _, err = file.WriteString(cmdStr); err != nil {
		return nil, err
	}
	file.Close()

	var cmd = exec.Command("/bin/bash", file.Name())
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	err = cmd.Wait()
	return output.Bytes(), err
}
