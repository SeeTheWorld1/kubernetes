package main

import (
	"bytes"
	"log"
	"time"

	execTool "k8s.io/utils/exec"
)

var testExamples = []string{"www.baidu.com", "www.google.com", "11.158.187.191", "11.158.187.31", "11.160.137.48"}

// func main() {
// 	exec := execTool.New()

// 	for _, addr := range testExamples {
// 		cmd := exec.Command("ping", "-c 4", addr)
// 		cmd.SetStderr(os.Stderr)
// 		cmd.SetStdin(os.Stdin)
// 		cmd.SetStdout(os.Stdout)

// 		go func() {
// 			err := cmd.Run()
// 			if err != nil {
// 				log.Fatalf("failed to call cmd.Run(): %v", err)
// 			}
// 		}()
// 	}
// 	time.Sleep(time.Second)
// }

func main() {
	exec := execTool.New()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command("ping", "-c 4", "www.baidu.com")
	cmd.SetStderr(&stderr)
	cmd.SetStdout(&stdout)

	err := cmd.Run()
	if err != nil {
		log.Fatalf("failed to call cmd.Run(): %v", err)
	}
	log.Printf("out:\n%s\nerr:\n%s", stdout.String(), stderr.String())

	time.Sleep(time.Second)
}
