package chart2kro_test

import (
	"context"
	"fmt"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

func ExampleConvert() {
	res, err := chart2kro.Convert(context.Background(), chart2kro.Options{
		Chart:  "../../testdata/charts/simple",
		Expose: []string{"replicaCount", "image.tag"},
		Group:  "apps.example.com",
	})
	if err != nil {
		fmt.Println("exit code", chart2kro.ExitCode(err))
		return
	}

	for _, v := range res.Report.Values {
		fmt.Println(v.Path, v.Outcome, v.Type)
	}
	// Output:
	// image.tag exposed string
	// replicaCount exposed integer
}
