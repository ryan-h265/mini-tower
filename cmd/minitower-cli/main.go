package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		exitCode := 1
		if ee, ok := err.(*exitError); ok {
			exitCode = ee.Code
			if ee.Message != "" {
				fmt.Fprintln(os.Stderr, "error:", ee.Message)
			}
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(exitCode)
	}
}
