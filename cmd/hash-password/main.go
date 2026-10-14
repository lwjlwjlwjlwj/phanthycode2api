// hash-password 生成 /admin 登录所需的 PBKDF2 密码哈希。
package main

import (
	"flag"
	"fmt"
	"os"

	"phanthycode2api/internal/admin"
)

func main() {
	password := flag.String("password", "", "admin password")
	flag.Parse()
	if *password == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/hash-password -password=...")
		os.Exit(1)
	}
	fmt.Println(admin.HashPassword(*password))
}
