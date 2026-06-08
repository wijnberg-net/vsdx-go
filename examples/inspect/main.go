// Command inspect prints the pages and text-bearing shapes of a .vsdx file.
//
// Usage:
//
//	go run ./examples/inspect path/to/diagram.vsdx
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/wijnberg-net/vsdx-go/vsdx"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <file.vsdx>", os.Args[0])
	}

	vis, err := vsdx.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer vis.Close()

	for i, name := range vis.GetPageNames() {
		fmt.Printf("Page %d: %s\n", i, name)
		for _, s := range vis.GetPage(i).AllShapes() {
			if t := s.Text(); t != "" {
				fmt.Printf("  shape %s: %q\n", s.ID, t)
			}
		}
	}
}
