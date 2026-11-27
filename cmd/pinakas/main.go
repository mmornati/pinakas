// Command pinakas keeps AI coding agents on the board.
package main

import (
	"os"

	"github.com/mmornati/pinakas/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.NewApp(), os.Args[1:]))
}
