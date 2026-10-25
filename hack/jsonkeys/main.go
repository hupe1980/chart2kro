// Command jsonkeys prints the keys of a top-level JSON object field, one per line.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var doc map[string]map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		panic(err)
	}

	keys := make([]string, 0, len(doc[os.Args[2]]))
	for k := range doc[os.Args[2]] {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Println(k)
	}
}
