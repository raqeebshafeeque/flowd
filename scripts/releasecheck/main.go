// releasecheck is a CI tool, not part of the installed application.
package main

import (
	"fmt"
	"github.com/raqeebshafeeque/flowd/internal/releasecheck"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: releasecheck VERSION DIRECTORY")
		os.Exit(2)
	}
	if err := releasecheck.Verify(os.Args[2], os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("verified release " + os.Args[1])
}
