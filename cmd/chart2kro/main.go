// chart2kro transforms Helm charts into KRO ResourceGraphDefinition resources.
package main

import (
	"os"

	"github.com/hupe1980/chart2kro/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
