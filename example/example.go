package main

import (
	"fmt"

	"github.com/projectdiscovery/blackrock"
)

func main() {
	test := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "h"}
	bl := blackrock.New(10, 5)

	for i := 0; i < len(test); i++ {
		idx := bl.Shuffle(int64(i))
		fmt.Println(test[idx])
	}
}
