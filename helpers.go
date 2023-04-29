package main

import (
	"fmt"
	"os"
)

func DebugPrint(s string, a ...any) {
	if debug {
		fmt.Printf(s, a)
	}
}

func Exit(m string, i int) {
	fmt.Print(m)
	os.Exit(i)
}

func printTokenUsage(usages []int) {
	usage := 0
	for _, u := range usages {
		usage += u
	}
	fmt.Printf("Used %d OpenAI tokens", usage)
}
