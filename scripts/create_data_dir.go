package main

import (
	"os"
)

func main() {
	err := os.MkdirAll("./data", 0755)
	if err != nil {
		panic(err)
	}
}
