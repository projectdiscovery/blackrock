package main

import (
	"fmt"

	"github.com/projectdiscovery/blackrock"
)

func main() {
	bl := blackrock.New(10, 5)
	fmt.Println(bl)

	shuffle := bl.Shuffle(1)
	fmt.Println(shuffle)
}
