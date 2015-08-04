package main

import (
	"flag"
)

var (
	safe bool
)

func init() {
	flag.BoolVar(&safe, "safe", true, "Safe mode")
}

func main() {
	flag.Parse()
}
